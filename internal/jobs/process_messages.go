package jobs

import (
	"bytes"
	"html/template"
	"log"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

type messagesData struct {
	Publisher string
	Message   string
	Date      string
	MapName   string
}
type EmailTemplateData struct {
	Messages []messagesData
	MapName  string
	Summary  OverviewSummary
}

// BuildMessagesPrompt constructs the system and user messages for the messages AI overview.
func BuildMessagesPrompt(messages []messagesData, congregationName string) (systemMsg, userMsg string) {
	systemMsg = `You write a short digest for congregation administrators of the messages below,
sent by publishers about their maps: boundaries, access problems, special conditions,
requests, and address corrections such as missing house numbers or wrong unit details.
The full list is shown under your text.

Return JSON with exactly two fields:
  "overview": at most 2 sentences on what the messages are about.
  "todo": up to 3 short lines. Each names the map and the one thing an administrator
          should do, taken only from the messages. Leave the list empty if nothing needs doing.

` + plainLanguageRules

	var sb strings.Builder
	sb.WriteString("Recent publisher feedback for " + congregationName + " congregation:\n\n")
	for _, m := range messages {
		sb.WriteString("Map: " + m.MapName + "\n")
		sb.WriteString("Date: " + m.Date + "\n")
		sb.WriteString("Message: " + m.Message + "\n\n")
	}
	userMsg = sb.String()
	return
}

// generateMessagesAISummary builds an OverviewSummary from the messages list.
// Returns an empty OverviewSummary (Available=false) if AI is disabled or the call fails.
func generateMessagesAISummary(messages []messagesData, congregationName string) OverviewSummary {
	if len(messages) < overviewMinItems {
		return OverviewSummary{}
	}
	client := newLLMClient()
	if client == nil {
		log.Printf("AI overview skipped for messages (%s): OPENAI_API_KEY not set", congregationName)
		return OverviewSummary{}
	}

	systemMsg, userMsg := BuildMessagesPrompt(messages, congregationName)
	resp, err := client.generateOverview(systemMsg, userMsg, messagesOverviewSchema)
	if err != nil {
		log.Printf("AI overview: LLM call failed for messages (%s): %v", congregationName, err)
		return OverviewSummary{}
	}
	todo := resp.Todo
	if len(todo) > 3 {
		todo = todo[:3]
	}
	if err := groundedNumbers(userMsg, append([]string{resp.Overview}, todo...)...); err != nil {
		log.Printf("AI overview dropped for messages (%s): %v", congregationName, err)
		return OverviewSummary{}
	}

	return OverviewSummary{
		Available: true,
		Overview:  resp.Overview,
		Todo:      todo,
	}
}

// MapData holds a map ID for batch processing. Used by processInstructions
// (process_instructions.go), which stays map-scoped.
type MapData struct {
	ID string `db:"id"`
}

func processMessage(congID string, app core.App) error {
	log.Printf("Processing messages for congregation: %s", congID)

	if congID == "" {
		return apis.NewBadRequestError("Congregation ID is required", nil)
	}

	congRecord, err := app.FindRecordById("congregations", congID)
	if err != nil {
		log.Println("Error finding congregation:", err)
		return err
	}

	// No age bound here: this must sweep the entire unread backlog for the
	// congregation, not just messages created within the discovery window,
	// otherwise messages older than that window can never be reached again.
	messages, err := app.FindRecordsByFilter("messages", "congregation = {:congregation} && read = false && type != 'administrator'", "created", 0, 0, dbx.Params{"congregation": congID})
	if err != nil {
		log.Println("Error finding messages by filter:", err)
		return err
	}

	if len(messages) == 0 {
		log.Println("No messages found")
		return nil
	}

	if expandErrs := app.ExpandRecords(messages, []string{"map"}, nil); len(expandErrs) > 0 {
		log.Printf("Warning: failed to expand map for some messages: %v", expandErrs)
	}

	recipients, err := fetchCongregationRecipients(app, congID, true)
	if err != nil {
		log.Println("Error fetching recipients:", err)
		return err
	}

	if len(recipients) == 0 {
		log.Println("No recipients found")
		return nil
	}
	log.Printf("Processing %d recipients\n", len(recipients))

	tmpl, err := template.ParseFiles("templates/messages.html")
	if err != nil {
		log.Println("Error parsing template:", err)
		return err
	}

	emailData := EmailTemplateData{
		Messages: make([]messagesData, 0),
	}

	location := loadCongregationLocation(congRecord)

	for _, message := range messages {
		mapName := "(unknown map)"
		if mapData := message.ExpandedOne("map"); mapData != nil {
			if name, ok := mapData.Get("description").(string); ok {
				mapName = name
			}
		}
		emailData.Messages = append(emailData.Messages, messagesData{
			Publisher: message.Get("created_by").(string),
			Message:   message.Get("message").(string),
			Date:      message.GetDateTime("created").Time().In(location).Format("03:04 PM, 02 Jan 2006"),
			MapName:   mapName,
		})
	}

	congregationName, _ := congRecord.Get("name").(string)

	if IsAISummaryEnabled() {
		emailData.Summary = generateMessagesAISummary(emailData.Messages, congregationName)
	}

	var body bytes.Buffer
	if err := tmpl.Execute(&body, emailData); err != nil {
		log.Println("Error executing template:", err)
		return err
	}

	subject := "New messages received for " + congregationName
	if err := sendHTMLEmail(recipients, subject, body.String()); err != nil {
		log.Println("Error sending email:", err)
		return err
	}
	log.Println("Email sent successfully")

	for _, message := range messages {
		message.Set("read", true)
		if err := app.Save(message); err != nil {
			log.Printf("Error marking message %s as read: %v\n", message.Id, err)
		}
	}

	return nil
}

// processMessages emails administrators a digest of unread non-administrator
// messages created within the last timeIntervalMinutes, grouped per congregation.
func processMessages(app core.App, timeIntervalMinutes int) error {
	log.Println("Starting messages processing")

	congregations := []CongregationData{}

	timeBuffer := time.Duration(-timeIntervalMinutes) * time.Minute

	err := app.DB().NewQuery("SELECT DISTINCT congregation FROM messages WHERE created > {:created} AND read = false AND type != 'administrator'").Bind(dbx.Params{"created": time.Now().UTC().Add(timeBuffer)}).All(&congregations)

	if err != nil {
		log.Println("Error fetching congregations:", err)
		return err
	}

	if len(congregations) == 0 {
		log.Println("Completed: No messages found")
		return nil
	}

	log.Printf("Processing %d congregations\n", len(congregations))

	for _, c := range congregations {
		err := processMessage(c.ID, app)
		if err != nil {
			log.Printf("Error processing congregation ID %s: %v\n", c.ID, err)
		}
	}

	log.Println("messages processing completed")
	return nil
}
