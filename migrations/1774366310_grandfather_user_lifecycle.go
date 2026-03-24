package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		// One-time grandfather migration run before the user lifecycle jobs are first
		// enabled in production.
		//
		// Background:
		//   The system has been running for years without user lifecycle enforcement.
		//   Enabling the jobs on an existing database without this migration would:
		//   - Immediately disable every roleless user older than 7 days (unprovisioned pipeline)
		//   - Mass-email every user inactive 91–182 days (inactive pipeline)
		//   - Immediately disable every user inactive 183+ days (cannot be prevented —
		//     the disable check fires before the warning check in the job)
		//
		// NOTE: PocketBase stores empty DateField values as '' (empty string), not SQL
		// NULL. All timestamp checks must use (IS NULL OR = '') — never IS NULL alone.
		//
		// Part 1 — Unprovisioned grace period:
		//   Stamp unprovisioned_since = now for all non-disabled roleless users that
		//   have not already been stamped. This resets their age to 0 from the job's
		//   perspective, giving every existing roleless user the full 7-day grace period
		//   instead of being immediately disabled based on account creation date.
		//
		// Part 2 — Inactive warning suppression:
		//   Stamp inactive_warning_sent_at = now for all non-disabled users already
		//   inactive 91+ days. The job treats this as "first warning already sent":
		//   - 91–151 day inactive users: skip first warning; resume normal lifecycle.
		//   - 152–182 day inactive users: skip first warning; final warning fires next run.
		//   - 183+ day inactive users: not protected — disable fires before warning check.

		// Part 1: stamp unprovisioned_since for roleless non-disabled users.
		_, err := app.DB().NewQuery(`
			UPDATE users
			SET unprovisioned_since = datetime('now')
			WHERE disabled = 0
			  AND (unprovisioned_since IS NULL OR unprovisioned_since = '')
			  AND id NOT IN (
			      SELECT DISTINCT user FROM roles
			      WHERE user IS NOT NULL AND user != ''
			  )
		`).Execute()
		if err != nil {
			return err
		}

		// Part 2: stamp inactive_warning_sent_at for 91+ day inactive non-disabled users.
		_, err = app.DB().NewQuery(`
			UPDATE users
			SET inactive_warning_sent_at = datetime('now')
			WHERE disabled = 0
			  AND (inactive_warning_sent_at IS NULL OR inactive_warning_sent_at = '')
			  AND (
			      (last_login IS NOT NULL AND last_login != ''
			       AND CAST(JULIANDAY('now') - JULIANDAY(last_login) AS INTEGER) >= 91)
			      OR
			      ((last_login IS NULL OR last_login = '')
			       AND CAST(JULIANDAY('now') - JULIANDAY(created) AS INTEGER) >= 91)
			  )
		`).Execute()
		return err
	}, func(app core.App) error {
		// Rollback: clear both stamps set above.
		_, err := app.DB().NewQuery(`
			UPDATE users
			SET unprovisioned_since = ''
			WHERE disabled = 0
			  AND unprovisioned_since IS NOT NULL AND unprovisioned_since != ''
			  AND id NOT IN (
			      SELECT DISTINCT user FROM roles
			      WHERE user IS NOT NULL AND user != ''
			  )
		`).Execute()
		if err != nil {
			return err
		}

		_, err = app.DB().NewQuery(`
			UPDATE users
			SET inactive_warning_sent_at = ''
			WHERE disabled = 0
			  AND inactive_warning_sent_at IS NOT NULL AND inactive_warning_sent_at != ''
			  AND (
			      (last_login IS NOT NULL AND last_login != ''
			       AND CAST(JULIANDAY('now') - JULIANDAY(last_login) AS INTEGER) >= 91)
			      OR
			      ((last_login IS NULL OR last_login = '')
			       AND CAST(JULIANDAY('now') - JULIANDAY(created) AS INTEGER) >= 91)
			  )
		`).Execute()
		return err
	})
}
