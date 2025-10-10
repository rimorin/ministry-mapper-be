package migrations

import (
	"log"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// This migration fixes a bug where dnc_time was not populated when addresses
// were marked with status "do_not_call". It backfills missing dnc_time values
// using the updated timestamp of each affected address.
// This is in response to PR: https://github.com/rimorin/ministry-mapper-v2/pull/56
func init() {
	m.Register(func(app core.App) error {
		// Update all addresses where status is "do_not_call" and dnc_time is null or empty
		// Set dnc_time to the updated timestamp to fix the bug where dnc_time was not initially populated
		result, err := app.DB().NewQuery(`
			UPDATE addresses 
			SET dnc_time = updated 
			WHERE status = {:status} AND (dnc_time IS NULL OR dnc_time = '')
		`).Bind(dbx.Params{"status": "do_not_call"}).Execute()

		if err != nil {
			return err
		}

		rowsAffected, _ := result.RowsAffected()
		log.Printf("Successfully patched %d addresses with missing dnc_time", rowsAffected)

		return nil
	}, nil)
}
