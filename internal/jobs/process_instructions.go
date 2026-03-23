package jobs

import (
	"bytes"
	"context"
	"html/template"
	"log"
	"os"
	"strings"
	"time"

	"github.com/mailersend/mailersend-go"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
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

func processInstruction(mapID string, app *pocketbase.PocketBase) error {
	log.Printf("Processing instructions for map: %s", mapID)

	if mapID == "" {
		return apis.NewBadRequestError("Map ID is required", nil)
	}

	// Get map record
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

	// Get instructions
	messages, err := app.FindRecordsByFilter("messages", "map = {:map} && pinned = true && type = 'administrator'", "created", 0, 0, dbx.Params{"map": mapID})
	if err != nil {
		log.Println("Error finding messages by filter:", err)
		return err
	}

	if len(messages) == 0 {
		log.Println("No instructions found")
		return nil
	}
	recipients := []Recipient{}

	err = app.DB().Select("users.*").From("users").InnerJoin("roles", dbx.NewExp("roles.user = users.id and roles.role != 'administrator'")).Where(dbx.NewExp("roles.congregation = {:congregation}", dbx.Params{"congregation": congregation})).All(&recipients)

	if err != nil {
		log.Println("Error fetching recipients:", err)
		return err
	}

	if len(recipients) == 0 {
		log.Println("No recipients found")
		return nil
	}
	log.Printf("Processing %d recipients\n", len(recipients))
	// Load template
	tmpl, err := template.ParseFiles("templates/instructions.html")
	if err != nil {
		log.Println("Error parsing template:", err)
		return err
	}

	// Prepare email data
	emailData := EmailTemplateData{
		Messages: make([]messagesData, 0),
		MapName:  territoryCode + " - " + mapRecord.Get("description").(string),
	}

	congregationTz := congRecord.Get("timezone").(string)

	location, err := time.LoadLocation(congregationTz)
	if err != nil {
		location = time.UTC // fallback
	}

	// Process instructions
	for _, messages := range messages {
		messagesData := messagesData{
			Publisher: messages.Get("created_by").(string),
			Message:   messages.Get("message").(string),
			Date:      messages.GetDateTime("created").Time().In(location).Format("03:04 PM, 02 Jan 2006"),
		}
		emailData.Messages = append(emailData.Messages, messagesData)
	}

	if IsAISummaryEnabled() {
		emailData.Summary = generateInstructionsAISummary(emailData.Messages, emailData.MapName)
	}

	// Execute template
	var body bytes.Buffer
	if err := tmpl.Execute(&body, emailData); err != nil {
		log.Println("Error executing template:", err)
		return err
	}

	// Initialize MailerSend
	ms := mailersend.NewMailersend(os.Getenv("MAILERSEND_API_KEY"))

	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Create email message
	message := ms.Email.NewMessage()

	message.SetFrom(mailersend.From{
		Email: os.Getenv("MAILERSEND_FROM_EMAIL"),
		Name:  "Ministry Mapper",
	})

	// Set recipients
	emailRecipents := []mailersend.Recipient{}
	for _, r := range recipients {
		emailRecipents = append(emailRecipents, mailersend.Recipient{
			Email: r.Email,
			Name:  r.Name,
		})
	}

	message.SetRecipients(emailRecipents)

	message.SetSubject("New instructions received for " + mapRecord.Get("description").(string))
	message.SetHTML(body.String())

	// Send email
	_, err = ms.Email.Send(ctx, message)
	if err != nil {
		log.Println("Error sending email:", err)
		return err
	}
	log.Println("Email sent successfully")
	return nil
}

func processInstructions(app *pocketbase.PocketBase, timeIntervalMinutes int) error {
	log.Println("Starting instructions processing")

	maps := []MapData{}

	// Calculate the time using the time interval
	timeBuffer := time.Duration(-timeIntervalMinutes) * time.Minute

	// Fetch all instructions that have not been processed
	err := app.DB().Select("maps.id").Distinct(true).From("maps").InnerJoin("messages", dbx.NewExp("messages.map = maps.id and messages.pinned = true and messages.type = 'administrator'")).Where(dbx.NewExp("messages.created > {:created}", dbx.Params{"created": time.Now().UTC().Add(timeBuffer)})).All(&maps)

	if err != nil {
		log.Println("Error fetching maps:", err)
		return err
	}

	// if no maps found, return
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
