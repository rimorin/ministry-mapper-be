package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		_, err := app.DB().NewQuery(`
			UPDATE address_options
			SET map = (SELECT map FROM addresses WHERE addresses.id = address_options.address)
			WHERE map IS NULL OR map = ''
		`).Execute()
		return err
	}, func(app core.App) error {
		_, err := app.DB().NewQuery(`
			UPDATE address_options SET map = ''
		`).Execute()
		return err
	})
}
