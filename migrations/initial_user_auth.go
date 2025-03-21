package migrations

import (
	"os"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}

		collection.OTP.Enabled = os.Getenv("PB_OTP_ENABLED") == "true"
		collection.MFA.Enabled = os.Getenv("PB_MFA_ENABLED") == "true"

		return app.Save(collection)
	}, nil)
}
