package jobs

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// BuildInstructionsPrompt constructs the system and user messages for the instructions AI overview.
func BuildInstructionsPrompt(messages []messagesData, mapName string) (systemMsg, userMsg string) {
	systemMsg = `You write a short summary for publishers of the administrator's instructions below
for one territory map. Publishers read it on a phone before working the map. The full
instructions are shown under your text.

Return JSON with exactly one field: "overview" (at most 2 sentences on what publishers
must do or watch out for).

` + plainLanguageRules

	var sb strings.Builder
	sb.WriteString("Administrator instructions for territory map " + mapName + ":\n\n")
	for _, m := range messages {
		sb.WriteString("Date: " + m.Date + "\n")
		sb.WriteString("Instruction: " + m.Message + "\n\n")
	}
	userMsg = sb.String()
	return
}

// generateInstructionsAISummary builds an OverviewSummary from the instructions list.
// Returns an empty OverviewSummary (Available=false) if AI is disabled or the call fails.
func generateInstructionsAISummary(messages []messagesData, mapName string) OverviewSummary {
	if len(messages) < overviewMinItems {
		return OverviewSummary{}
	}
	client := newLLMClient()
	if client == nil {
		log.Printf("AI overview skipped for instructions (%s): OPENAI_API_KEY not set", mapName)
		return OverviewSummary{}
	}

	systemMsg, userMsg := BuildInstructionsPrompt(messages, mapName)
	resp, err := client.generateOverview(systemMsg, userMsg, overviewOnlySchema)
	if err != nil {
		log.Printf("AI overview: LLM call failed for instructions (%s): %v", mapName, err)
		return OverviewSummary{}
	}
	if err := groundedNumbers(userMsg, resp.Overview); err != nil {
		log.Printf("AI overview dropped for instructions (%s): %v", mapName, err)
		return OverviewSummary{}
	}

	return OverviewSummary{
		Available: true,
		Overview:  resp.Overview,
	}
}

func processInstruction(mapID string, app core.App) error {
	log.Printf("Processing instructions for map: %s", mapID)

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

	messages, err := app.FindRecordsByFilter("messages", "map = {:map} && pinned = true && type = 'administrator'", "created", 0, 0, dbx.Params{"map": mapID})
	if err != nil {
		log.Println("Error finding messages by filter:", err)
		return err
	}

	if len(messages) == 0 {
		log.Println("No instructions found")
		return nil
	}

	recipients, err := fetchCongregationRecipients(app, congregation, false)
	if err != nil {
		log.Println("Error fetching recipients:", err)
		return err
	}

	if len(recipients) == 0 {
		log.Println("No recipients found")
		return nil
	}
	log.Printf("Processing %d recipients\n", len(recipients))

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
		emailData.Summary = generateInstructionsAISummary(emailData.Messages, emailData.MapName)
	}

	mapDescription := mapRecord.Get("description").(string)
	emailData.emailChrome = emailChrome{
		Preheader:   fmt.Sprintf("%s for %s.", pluralize(len(emailData.Messages), "instruction"), mapDescription),
		Kicker:      congRecord.Get("name").(string),
		Title:       "Instructions for your map",
		Subtitle:    emailData.MapName,
		ButtonLabel: "Open the map",
		ButtonURL:   os.Getenv("PB_APP_URL"),
		Footer:      "Sent to everyone in the congregation when an administrator pins instructions to a map.",
	}
	htmlBody, textBody, err := renderEmail("instructions.html", emailData)
	if err != nil {
		log.Println("Error rendering instructions email:", err)
		return err
	}

	subject := "Instructions for " + mapDescription
	if err := sendHTMLEmail(recipients, subject, htmlBody, textBody); err != nil {
		log.Println("Error sending email:", err)
		return err
	}
	log.Println("Email sent successfully")
	return nil
}

// processInstructions emails publishers the pinned administrator instructions
// created within the last timeIntervalMinutes, grouped per map.
func processInstructions(app core.App, timeIntervalMinutes int) error {
	log.Println("Starting instructions processing")

	maps := []MapData{}

	timeBuffer := time.Duration(-timeIntervalMinutes) * time.Minute

	err := app.DB().Select("maps.id").Distinct(true).From("maps").InnerJoin("messages", dbx.NewExp("messages.map = maps.id and messages.pinned = true and messages.type = 'administrator'")).Where(dbx.NewExp("messages.created > {:created}", dbx.Params{"created": time.Now().UTC().Add(timeBuffer)})).All(&maps)

	if err != nil {
		log.Println("Error fetching maps:", err)
		return err
	}

	if len(maps) == 0 {
		log.Println("Completed: No maps found in the time interval")
		return nil
	}

	log.Printf("Processing %d maps\n", len(maps))

	for _, m := range maps {
		err := processInstruction(m.ID, app)
		if err != nil {
			log.Printf("Error processing map ID %s: %v\n", m.ID, err)
		}
	}

	log.Println("instructions processing completed")
	return nil
}
