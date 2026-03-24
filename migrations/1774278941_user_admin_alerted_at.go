package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}

		// admin_alerted_at tracks whether congregation admins have been notified about
		// this unprovisioned account. Acts as an idempotency guard — prevents double-alerts
		// from daily job timing drift and missed-alert gaps when the job is down.
		if collection.Fields.GetByName("admin_alerted_at") == nil {
			collection.Fields.Add(&core.DateField{
				Name:     "admin_alerted_at",
				Required: false,
			})
		}

		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}

		collection.Fields.RemoveByName("admin_alerted_at")

		return app.Save(collection)
	})
}
