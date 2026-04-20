package migrations

import (
	"strings"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Drop indexes that are no longer used after moving @collection joins to hooks.
//
// Removed indexes:
//   - idx_NfI5WhsRsK (maps) — no query filters/sorts by updated
//   - idx_zJ75UsEjFK (addresses_log) — no query filters by address; collection is admin-only
//   - idx_g8Vt8JC1av (addresses_log) — no query filters by congregation+created
//   - idx_2gqasRAvRc (addresses_log) — no query filters by new_status+created
func init() {
	m.Register(func(app core.App) error {
		removals := map[string][]string{
			"maps":          {"idx_NfI5WhsRsK"},
			"addresses_log": {"idx_zJ75UsEjFK", "idx_g8Vt8JC1av", "idx_2gqasRAvRc"},
		}
		for collection, idxNames := range removals {
			col, err := app.FindCollectionByNameOrId(collection)
			if err != nil {
				return err
			}
			col.Indexes = filterOutIndexes(col.Indexes, idxNames)
			if err := app.Save(col); err != nil {
				return err
			}
		}
		return nil
	}, func(app core.App) error {
		// DOWN: restore dropped indexes
		additions := map[string][]string{
			"maps": {
				"CREATE INDEX `idx_NfI5WhsRsK` ON `maps` (`updated`)",
			},
			"addresses_log": {
				"CREATE INDEX `idx_zJ75UsEjFK` ON `addresses_log` (`address`)",
				"CREATE INDEX `idx_g8Vt8JC1av` ON `addresses_log` (`congregation`, `created`)",
				"CREATE INDEX `idx_2gqasRAvRc` ON `addresses_log` (`new_status`, `created`)",
			},
		}
		for collection, idxDefs := range additions {
			col, err := app.FindCollectionByNameOrId(collection)
			if err != nil {
				return err
			}
			col.Indexes = append(col.Indexes, idxDefs...)
			if err := app.Save(col); err != nil {
				return err
			}
		}
		return nil
	})
}

func filterOutIndexes(indexes []string, removeNames []string) []string {
	remove := make(map[string]bool, len(removeNames))
	for _, name := range removeNames {
		remove[name] = true
	}
	var result []string
	for _, idx := range indexes {
		shouldRemove := false
		for name := range remove {
			if strings.Contains(idx, name) {
				shouldRemove = true
				break
			}
		}
		if !shouldRemove {
			result = append(result, idx)
		}
	}
	return result
}
