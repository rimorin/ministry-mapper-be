package jobs

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/mailersend/mailersend-go"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// Recipient holds the name and email of an email recipient.
type Recipient struct {
	Name  string `db:"name"`
	Email string `db:"email"`
}

// fetchCongregationRecipients returns the users holding (adminOnly) or not holding
// (!adminOnly) the administrator role in the given congregation.
func fetchCongregationRecipients(app core.App, congregationId string, adminOnly bool) ([]Recipient, error) {
	roleCond := "roles.role = 'administrator'"
	if !adminOnly {
		roleCond = "roles.role != 'administrator'"
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

// sendHTMLEmail sends a single HTML email via MailerSend to the given recipients.
// It's a package-level var so tests can substitute a stub instead of sending real email.
var sendHTMLEmail = func(recipients []Recipient, subject, htmlBody string) error {
	apiKey := os.Getenv("MAILERSEND_API_KEY")
	fromEmail := os.Getenv("MAILERSEND_FROM_EMAIL")
	if apiKey == "" || fromEmail == "" {
		return fmt.Errorf("MAILERSEND_API_KEY or MAILERSEND_FROM_EMAIL not configured")
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
	message.SetSubject(subject)
	message.SetHTML(htmlBody)

	_, err := ms.Email.Send(ctx, message)
	return err
}
