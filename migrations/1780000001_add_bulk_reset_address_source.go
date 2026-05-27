package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("addresses")
		if err != nil {
			return nil
		}

		return setAddressSourceValues(app, collection, []string{"app", "admin", "map_init", "floor_copy", "bulk_reset"})
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("addresses")
		if err != nil {
			return nil
		}

		return setAddressSourceValues(app, collection, []string{"app", "admin", "map_init", "floor_copy"})
	})
}

func setAddressSourceValues(app core.App, collection *core.Collection, values []string) error {
	field := collection.Fields.GetByName("source")
	if field == nil {
		return fmt.Errorf("addresses.source field not found")
	}

	selectField, ok := field.(*core.SelectField)
	if !ok {
		return fmt.Errorf("addresses.source field is %T, want *core.SelectField", field)
	}

	selectField.Values = append([]string(nil), values...)

	return app.Save(collection)
}
