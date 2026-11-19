package migrations

import (
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	m "ganigeorgiev.com/pocketbase/pocketbase/migrations"
)

// Remembers which account a conductor meant by a name they typed to the
// assistant, so "John" is asked about once per congregation and not on every
// link. alias is the name reduced to its words; one account per alias.
//
// Rules stay nil (superuser only); the assistant routes read and write it.
func init() {
	m.Register(func(app core.App) error {
		if _, err := app.FindCollectionByNameOrId("assistant_name_links"); err == nil {
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
		links := core.NewBaseCollection("assistant_name_links")
		links.Fields.Add(
			&core.RelationField{Name: "congregation", CollectionId: congregations.Id, MaxSelect: 1, Required: true, CascadeDelete: true},
			&core.TextField{Name: "alias", Required: true, Max: 200},
			&core.RelationField{Name: "user", CollectionId: users.Id, MaxSelect: 1, Required: true, CascadeDelete: true},
		)
		links.AddIndex("idx_assistant_name_links_alias", true, "`congregation`, `alias`", "")
		return app.Save(links)
	}, func(app core.App) error {
		links, err := app.FindCollectionByNameOrId("assistant_name_links")
		if err != nil {
			return nil
		}
		return app.Delete(links)
	})
}
