package jobs

import (
	"bytes"
	"fmt"
	"html/template"
	"log"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
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
		return err
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

	tmpl, err := template.ParseFiles("templates/notes.html")
	if err != nil {
		log.Println("Error parsing template:", err)
		return err
	}

	emailData := NotesTemplateData{
		Notes: make([]notesData, 0),
	}

	location := loadCongregationLocation(congRecord)

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

	var body bytes.Buffer
	if err := tmpl.Execute(&body, emailData); err != nil {
		log.Println("Error executing template:", err)
		return err
	}

	subject := "Notes updated for " + congRecord.Get("name").(string) + " - " + time.Now().Format("02 Jan 2006")
	if err := sendHTMLEmail(recipients, subject, body.String()); err != nil {
		log.Println("Error sending email:", err)
		return err
	}
	log.Println("Email sent successfully")
	return nil
}

// ProcessNotes emails administrators a digest of notes updated within the last
// timeIntervalMinutes, grouped per congregation.
func ProcessNotes(app core.App, timeIntervalMinutes int) error {
	log.Println("Starting notes processing")

	congregations := []CongregationData{}

	timeBuffer := time.Duration(-timeIntervalMinutes) * time.Minute

	err := app.DB().NewQuery("SELECT DISTINCT congregation FROM addresses WHERE last_notes_updated > {:created} and notes IS NOT NULL and notes != ''").Bind(dbx.Params{"created": time.Now().UTC().Add(timeBuffer)}).All(&congregations)
	if err != nil {
		log.Println("Error fetching congregations:", err)
		return err
	}

	if len(congregations) == 0 {
		log.Println("Completed: No congregations with recent notes found")
		return nil
	}

	log.Printf("Processing %d congregation\n", len(congregations))

	for _, m := range congregations {
		err := ProcessNote(m.ID, app, timeBuffer)
		if err != nil {
			log.Printf("Error processing congregation ID %s: %v\n", m.ID, err)
		}
	}

	log.Println("notes processing completed")
	return nil
}
