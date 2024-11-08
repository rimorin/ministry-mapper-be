package jobs

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"text/template"
	"time"

	"github.com/mailersend/mailersend-go"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
)

type notesData struct {
	Publisher string
	Message   string
	Date      string
	Address   string
}
type NotesTemplateData struct {
	Notes []notesData
}

type CongregationData struct {
	ID string `db:"congregation"`
}

// ProcessNote processes notes for a given congregation, sends an email with the notes to the administrators of the congregation.
//
// Parameters:
//   - congID: The ID of the congregation.
//   - app: The PocketBase application instance.
//   - timeBuffer: The duration to filter notes that were updated after the current time minus the timeBuffer.
//
// Returns:
//   - error: An error if something goes wrong, otherwise nil.
//
// The function performs the following steps:
//  1. Logs the start of the note processing for the given congregation.
//  2. Validates the congID parameter.
//  3. Retrieves the congregation record by ID.
//  4. Finds notes for the congregation that were updated after the specified time buffer.
//  5. Expands the notes with related map data.
//  6. Retrieves the list of administrator recipients for the congregation.
//  7. Loads the email template.
//  8. Prepares the email data with the notes information.
//  9. Executes the email template with the prepared data.
//  10. Initializes the MailerSend client and creates the email message.
//  11. Sets the email recipients and subject.
//  12. Sends the email with the notes to the recipients.
//  13. Logs the success or failure of the email sending process.
func ProcessNote(congID string, app *pocketbase.PocketBase, timeBuffer time.Duration) error {
	log.Printf("Processing notes for congregation: %s", congID)

	if congID == "" {
		return apis.NewBadRequestError("Cong ID is required", nil)
	}

	congRecord, err := app.FindRecordById("congregations", congID)
	if err != nil {
		log.Println("Error finding congregation:", err)
	}

	notes, err := app.FindRecordsByFilter("addresses", "congregation = {:congregation} && last_notes_updated > {:created}", "last_notes_updated", 0, 0, dbx.Params{"congregation": congID, "created": time.Now().UTC().Add(timeBuffer)})
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
			Message:   note.Get("notes").(string),
			Date:      note.GetDateTime("last_notes_updated").Time().In(location).Format("03:04 PM, 02 Jan 2006"),
		}
		emailData.Notes = append(emailData.Notes, notesData)
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
func ProcessNotes(app *pocketbase.PocketBase, timeIntervalMinutes int) error {
	log.Println("Starting notes processing")

	congregations := []CongregationData{}

	timeBuffer := time.Duration(-timeIntervalMinutes) * time.Minute

	// Fetch all notes that have not been processed
	err := app.DB().NewQuery("SELECT DISTINCT congregation FROM addresses WHERE last_notes_updated > {:created}").Bind(dbx.Params{"created": time.Now().UTC().Add(timeBuffer)}).All(&congregations)
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
