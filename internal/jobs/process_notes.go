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

type notesData struct {
	Publisher string
	Message   string
	Date      string
	Address   string
}
type NotesTemplateData struct {
	emailChrome
	Notes   []notesData
	Summary OverviewSummary
}

// BuildNotesPrompt constructs the system and user messages for the notes AI overview.
func BuildNotesPrompt(notes []notesData, congregationName string) (systemMsg, userMsg string) {
	systemMsg = `You write a short summary for congregation administrators of the property notes
below, left by publishers in the house-to-house ministry. Notes describe things like dogs,
gates, intercoms, parking, renovations and empty units. The full list is shown under your
text, so say what stands out; do not repeat every note.

Return JSON with exactly one field: "overview" (at most 2 sentences).

` + plainLanguageRules

	var sb strings.Builder
	sb.WriteString("Recent property notes for " + congregationName + " congregation:\n\n")
	for _, n := range notes {
		sb.WriteString("Address: " + n.Address + "\n")
		sb.WriteString("Date: " + n.Date + "\n")
		sb.WriteString("Note: " + n.Message + "\n\n")
	}
	userMsg = sb.String()
	return
}

// generateNotesAISummary builds an OverviewSummary from the notes list.
// Returns an empty OverviewSummary (Available=false) if AI is disabled or the call fails.
func generateNotesAISummary(notes []notesData, congregationName string) OverviewSummary {
	if len(notes) < overviewMinItems {
		return OverviewSummary{}
	}
	client := newLLMClient()
	if client == nil {
		log.Printf("AI overview skipped for notes (%s): OPENAI_API_KEY not set", congregationName)
		return OverviewSummary{}
	}

	systemMsg, userMsg := BuildNotesPrompt(notes, congregationName)
	resp, err := client.generateOverview(systemMsg, userMsg, overviewOnlySchema)
	if err != nil {
		log.Printf("AI overview: LLM call failed for notes (%s): %v", congregationName, err)
		return OverviewSummary{}
	}
	if err := groundedNumbers(userMsg, resp.Overview); err != nil {
		log.Printf("AI overview dropped for notes (%s): %v", congregationName, err)
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

	congregationName, _ := congRecord.Get("name").(string)
	if IsAISummaryEnabled() {
		emailData.Summary = generateNotesAISummary(emailData.Notes, congregationName)
	}

	count := len(emailData.Notes)
	emailData.emailChrome = emailChrome{
		Preheader:   fmt.Sprintf("%s across your maps.", pluralize(count, "note")),
		Kicker:      congregationName,
		Title:       "New notes from the field",
		Subtitle:    fmt.Sprintf("%s updated · %s", pluralize(count, "note"), time.Now().In(location).Format("2 Jan 2006")),
		ButtonLabel: "Open Ministry Mapper",
		ButtonURL:   os.Getenv("PB_APP_URL"),
		Footer:      fmt.Sprintf("Sent to administrators of %s when publishers update notes on their maps.", congregationName),
	}
	htmlBody, textBody, err := renderEmail("notes.html", emailData)
	if err != nil {
		log.Println("Error rendering notes email:", err)
		return err
	}

	subject := fmt.Sprintf("%s: %s updated", congregationName, pluralize(count, "note"))
	if err := sendHTMLEmail(recipients, subject, htmlBody, textBody); err != nil {
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
