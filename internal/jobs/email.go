package jobs

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"ministry-mapper/internal/appenv"

	"ganigeorgiev.com/pocketbase/dbx"
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"github.com/mailersend/mailersend-go"
)

// Recipient holds the name and email of an email recipient.
type Recipient struct {
	Name  string `db:"name"`
	Email string `db:"email"`
}

// fetchCongregationRecipients returns the congregation's administrators
// (adminOnly) or its conductors and read-only users (!adminOnly). Publishers
// are never emailed: they read instructions on the map they are given.
func fetchCongregationRecipients(app core.App, congregationId string, adminOnly bool) ([]Recipient, error) {
	roleCond := "roles.role = 'administrator'"
	if !adminOnly {
		roleCond = "roles.role IN ('conductor', 'read_only')"
	}
	recipients := []Recipient{}
	err := app.DB().Select("users.*").From("users").
		InnerJoin("roles", dbx.NewExp("roles.user = users.id and "+roleCond)).
		Where(dbx.NewExp("roles.congregation = {:congregation}", dbx.Params{"congregation": congregationId})).
		All(&recipients)
	return recipients, err
}

// loadCongregationLocation returns the congregation's timezone, falling back to UTC.
func loadCongregationLocation(congRecord *core.Record) *time.Location {
	tz, _ := congRecord.Get("timezone").(string)
	location, err := time.LoadLocation(tz)
	if err != nil {
		return time.UTC
	}
	return location
}

// sendHTMLEmail sends one email via MailerSend with an HTML body and its plain-text
// alternative. It's a package-level var so tests can substitute a stub.
var sendHTMLEmail = func(recipients []Recipient, subject, htmlBody, textBody string) error {
	return deliver(recipients, subject, htmlBody, textBody)
}

func allowedRecipients(recipients []Recipient) []Recipient {
	allowed := make([]Recipient, 0, len(recipients))
	for _, r := range recipients {
		if appenv.MayEmail(r.Email) {
			allowed = append(allowed, r)
		}
	}
	return allowed
}

// deliver is the only path to MailerSend. Outside production it marks the
// subject and applies the allow-list.
func deliver(recipients []Recipient, subject, htmlBody, textBody string, attachments ...mailersend.Attachment) error {
	apiKey := os.Getenv("MAILERSEND_API_KEY")
	fromEmail := os.Getenv("MAILERSEND_FROM_EMAIL")
	if apiKey == "" || fromEmail == "" {
		return fmt.Errorf("MAILERSEND_API_KEY or MAILERSEND_FROM_EMAIL not configured")
	}

	total := len(recipients)
	recipients = allowedRecipients(recipients)
	if len(recipients) == 0 {
		log.Printf("Email held back in %s: none of its %d recipients is in EMAIL_ALLOWLIST", appenv.Name(), total)
		return nil
	}

	ms := mailersend.NewMailersend(apiKey)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	emailRecipients := make([]mailersend.Recipient, 0, len(recipients))
	for _, r := range recipients {
		emailRecipients = append(emailRecipients, mailersend.Recipient{Email: r.Email, Name: r.Name})
	}

	message := ms.Email.NewMessage()
	message.SetFrom(mailersend.From{Email: fromEmail, Name: "Ministry Mapper"})
	message.SetRecipients(emailRecipients)
	message.SetSubject(appenv.MarkSubject(subject))
	message.SetHTML(htmlBody)
	if textBody != "" {
		message.SetText(textBody)
	}
	for _, attachment := range attachments {
		message.AddAttachment(attachment)
	}

	_, err := ms.Email.Send(ctx, message)
	return err
}
