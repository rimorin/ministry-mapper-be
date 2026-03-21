package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		_, err := app.DB().NewQuery("UPDATE users SET emailVisibility = TRUE WHERE emailVisibility = FALSE").Execute()
		return err
	}, func(app core.App) error {
		// non-reversible
		return nil
	})
}
