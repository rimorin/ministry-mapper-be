package jobs

import (
	"bytes"
	"context"
	"log"
	"os"
	"strings"
	"html/template"
	"time"

	"github.com/mailersend/mailersend-go"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
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

// Recipient holds the name and email of an email recipient.
type Recipient struct {
	Name  string `db:"name"`
	Email string `db:"email"`
}

func processMessage(mapID string, app *pocketbase.PocketBase) error {
	log.Printf("Processing messages for map: %s", mapID)

	if mapID == "" {
		return apis.NewBadRequestError("Map ID is required", nil)
	}

	// Get map record to get publisher info
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

	// Get messages
	messages, err := app.FindRecordsByFilter("messages", "map = {:map} && read = false && type != 'administrator'", "created", 0, 0, dbx.Params{"map": mapID})
	if err != nil {
		log.Println("Error finding messages by filter:", err)
		return err
	}

	if len(messages) == 0 {
		log.Println("No messages found")
		return nil
	}
	recipients := []Recipient{}

	err = app.DB().Select("users.*").From("users").InnerJoin("roles", dbx.NewExp("roles.user = users.id and roles.role = 'administrator'")).Where(dbx.NewExp("roles.congregation = {:congregation}", dbx.Params{"congregation": congregation})).All(&recipients)

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
	tmpl, err := template.ParseFiles("templates/messages.html")
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

	// Process messages
	for _, messages := range messages {
		messagesData := messagesData{
			Publisher: messages.Get("created_by").(string),
			Message:   messages.Get("message").(string),
			Date:      messages.GetDateTime("created").Time().In(location).Format("03:04 PM, 02 Jan 2006"),
		}
		emailData.Messages = append(emailData.Messages, messagesData)
	}

	if IsAISummaryEnabled() {
		emailData.Summary = generateMessagesAISummary(emailData.Messages, emailData.MapName)
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

	message.SetSubject("New messages received for " + mapRecord.Get("description").(string))
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

// processMessages processes unread messages within a specified time interval.
// It fetches distinct map IDs associated with unread messages that are not from administrators
// and processes each map's messages.
//
// Parameters:
//   - app: A pointer to the PocketBase application instance.
//   - timeIntervalMinutes: The time interval in minutes to look back for unread messages.
//
// Returns:
//   - error: An error if there is an issue fetching or processing messages, otherwise nil.
func processMessages(app *pocketbase.PocketBase, timeIntervalMinutes int) error {
	log.Println("Starting messages processing")

	maps := []MapData{}

	// Calculate the time using the time interval
	timeBuffer := time.Duration(-timeIntervalMinutes) * time.Minute

	// Fetch all messages that have not been processed
	err := app.DB().Select("maps.id").Distinct(true).From("maps").InnerJoin("messages", dbx.NewExp("messages.map = maps.id and messages.read = false and messages.type != 'administrator'")).Where(dbx.NewExp("messages.created > {:created}", dbx.Params{"created": time.Now().UTC().Add(timeBuffer)})).All(&maps)

	if err != nil {
		log.Println("Error fetching maps:", err)
		return err
	}

	// if no messages found, return
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
