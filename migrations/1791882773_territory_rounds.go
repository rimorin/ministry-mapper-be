package migrations

import (
	"database/sql"
	"errors"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// Adds S-13 territory round tracking: the territories_log collection, the round
// fields on territories and maps, and congregations.completion_threshold
// (existing congregations get the default of 90).
//
// Territories already at the threshold with recorded work are marked completed,
// dated by their last work, so tracking starts from what the app already shows.
// That check is written out here rather than calling the handler, so this
// migration keeps doing the same thing when the live rule changes later.
func init() {
	m.Register(func(app core.App) error {
		if err := createTerritoriesLog(app); err != nil {
			return err
		}
		if err := addFieldsIfMissing(app, "territories",
			&core.DateField{Name: "round_started_at"},
			&core.DateField{Name: "last_completed_at"},
		); err != nil {
			return err
		}
		if err := addFieldsIfMissing(app, "maps", &core.DateField{Name: "reset_at"}); err != nil {
			return err
		}
		if err := addFieldsIfMissing(app, "congregations", &core.NumberField{
			Name:    "completion_threshold",
			Min:     types.Pointer(50.0),
			Max:     types.Pointer(100.0),
			OnlyInt: true,
		}); err != nil {
			return err
		}
		if _, err := app.DB().NewQuery(
			"UPDATE congregations SET completion_threshold = 90 WHERE COALESCE(completion_threshold, 0) = 0",
		).Execute(); err != nil {
			return err
		}
		return backfillTerritoryCompletions(app)
	}, func(app core.App) error {
		if collection, err := app.FindCollectionByNameOrId("territories_log"); err == nil {
			if err := app.Delete(collection); err != nil {
				return err
			}
		}
		for name, fields := range map[string][]string{
			"territories":   {"round_started_at", "last_completed_at"},
			"maps":          {"reset_at"},
			"congregations": {"completion_threshold"},
		} {
			collection, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				continue
			}
			for _, field := range fields {
				collection.Fields.RemoveByName(field)
			}
			if err := app.Save(collection); err != nil {
				return err
			}
		}
		return nil
	})
}

func createTerritoriesLog(app core.App) error {
	if _, err := app.FindCollectionByNameOrId("territories_log"); err == nil {
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

	collection := core.NewBaseCollection("territories_log")
	collection.Fields.Add(
		&core.RelationField{Name: "congregation", CollectionId: congregations.Id, MaxSelect: 1},
		&core.TextField{Name: "territory"},
		&core.TextField{Name: "territory_code"},
		&core.TextField{Name: "action", Required: true},
		&core.JSONField{Name: "details"},
		&core.RelationField{Name: "changed_by", CollectionId: users.Id, MaxSelect: 1},
		&core.AutodateField{Name: "created", OnCreate: true},
		&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
	)
	collection.AddIndex("idx_territories_log_territory_created", false, "`territory`, `created`", "")
	collection.AddIndex("idx_territories_log_congregation_created", false, "`congregation`, `created`", "")
	return app.Save(collection)
}

func addFieldsIfMissing(app core.App, collectionName string, fields ...core.Field) error {
	collection, err := app.FindCollectionByNameOrId(collectionName)
	if err != nil {
		return nil
	}
	changed := false
	for _, field := range fields {
		if collection.Fields.GetByName(field.GetName()) == nil {
			collection.Fields.Add(field)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return app.Save(collection)
}

func backfillTerritoryCompletions(app core.App) error {
	var candidates []struct {
		ID           string `db:"id"`
		Code         string `db:"code"`
		Congregation string `db:"congregation"`
		Progress     int    `db:"progress"`
		Threshold    int    `db:"threshold"`
	}
	if err := app.DB().NewQuery(`
		SELECT t.id, t.code, t.congregation, t.progress, c.completion_threshold AS threshold
		FROM territories t
		JOIN congregations c ON c.id = t.congregation
		WHERE t.progress >= c.completion_threshold
		AND COALESCE(t.last_completed_at, '') = ''
	`).All(&candidates); err != nil {
		return err
	}
	if len(candidates) == 0 {
		return nil
	}

	logs, err := app.FindCollectionByNameOrId("territories_log")
	if err != nil {
		return err
	}

	for _, territory := range candidates {
		var last string
		err := app.DB().NewQuery(`
			SELECT MAX(created) FROM addresses_log
			WHERE territory = {:territory}
			AND new_status IN ('done', 'not_home')
			HAVING SUM(CASE WHEN old_status = 'not_done' THEN 1 ELSE 0 END) > 0
		`).Bind(dbx.Params{"territory": territory.ID}).Row(&last)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		completedAt, err := types.ParseDateTime(last)
		if err != nil {
			return err
		}

		record := core.NewRecord(logs)
		record.Set("congregation", territory.Congregation)
		record.Set("territory", territory.ID)
		record.Set("territory_code", territory.Code)
		record.Set("action", "completed")
		record.Set("details", map[string]any{
			"threshold":  territory.Threshold,
			"progress":   territory.Progress,
			"backfilled": true,
		})
		record.SetRaw("created", completedAt)
		if err := app.SaveNoValidate(record); err != nil {
			return err
		}
		if _, err := app.DB().NewQuery(
			"UPDATE territories SET last_completed_at = {:at} WHERE id = {:id}",
		).Bind(dbx.Params{"at": completedAt.String(), "id": territory.ID}).Execute(); err != nil {
			return err
		}
	}
	return nil
}
