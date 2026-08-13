package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Drops six indexes whose columns are a strict leading prefix of another index
// on the same table, so SQLite can already answer their queries from the wider
// index. Verified against a copy of production data (1,160,985 addresses):
// every affected query keeps an indexed SEARCH, only the index name changes,
// and the two `roles` lookups stay covering.
//
//	addresses.idx_7CBdHug     (map)                 -> idx_Fx581hd    (map, status)
//	maps.idx_O2TlLJr          (territory)           -> idx_TzbzxPXi9e (territory, sequence)
//	territories.idx_Otsl0yR   (congregation)        -> idx_fMh5sfU    (congregation, code)
//	assignments.idx_pI4sxv2   (map)                 -> idx_Su1rP10S5r (map, expiry_date)
//	roles.idx_iPooFW46s8      (user)                -> idx_PUEoaq44d4 (user, congregation, role)
//	roles.idx_Dya44KEsGS      (user, congregation)  -> idx_PUEoaq44d4 (user, congregation, role)
//
// Reclaims ~27 MB and removes six index writes per row inserted into those
// tables. The freed pages go to the freelist; a VACUUM is needed to return them
// to the filesystem and cannot run here because migrations execute inside a
// transaction.
//
// Deliberately NOT touched: addresses.idx_4xBUDiPsKJ (source, created), 38 MB
// for the 438 rows the new-address digest reads. A partial index would shrink it
// to almost nothing, but PocketBase's filter engine compiles the literal in
// "source = 'app'" down to a bound parameter, and SQLite cannot prove a bound
// parameter satisfies a partial index predicate — it falls back to a full scan.
// Making that pay off means restructuring ProcessNewAddress to query by id
// first, which is a code change, not an index change.
// redundantIndexes maps each collection to the indexes dropped by this migration,
// paired with the column expression needed to recreate them on the way down.
var redundantIndexes = map[string][][2]string{
	"addresses":   {{"idx_7CBdHug", "`map`"}},
	"maps":        {{"idx_O2TlLJr", "`territory`"}},
	"territories": {{"idx_Otsl0yR", "`congregation`"}},
	"assignments": {{"idx_pI4sxv2", "`map`"}},
	"roles": {
		{"idx_iPooFW46s8", "`user`"},
		{"idx_Dya44KEsGS", "`user`,\n  `congregation`"},
	},
}

func init() {
	m.Register(func(app core.App) error {
		return updateIndexes(app, func(collection *core.Collection, name, _ string) bool {
			if collection.GetIndex(name) == "" {
				return false
			}
			collection.RemoveIndex(name)
			return true
		})
	}, func(app core.App) error {
		return updateIndexes(app, func(collection *core.Collection, name, columnsExpr string) bool {
			if collection.GetIndex(name) != "" {
				return false
			}
			collection.AddIndex(name, false, columnsExpr, "")
			return true
		})
	})
}

// updateIndexes applies change to every index in redundantIndexes, saving only
// the collections it actually modified. Collections that don't exist and indexes
// already in the desired state are skipped, so this runs cleanly on a fresh
// database as well as on one that has already been migrated.
func updateIndexes(app core.App, change func(collection *core.Collection, name, columnsExpr string) bool) error {
	for collectionName, indexes := range redundantIndexes {
		collection, err := app.FindCollectionByNameOrId(collectionName)
		if err != nil {
			continue
		}

		changed := false
		for _, idx := range indexes {
			if change(collection, idx[0], idx[1]) {
				changed = true
			}
		}

		if changed {
			if err := app.Save(collection); err != nil {
				return err
			}
		}
	}

	return nil
}
