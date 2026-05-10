package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		usersCol, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		congregationsCol, err := app.FindCollectionByNameOrId("congregations")
		if err != nil {
			return err
		}
		mapsCol, err := app.FindCollectionByNameOrId("maps")
		if err != nil {
			return err
		}

		// Create assignments_log
		aLog := core.NewBaseCollection("assignments_log")
		aLog.Fields.Add(
			&core.TextField{Name: "assignment"},
			&core.RelationField{Name: "congregation", CollectionId: congregationsCol.Id, CascadeDelete: false},
			&core.RelationField{Name: "map", CollectionId: mapsCol.Id, CascadeDelete: false},
			&core.RelationField{Name: "user", CollectionId: usersCol.Id, CascadeDelete: false},
			&core.TextField{Name: "publisher"},
			&core.TextField{Name: "type"},
			&core.TextField{Name: "action"},
			&core.DateField{Name: "expiry_date"},
			&core.RelationField{Name: "changed_by", CollectionId: usersCol.Id, CascadeDelete: false},
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		aLog.AddIndex("idx_assignments_log_map_created", false, "map, created", "")
		aLog.AddIndex("idx_assignments_log_user_created", false, "user, created", "")
		if err := app.Save(aLog); err != nil {
			return err
		}

		// Create roles_log
		rLog := core.NewBaseCollection("roles_log")
		rLog.Fields.Add(
			&core.RelationField{Name: "congregation", CollectionId: congregationsCol.Id, CascadeDelete: false},
			&core.RelationField{Name: "user", CollectionId: usersCol.Id, CascadeDelete: false},
			&core.TextField{Name: "old_role"},
			&core.TextField{Name: "new_role"},
			&core.TextField{Name: "action"},
			&core.RelationField{Name: "changed_by", CollectionId: usersCol.Id, CascadeDelete: false},
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		rLog.AddIndex("idx_roles_log_congregation_created", false, "congregation, created", "")
		rLog.AddIndex("idx_roles_log_user_created", false, "user, created", "")
		if err := app.Save(rLog); err != nil {
			return err
		}

		return nil
	}, func(app core.App) error {
		if col, err := app.FindCollectionByNameOrId("assignments_log"); err == nil {
			if err := app.Delete(col); err != nil {
				return err
			}
		}

		if col, err := app.FindCollectionByNameOrId("roles_log"); err == nil {
			if err := app.Delete(col); err != nil {
				return err
			}
		}

		return nil
	})
}
