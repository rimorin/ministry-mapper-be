package migrations

import (
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	m "ganigeorgiev.com/pocketbase/pocketbase/migrations"
)

// Adds old_tries/new_tries to addresses_log. Existing rows are left at 0: the
// tries were never logged and can't be reconstructed. A not_home → not_home row
// with 0/0 is therefore a pre-migration row, since new ones always differ.
func init() {
	m.Register(func(app core.App) error {
		return addFieldsIfMissing(app, "addresses_log",
			&core.NumberField{Name: "old_tries", OnlyInt: true},
			&core.NumberField{Name: "new_tries", OnlyInt: true},
		)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("addresses_log")
		if err != nil {
			return nil
		}
		collection.Fields.RemoveByName("old_tries")
		collection.Fields.RemoveByName("new_tries")
		return app.Save(collection)
	})
}
