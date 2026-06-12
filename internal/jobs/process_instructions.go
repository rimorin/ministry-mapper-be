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

// BuildInstructionsPrompt constructs the system and user messages for the instructions AI overview.
func BuildInstructionsPrompt(messages []messagesData, mapName string) (systemMsg, userMsg string) {
	systemMsg = `You are an assistant helping congregation publishers understand instructions ` +
		`from their administrator about their assigned territory map. ` +
		`These instructions may include directives, special conditions, access notes, ` +
		`or other guidance the administrator wants publishers to follow. ` +
		`Analyse the instructions and return a JSON object with exactly one field: ` +
		`"overview" (2-3 sentence narrative summarising the key directives and what publishers need to do or be aware of). ` +
		`Be factual, concise, and respectful.`

	var sb strings.Builder
	sb.WriteString("Administrator instructions for territory map " + mapName + ":\n\n")
	for _, m := range messages {
		sb.WriteString("From: " + m.Publisher + "\n")
		sb.WriteString("Date: " + m.Date + "\n")
		sb.WriteString("Instruction: " + m.Message + "\n\n")
	}
	userMsg = sb.String()
	return
}

// generateInstructionsAISummary builds an OverviewSummary from the instructions list.
// Returns an empty OverviewSummary (Available=false) if AI is disabled or the call fails.
func generateInstructionsAISummary(messages []messagesData, mapName string) OverviewSummary {
	client := newLLMClient()
	if client == nil {
		log.Printf("AI overview skipped for instructions (%s): OPENAI_API_KEY not set", mapName)
		return OverviewSummary{}
	}

	systemMsg, userMsg := BuildInstructionsPrompt(messages, mapName)
	resp, err := client.generateOverview(systemMsg, userMsg)
	if err != nil {
		log.Printf("AI overview: LLM call failed for instructions (%s): %v", mapName, err)
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

	tmpl, err := template.ParseFiles("templates/instructions.html")
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
		emailData.Summary = generateInstructionsAISummary(emailData.Messages, emailData.MapName)
	}

	var body bytes.Buffer
	if err := tmpl.Execute(&body, emailData); err != nil {
		log.Println("Error executing template:", err)
		return err
	}

	subject := "New instructions received for " + mapRecord.Get("description").(string)
	if err := sendHTMLEmail(recipients, subject, body.String()); err != nil {
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
