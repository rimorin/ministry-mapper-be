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

		// unprovisioned_since tracks when a user became role-less.
		// - For brand new accounts (never had a role) this field is left empty and
		//   the job falls back to `created` for the age calculation.
		// - When an existing user's last role is removed, an OnRecordAfterDeleteSuccess
		//   hook stamps this field to now so the user gets a fresh 7-day grace period
		//   instead of being immediately disabled based on their account creation date.
		if collection.Fields.GetByName("unprovisioned_since") == nil {
			collection.Fields.Add(&core.DateField{
				Name:     "unprovisioned_since",
				Required: false,
			})
		}

		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}

		collection.Fields.RemoveByName("unprovisioned_since")

		return app.Save(collection)
	})
}
