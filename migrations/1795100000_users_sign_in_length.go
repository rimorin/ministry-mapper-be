package migrations

import (
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	m "ganigeorgiev.com/pocketbase/pocketbase/migrations"
)

// How long a sign-in lasts, in seconds. The app renews it each time it is
// opened, so this is how long an account can go unused before it has to sign
// in again. Field service is weekly or fortnightly; fourteen days signed out
// anyone who missed a fortnight. Thirty days is NIST SP 800-63B's limit for
// a password-level sign-in, and the setting covers staff accounts too.
const (
	signInSeconds         int64 = 30 * 24 * 60 * 60
	previousSignInSeconds int64 = 14 * 24 * 60 * 60
)

func init() {
	m.Register(func(app core.App) error {
		return setUsersSignInLength(app, signInSeconds, func(current int64) bool {
			// A longer value set by hand in the dashboard is left alone.
			return current < signInSeconds
		})
	}, func(app core.App) error {
		return setUsersSignInLength(app, previousSignInSeconds, func(current int64) bool {
			return current == signInSeconds
		})
	})
}

func setUsersSignInLength(app core.App, seconds int64, applies func(current int64) bool) error {
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return nil
	}
	if !applies(users.AuthToken.Duration) {
		return nil
	}
	users.AuthToken.Duration = seconds
	return app.Save(users)
}
