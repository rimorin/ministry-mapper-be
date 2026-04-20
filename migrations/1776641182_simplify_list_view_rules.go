package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// Simplify list/view security rules by removing @collection relational joins.
// Authorization for these collections is handled by Go hooks (auth_hooks.go).
//
// Rules changed:
//   - messages LIST: remove @collection.roles join (subscribe hook validates)
//   - users LIST: remove @collection.roles admin join
//   - congregations VIEW: remove @collection.assignments link join
//   - options LIST: remove @collection.assignments link join
//   - options VIEW: remove @collection.assignments link join
func init() {
	m.Register(func(app core.App) error {
		// messages LIST
		{
			col, err := app.FindCollectionByNameOrId("messages")
			if err != nil {
				return err
			}
			col.ListRule = types.Pointer(`(@request.auth.id != "" || @request.headers.link_id != "") && @request.query.filter:isset = true && @request.query.filter ~ "map=" && @request.query.fields:isset = true`)
			if err := app.Save(col); err != nil {
				return err
			}
		}

		// users LIST
		{
			col, err := app.FindCollectionByNameOrId("users")
			if err != nil {
				return err
			}
			col.ListRule = types.Pointer(`@request.auth.id != "" && @request.query.filter:isset = true && (@request.query.filter ~ "email~" || @request.query.filter ~ "name~") && @request.query.fields:isset = true && verified = true && disabled = false`)
			if err := app.Save(col); err != nil {
				return err
			}
		}

		// congregations VIEW
		{
			col, err := app.FindCollectionByNameOrId("congregations")
			if err != nil {
				return err
			}
			col.ViewRule = types.Pointer(`@request.auth.id != "" || @request.headers.link_id != ""`)
			if err := app.Save(col); err != nil {
				return err
			}
		}

		// options LIST + VIEW
		{
			col, err := app.FindCollectionByNameOrId("options")
			if err != nil {
				return err
			}
			col.ListRule = types.Pointer(`(@request.auth.id != "" || @request.headers.link_id != "") && @request.query.filter:isset = true && @request.query.filter ~ "congregation=" && @request.query.fields:isset = true`)
			col.ViewRule = types.Pointer(`((@request.auth.id != "" && @request.query.filter:isset = true && @request.query.filter ~ "user=") || @request.headers.link_id != "") && @request.query.fields:isset = true`)
			if err := app.Save(col); err != nil {
				return err
			}
		}

		return nil
	}, func(app core.App) error {
		// DOWN: restore original rules with @collection joins

		// messages
		{
			col, err := app.FindCollectionByNameOrId("messages")
			if err != nil {
				return err
			}
			col.ListRule = types.Pointer(`((@request.auth.id != "" && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.congregation ?= congregation) || (@request.headers.link_id != "" && @collection.assignments:link.id ?= @request.headers.link_id && @collection.assignments:link.map ?= map)) && @request.query.filter:isset = true && @request.query.filter ~ "map=" && @request.query.fields:isset = true`)
			if err := app.Save(col); err != nil {
				return err
			}
		}

		// users
		{
			col, err := app.FindCollectionByNameOrId("users")
			if err != nil {
				return err
			}
			col.ListRule = types.Pointer(`@request.auth.id != "" && @request.query.filter:isset = true && (@request.query.filter ~ "email~" || @request.query.filter ~ "name~") && @request.query.fields:isset = true && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.role ?= 'administrator' && verified = true && disabled = false`)
			if err := app.Save(col); err != nil {
				return err
			}
		}

		// congregations
		{
			col, err := app.FindCollectionByNameOrId("congregations")
			if err != nil {
				return err
			}
			col.ViewRule = types.Pointer(`@request.auth.id != "" || (@request.headers.link_id != "" && @collection.assignments:link.id ?= @request.headers.link_id && @collection.assignments:link.congregation ?= id)`)
			if err := app.Save(col); err != nil {
				return err
			}
		}

		// options
		{
			col, err := app.FindCollectionByNameOrId("options")
			if err != nil {
				return err
			}
			col.ListRule = types.Pointer(`(@request.auth.id != "" || (@request.headers.link_id != "" && @collection.assignments:link.id ?= @request.headers.link_id)) && @request.query.filter:isset = true && @request.query.filter ~ "congregation=" && @request.query.fields:isset = true`)
			col.ViewRule = types.Pointer(`((@request.auth.id != "" && @request.query.filter:isset = true && @request.query.filter ~ "user=" ) || (@request.headers.link_id != "" && @collection.assignments:link.id ?= @request.headers.link_id)) && @request.query.fields:isset = true`)
			if err := app.Save(col); err != nil {
				return err
			}
		}

		return nil
	})
}
