package migrations

import (
	"os"

	"github.com/getsentry/sentry-go"
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		superusers, err := app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
		if err != nil {
			sentry.CaptureException(err)
			return err
		}

		// Get email and password from environment variables
		email := os.Getenv("PB_ADMIN_EMAIL")
		if email == "" {
			email = "testing_account@ministry-mapper.com" // fallback value
		}

		password := os.Getenv("PB_ADMIN_PASSWORD")
		if password == "" {
			password = "pb123456789" // fallback value
		}

		// check if user already exists, if so, skip
		if _, err := app.FindAuthRecordByEmail(core.CollectionNameSuperusers, email); err == nil {
			return nil
		}

		record := core.NewRecord(superusers)

		record.Set("email", email)
		record.SetPassword(password)

		return app.Save(record)
	}, nil)
}
