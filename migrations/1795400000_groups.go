package migrations

import (
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	m "ganigeorgiev.com/pocketbase/pocketbase/migrations"
	"ganigeorgiev.com/pocketbase/pocketbase/tools/types"
)

// A congregation's groups (field service groups, a language group, visitors
// supporting it) narrow the member picker to the people a conductor works
// with. They grant nothing. Rules stay basic; the hooks in
// internal/handlers scope them like territories.
func init() {
	m.Register(func(app core.App) error {
		if _, err := app.FindCollectionByNameOrId("groups"); err == nil {
			return nil
		}
		congregations, err := app.FindCollectionByNameOrId("congregations")
		if err != nil {
			return err
		}
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		groups := core.NewBaseCollection("groups")
		groups.ListRule = types.Pointer(`@request.auth.id != ""`)
		groups.ViewRule = types.Pointer(`@request.auth.id != ""`)
		groups.CreateRule = types.Pointer(`@request.auth.id != ""`)
		groups.UpdateRule = types.Pointer(`@request.auth.id != ""`)
		groups.DeleteRule = types.Pointer(`@request.auth.id != ""`)
		groups.Fields.Add(
			&core.RelationField{Name: "congregation", CollectionId: congregations.Id, MaxSelect: 1, Required: true, CascadeDelete: true},
			&core.TextField{Name: "name", Required: true, Max: 50},
			&core.RelationField{Name: "members", CollectionId: users.Id, MaxSelect: 1000},
		)
		groups.AddIndex("idx_groups_name", true, "`congregation`, `name`", "")
		return app.Save(groups)
	}, func(app core.App) error {
		groups, err := app.FindCollectionByNameOrId("groups")
		if err != nil {
			return nil
		}
		return app.Delete(groups)
	})
}
