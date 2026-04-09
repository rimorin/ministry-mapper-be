package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Backfill the `territory` field on address records that were created without
// it (introduced by the "add address on the fly" feature).  The correct
// territory can be derived from the address's own `map` relation, since every
// map already carries its `territory` foreign key.
func init() {
	m.Register(func(app core.App) error {
		_, err := app.DB().NewQuery(`
			UPDATE addresses
			SET    territory = (
				SELECT maps.territory
				FROM   maps
				WHERE  maps.id = addresses.map
			)
			WHERE  (territory IS NULL OR territory = '')
			  AND  map IS NOT NULL
			  AND  map != ''
		`).Execute()
		return err
	}, func(app core.App) error {
		// The down migration intentionally clears territory only on records
		// that were affected (i.e. those whose territory matches their map's
		// territory and that therefore had an empty territory before this
		// migration ran).  Because we cannot know which rows were originally
		// empty vs. legitimately set, we leave the down migration as a no-op
		// to avoid data loss.
		return nil
	})
}
