package migrations

import (
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	m "ganigeorgiev.com/pocketbase/pocketbase/migrations"
	"ganigeorgiev.com/pocketbase/pocketbase/tools/types"
)

// usersAuthRule stops a disabled account from signing in. PocketBase checks
// the auth rule on every sign-in method (password, OTP, OAuth2) and on token
// refresh, and answers 403 when it fails. Tokens issued before an account was
// disabled are cut off by the users update hook in setup.
const usersAuthRule = "disabled = false"

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		collection.AuthRule = types.Pointer(usersAuthRule)
		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}
		collection.AuthRule = types.Pointer("")
		return app.Save(collection)
	})
}
