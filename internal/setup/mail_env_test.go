//go:build testdata

package setup

import (
	"net/mail"
	"testing"

	"ganigeorgiev.com/pocketbase/pocketbase/tools/mailer"
)

// Emails sent through PocketBase's mailer (sign-in codes and the like) are
// marked outside production and left alone in it.
func TestMailerSubjectIsMarkedOutsideProduction(t *testing.T) {
	cases := map[string]string{
		"staging":    "[STAGING] Your code",
		"production": "Your code",
		"":           "Your code",
	}
	for env, want := range cases {
		t.Setenv("APP_ENV", env)
		app := setupTestApp(t)
		message := &mailer.Message{
			From:    mail.Address{Address: "from@one.test"},
			To:      []mail.Address{{Address: "to@two.test"}},
			Subject: "Your code",
			HTML:    "<p>123</p>",
		}
		if err := app.NewMailClient().Send(message); err != nil {
			t.Fatal(err)
		}
		if got := app.TestMailer.LastMessage().Subject; got != want {
			t.Errorf("APP_ENV=%q: subject = %q, want %q", env, got, want)
		}
		if got := len(app.TestMailer.LastMessage().To); got != 1 {
			t.Errorf("APP_ENV=%q: %d recipients, want 1", env, got)
		}
		app.Cleanup()
	}
}
