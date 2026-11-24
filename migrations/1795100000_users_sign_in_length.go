package migrations

import (
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	m "ganigeorgiev.com/pocketbase/pocketbase/migrations"
)

// Sign-in length in seconds. Applies to every account in users.
const (
	signInSeconds         int64 = 30 * 24 * 60 * 60
	previousSignInSeconds int64 = 14 * 24 * 60 * 60
)

func init() {
	m.Register(func(app core.App) error {
		return setUsersSignInLength(app, signInSeconds, func(current int64) bool {
			// Keep a longer value set in the dashboard.
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
