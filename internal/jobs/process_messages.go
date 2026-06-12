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
}
type EmailTemplateData struct {
	Messages []messagesData
	MapName  string
	Summary  OverviewSummary
}

// BuildMessagesPrompt constructs the system and user messages for the messages AI overview.
func BuildMessagesPrompt(messages []messagesData, mapName string) (systemMsg, userMsg string) {
	systemMsg = `You are an assistant helping congregation administrators review a digest of ` +
		`recently received feedback messages from publishers about their assigned map territory. ` +
		`Messages may cover topics like map boundaries, access difficulties, special conditions, ` +
		`coordination requests, territory observations, or address corrections such as missing ` +
		`house numbers and incorrect unit details. ` +
		`Analyse the messages and return a JSON object with exactly two fields: ` +
		`"overview" (2-3 sentence narrative summarising the recent publisher feedback about the map) and ` +
		`"key_themes" (1-2 sentences identifying the main concerns or action items administrators ` +
		`should address, including any address data corrections needed). ` +
		`Be factual, concise, and respectful.`

	var sb strings.Builder
	sb.WriteString("Recent publisher feedback for map " + mapName + ":\n\n")
	for _, m := range messages {
		sb.WriteString("Publisher: " + m.Publisher + "\n")
		sb.WriteString("Date: " + m.Date + "\n")
		sb.WriteString("Message: " + m.Message + "\n\n")
	}
	userMsg = sb.String()
	return
}

// generateMessagesAISummary builds an OverviewSummary from the messages list.
// Returns an empty OverviewSummary (Available=false) if AI is disabled or the call fails.
func generateMessagesAISummary(messages []messagesData, mapName string) OverviewSummary {
	client := newLLMClient()
	if client == nil {
		log.Printf("AI overview skipped for messages (%s): OPENAI_API_KEY not set", mapName)
		return OverviewSummary{}
	}

	systemMsg, userMsg := BuildMessagesPrompt(messages, mapName)
	resp, err := client.generateOverview(systemMsg, userMsg)
	if err != nil {
		log.Printf("AI overview: LLM call failed for messages (%s): %v", mapName, err)
		return OverviewSummary{}
	}

	return OverviewSummary{
		Available: true,
		Overview:  resp.Overview,
		KeyThemes: resp.KeyThemes,
	}
}

// MapData holds a map ID for batch processing.
type MapData struct {
	ID string `db:"id"`
}

func processMessage(mapID string, app core.App) error {
	log.Printf("Processing messages for map: %s", mapID)

	if mapID == "" {
		return apis.NewBadRequestError("Map ID is required", nil)
	}

	mapRecord, err := app.FindRecordById("maps", mapID)
	if err != nil {
		log.Println("Error finding map:", err)
		return err
	}

	congregation := mapRecord.Get("congregation").(string)

	congRecord, err := app.FindRecordById("congregations", congregation)
	if err != nil {
		log.Println("Error finding congregation:", err)
		return err
	}

	territoryRecord, err := app.FindRecordById("territories", mapRecord.Get("territory").(string))

	if err != nil {
		log.Println("Error finding territory:", err)
		return err
	}

	territoryCode := territoryRecord.Get("code").(string)

	messages, err := app.FindRecordsByFilter("messages", "map = {:map} && read = false && type != 'administrator'", "created", 0, 0, dbx.Params{"map": mapID})
	if err != nil {
		log.Println("Error finding messages by filter:", err)
		return err
	}

	if len(messages) == 0 {
		log.Println("No messages found")
		return nil
	}

	recipients, err := fetchCongregationRecipients(app, congregation, true)
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
		MapName:  territoryCode + " - " + mapRecord.Get("description").(string),
	}

	location := loadCongregationLocation(congRecord)

	for _, message := range messages {
		emailData.Messages = append(emailData.Messages, messagesData{
			Publisher: message.Get("created_by").(string),
			Message:   message.Get("message").(string),
			Date:      message.GetDateTime("created").Time().In(location).Format("03:04 PM, 02 Jan 2006"),
		})
	}

	if IsAISummaryEnabled() {
		emailData.Summary = generateMessagesAISummary(emailData.Messages, emailData.MapName)
	}

	var body bytes.Buffer
	if err := tmpl.Execute(&body, emailData); err != nil {
		log.Println("Error executing template:", err)
		return err
	}

	subject := "New messages received for " + mapRecord.Get("description").(string)
	if err := sendHTMLEmail(recipients, subject, body.String()); err != nil {
		log.Println("Error sending email:", err)
		return err
	}
	log.Println("Email sent successfully")
	return nil
}

// processMessages emails administrators a digest of unread non-administrator
// messages created within the last timeIntervalMinutes, grouped per map.
func processMessages(app core.App, timeIntervalMinutes int) error {
	log.Println("Starting messages processing")

	maps := []MapData{}

	timeBuffer := time.Duration(-timeIntervalMinutes) * time.Minute

	err := app.DB().Select("maps.id").Distinct(true).From("maps").InnerJoin("messages", dbx.NewExp("messages.map = maps.id and messages.read = false and messages.type != 'administrator'")).Where(dbx.NewExp("messages.created > {:created}", dbx.Params{"created": time.Now().UTC().Add(timeBuffer)})).All(&maps)

	if err != nil {
		log.Println("Error fetching maps:", err)
		return err
	}

	if len(maps) == 0 {
		log.Println("Completed: No messages found")
		return nil
	}

	log.Printf("Processing %d maps\n", len(maps))

	for _, m := range maps {
		err := processMessage(m.ID, app)
		if err != nil {
			log.Printf("Error processing map ID %s: %v\n", m.ID, err)
		}
	}

	log.Println("messages processing completed")
	return nil
}
