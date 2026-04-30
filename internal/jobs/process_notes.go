package jobs

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"html/template"
	"time"

	"github.com/mailersend/mailersend-go"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/apis"
)

type notesData struct {
	Publisher string
	Message   string
	Date      string
	Address   string
}
type NotesTemplateData struct {
	Notes   []notesData
	Summary OverviewSummary
}

// BuildNotesPrompt constructs the system and user messages for the notes AI overview.
func BuildNotesPrompt(notes []notesData, congregationName string) (systemMsg, userMsg string) {
	systemMsg = `You are an assistant helping congregation administrators review a digest of ` +
		`recently updated property notes left by publishers during field ministry. ` +
		`These notes describe physical characteristics and conditions of the property ` +
		`(e.g. dogs, gate access, intercom, parking, renovations, unit vacancy). ` +
		`Analyse the notes and return a JSON object with exactly one field: ` +
		`"overview" (2-3 sentence narrative summarising the key property observations from this period). ` +
		`Be factual, concise, and respectful.`

	var sb strings.Builder
	sb.WriteString("Recent property notes for " + congregationName + " congregation:\n\n")
	for _, n := range notes {
		sb.WriteString("Address: " + n.Address + "\n")
		sb.WriteString("Publisher: " + n.Publisher + "\n")
		sb.WriteString("Date: " + n.Date + "\n")
		sb.WriteString("Note: " + n.Message + "\n\n")
	}
	userMsg = sb.String()
	return
}

// generateNotesAISummary builds an OverviewSummary from the notes list.
// Returns an empty OverviewSummary (Available=false) if AI is disabled or the call fails.
func generateNotesAISummary(notes []notesData, congregationName string) OverviewSummary {
	client := newLLMClient()
	if client == nil {
		log.Printf("AI overview skipped for notes (%s): OPENAI_API_KEY not set", congregationName)
		return OverviewSummary{}
	}

	systemMsg, userMsg := BuildNotesPrompt(notes, congregationName)
	resp, err := client.generateOverview(systemMsg, userMsg)
	if err != nil {
		log.Printf("AI overview: LLM call failed for notes (%s): %v", congregationName, err)
		return OverviewSummary{}
	}

	return OverviewSummary{
		Available: true,
		Overview:  resp.Overview,
	}
}

type CongregationData struct {
	ID string `db:"congregation"`
}

func ProcessNote(congID string, app core.App, timeBuffer time.Duration) error {
	log.Printf("Processing notes for congregation: %s", congID)

	if congID == "" {
		return apis.NewBadRequestError("Cong ID is required", nil)
	}

	congRecord, err := app.FindRecordById("congregations", congID)
	if err != nil {
		log.Println("Error finding congregation:", err)
	}

	notes, err := app.FindRecordsByFilter("addresses", "congregation = {:congregation} && last_notes_updated > {:created} && notes != NULL && notes != ''", "last_notes_updated", 0, 0, dbx.Params{"congregation": congID, "created": time.Now().UTC().Add(timeBuffer)})
	if err != nil {
		log.Println("Error finding notes by filter:", err)
		return err
	}

	app.ExpandRecords(notes, []string{"map"}, nil)

	if len(notes) == 0 {
		log.Println("No notes found")
		return nil
	}
	recipients := []Recipient{}

	err = app.DB().Select("users.*").From("users").InnerJoin("roles", dbx.NewExp("roles.user = users.id and roles.role = 'administrator'")).Where(dbx.NewExp("roles.congregation = {:congregation}", dbx.Params{"congregation": congID})).All(&recipients)

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
	tmpl, err := template.ParseFiles("templates/notes.html")
	if err != nil {
		log.Println("Error parsing template:", err)
		return err
	}

	// Prepare email data
	emailData := NotesTemplateData{
		Notes: make([]notesData, 0),
	}

	congregationTz := congRecord.Get("timezone").(string)

	location, err := time.LoadLocation(congregationTz)
	if err != nil {
		location = time.UTC // fallback
	}

	// Process notes
	for _, note := range notes {
		noteText := note.Get("notes").(string)
		if len(noteText) == 0 || strings.TrimSpace(noteText) == "" {
			continue
		}
		mapData := note.ExpandedOne("map")
		mapName := mapData.Get("description").(string)
		mapType := mapData.Get("type").(string)
		addressName := mapName + " # " + fmt.Sprintf("%.0f", note.Get("floor").(float64)) + " - " + note.Get("code").(string)
		if mapType == "single" {
			addressName = note.Get("code").(string) + " " + mapName
		}

		notesData := notesData{
			Address:   addressName,
			Publisher: note.Get("last_notes_updated_by").(string),
			Message:   noteText,
			Date:      note.GetDateTime("last_notes_updated").Time().In(location).Format("03:04 PM, 02 Jan 2006"),
		}
		emailData.Notes = append(emailData.Notes, notesData)
	}

	if IsAISummaryEnabled() {
		congregationName, _ := congRecord.Get("name").(string)
		emailData.Summary = generateNotesAISummary(emailData.Notes, congregationName)
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

	message.SetSubject("Notes updated for " + congRecord.Get("name").(string) + " - " + time.Now().Format("02 Jan 2006"))
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

// ProcessNotes processes notes for congregations that have been updated within a specified time interval.
// It fetches all congregations with recent notes updates and processes each one individually.
//
// Parameters:
// - app: A pointer to the PocketBase application instance.
// - timeIntervalMinutes: The time interval in minutes to look back for recent notes updates.
//
// Returns:
// - error: An error if there is an issue fetching or processing the notes, otherwise nil.
func ProcessNotes(app core.App, timeIntervalMinutes int) error {
	log.Println("Starting notes processing")

	congregations := []CongregationData{}

	timeBuffer := time.Duration(-timeIntervalMinutes) * time.Minute

	// Fetch all notes that have not been processed
	err := app.DB().NewQuery("SELECT DISTINCT congregation FROM addresses WHERE last_notes_updated > {:created} and notes IS NOT NULL and notes != ''").Bind(dbx.Params{"created": time.Now().UTC().Add(timeBuffer)}).All(&congregations)
	log.Printf("congregations: %v\n", congregations)
	if err != nil {
		log.Println("Error fetching maps:", err)
		return err
	}

	// if no messages found, return
	if len(congregations) == 0 {
		log.Println("Completed: No congregations with recent notes found")
		return nil
	}

	log.Printf("Processing %d congregation\n", len(congregations))

	for _, m := range congregations {
		log.Printf("Processing congregation ID %s\n", m)
		err := ProcessNote(m.ID, app, timeBuffer)
		if err != nil {
			log.Printf("Error processing congregation ID %s: %v\n", m.ID, err)
		}
	}

	log.Println("notes processing completed")
	return nil
}
