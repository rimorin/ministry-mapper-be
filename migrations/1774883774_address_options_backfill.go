package migrations

import (
	m "github.com/pocketbase/pocketbase/migrations"

	"github.com/pocketbase/pocketbase/core"
)

// Backfill address_options from existing addresses.type JSON arrays.
//
// Uses a single INSERT ... SELECT with json_each() instead of iterating
// records in Go. This avoids loading 1M+ rows into memory, skips hook
// firing overhead, and lets SQLite bulk-insert in one pass (~seconds vs
// 2-3 minutes for the record-by-record approach).
//
// INSERT OR IGNORE respects the UNIQUE(address, option) constraint so the
// migration is idempotent — safe to re-run if partially completed.
// The id DEFAULT expression ('r'||lower(hex(randomblob(7)))) is called
// automatically by SQLite for every inserted row.
func init() {
	m.Register(func(app core.App) error {
		_, err := app.DB().NewQuery(`
			INSERT OR IGNORE INTO address_options (address, option, congregation, created, updated)
			SELECT
				a.id,
				jt.value,
				a.congregation,
				strftime('%Y-%m-%d %H:%M:%S.000Z', 'now'),
				strftime('%Y-%m-%d %H:%M:%S.000Z', 'now')
			FROM addresses a, json_each(a.type) AS jt
			WHERE jt.value != ''
		`).Execute()
		return err
	}, func(app core.App) error {
		_, err := app.DB().NewQuery("DELETE FROM address_options").Execute()
		return err
	})
}
