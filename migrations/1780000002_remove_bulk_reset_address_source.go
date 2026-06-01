package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		// Clear any addresses that still carry source="bulk_reset" from resets
		// that ran before hook suppression was moved to app.Store().
		if _, err := app.DB().NewQuery(
			"UPDATE addresses SET source = '' WHERE source = 'bulk_reset'",
		).Execute(); err != nil {
			return err
		}

		collection, err := app.FindCollectionByNameOrId("addresses")
		if err != nil {
			return err
		}

		return setAddressSourceValues(app, collection, []string{"app", "admin", "map_init", "floor_copy"})
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("addresses")
		if err != nil {
			return err
		}

		return setAddressSourceValues(app, collection, []string{"app", "admin", "map_init", "floor_copy", "bulk_reset"})
	})
}
