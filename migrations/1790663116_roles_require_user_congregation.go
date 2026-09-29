package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Deletes roles missing a user or congregation, then makes both required.
func init() {
	m.Register(func(app core.App) error {
		if _, err := app.DB().NewQuery(
			"DELETE FROM roles WHERE COALESCE(user, '') = '' OR COALESCE(congregation, '') = ''",
		).Execute(); err != nil {
			return err
		}
		return setRolesRelationsRequired(app, true)
	}, func(app core.App) error {
		return setRolesRelationsRequired(app, false)
	})
}

func setRolesRelationsRequired(app core.App, required bool) error {
	collection, err := app.FindCollectionByNameOrId("roles")
	if err != nil {
		return nil
	}
	for _, name := range []string{"user", "congregation"} {
		if field, ok := collection.Fields.GetByName(name).(*core.RelationField); ok {
			field.Required = required
		}
	}
	return app.Save(collection)
}
