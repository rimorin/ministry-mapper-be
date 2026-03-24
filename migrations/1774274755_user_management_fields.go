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

		// Add warning timestamp fields for automated user lifecycle management.
		// These prevent duplicate emails from being sent on each daily job run.
		fieldsToAdd := []struct {
			name string
		}{
			{"inactive_warning_sent_at"},
			{"inactive_final_warning_sent_at"},
			{"unprovisioned_warning_sent_at"},
			{"unprovisioned_final_warning_sent_at"},
		}

		for _, f := range fieldsToAdd {
			if collection.Fields.GetByName(f.name) == nil {
				collection.Fields.Add(&core.DateField{
					Name:     f.name,
					Required: false,
				})
			}
		}

		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}

		for _, name := range []string{
			"inactive_warning_sent_at",
			"inactive_final_warning_sent_at",
			"unprovisioned_warning_sent_at",
			"unprovisioned_final_warning_sent_at",
		} {
			collection.Fields.RemoveByName(name)
		}

		return app.Save(collection)
	})
}
