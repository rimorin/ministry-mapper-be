package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// Remove all @collection.roles and @collection.assignments relational joins
// from security rules. Authorization is now handled by Go hooks (auth_hooks.go),
// which use direct indexed queries instead of PB's filter-engine joins.
//
// Rules are simplified to gate-level checks only:
//   - Auth type: @request.auth.id != ""
//   - Link type: @request.headers.link_id != ""
//   - Filter guards: @request.query.filter ~ "map=", etc.
//   - Data filters: expiry_date > @now (assignments)
func init() {
	m.Register(func(app core.App) error {
		authOnly := types.Pointer(`@request.auth.id != ""`)
		authOrLink := types.Pointer(`@request.auth.id != "" || @request.headers.link_id != ""`)

		// addresses
		{
			col, err := app.FindCollectionByNameOrId("addresses")
			if err != nil {
				return err
			}
			col.ListRule = types.Pointer(`(@request.auth.id != "" || @request.headers.link_id != "") && @request.query.filter:isset = true && @request.query.fields:isset = true && @request.query.filter ~ "map="`)
			col.CreateRule = authOrLink
			col.UpdateRule = authOrLink
			if err := app.Save(col); err != nil {
				return err
			}
		}

		// address_options
		{
			col, err := app.FindCollectionByNameOrId("address_options")
			if err != nil {
				return err
			}
			col.ListRule = types.Pointer(`(@request.auth.id != "" || @request.headers.link_id != "") && @request.query.filter:isset = true && @request.query.fields:isset = true && @request.query.filter ~ "map="`)
			col.ViewRule = authOrLink
			col.CreateRule = authOrLink
			col.DeleteRule = authOrLink
			if err := app.Save(col); err != nil {
				return err
			}
		}

		// maps
		{
			col, err := app.FindCollectionByNameOrId("maps")
			if err != nil {
				return err
			}
			col.ListRule = types.Pointer(`@request.auth.id != "" && @request.query.filter:isset = true && (@request.query.filter ~ "territory=" || @request.query.filter ~ "congregation=") && @request.query.fields:isset = true`)
			col.ViewRule = authOrLink
			col.UpdateRule = authOnly
			col.DeleteRule = authOnly
			if err := app.Save(col); err != nil {
				return err
			}
		}

		// territories
		{
			col, err := app.FindCollectionByNameOrId("territories")
			if err != nil {
				return err
			}
			col.ListRule = types.Pointer(`@request.auth.id != "" && @request.query.filter:isset = true && @request.query.filter ~ "congregation=" && @request.query.fields:isset = true`)
			col.CreateRule = authOnly
			col.UpdateRule = authOnly
			col.DeleteRule = authOnly
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
			col.ViewRule = authOrLink
			col.UpdateRule = authOnly
			if err := app.Save(col); err != nil {
				return err
			}
		}

		// messages
		{
			col, err := app.FindCollectionByNameOrId("messages")
			if err != nil {
				return err
			}
			col.ListRule = types.Pointer(`(@request.auth.id != "" || @request.headers.link_id != "") && @request.query.filter:isset = true && @request.query.filter ~ "map=" && @request.query.fields:isset = true`)
			col.CreateRule = authOrLink
			col.UpdateRule = authOnly
			col.DeleteRule = authOnly
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
			col.ListRule = types.Pointer(`(@request.auth.id != "" || @request.headers.link_id != "") && @request.query.filter:isset = true && @request.query.filter ~ "congregation=" && @request.query.fields:isset = true`)
			col.ViewRule = authOrLink
			if err := app.Save(col); err != nil {
				return err
			}
		}

		// roles
		{
			col, err := app.FindCollectionByNameOrId("roles")
			if err != nil {
				return err
			}
			col.ListRule = types.Pointer(`@request.auth.id != "" && @request.query.filter:isset = true && (@request.query.filter ~ "user=" || @request.query.filter ~ "congregation=") && @request.query.fields:isset = true`)
			col.CreateRule = authOnly
			col.UpdateRule = authOnly
			col.DeleteRule = authOnly
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
			col.ListRule = types.Pointer(`@request.auth.id != "" && @request.query.filter:isset = true && (@request.query.filter ~ "email~" || @request.query.filter ~ "name~") && @request.query.fields:isset = true && verified = true`)
			col.ViewRule = authOnly
			if err := app.Save(col); err != nil {
				return err
			}
		}

		// assignments
		{
			col, err := app.FindCollectionByNameOrId("assignments")
			if err != nil {
				return err
			}
			col.ListRule = types.Pointer(`@request.auth.id != "" && @request.query.filter:isset = true && (@request.query.filter ~ "map=" || @request.query.filter ~ "user=") && @request.query.fields:isset = true && expiry_date > @now`)
			col.ViewRule = types.Pointer(`(@request.auth.id != "" || @request.headers.link_id != "") && expiry_date > @now`)
			col.CreateRule = authOnly
			col.DeleteRule = authOnly
			if err := app.Save(col); err != nil {
				return err
			}
		}

		return nil
	}, func(app core.App) error {
		// DOWN: restore original rules with @collection joins

		rolesAdminRule := types.Pointer(`@request.auth.id != "" && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.congregation ?= congregation && @collection.roles:access.role ?= 'administrator'`)
		rolesAdminOrConductorRule := types.Pointer(`@request.auth.id != "" && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.congregation ?= congregation && (@collection.roles:access.role ?= 'administrator' || @collection.roles:access.role ?= 'conductor')`)
		rolesMemberOrLinkRule := types.Pointer(`(@request.auth.id != "" && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.congregation ?= congregation) || (@request.headers.link_id != "" && @collection.assignments:link.id ?= @request.headers.link_id && @collection.assignments:link.map ?= map)`)

		// addresses
		{
			col, err := app.FindCollectionByNameOrId("addresses")
			if err != nil {
				return err
			}
			col.ListRule = types.Pointer(`// PB Limitation: Reduce role joins for registered users as addresses are huge
(@request.auth.id != "" || (@request.headers.link_id != "" && @collection.assignments:link.id ?= @request.headers.link_id && @collection.assignments:link.map ?= map)) && @request.query.filter:isset = true && @request.query.fields:isset = true && @request.query.filter ~ "map="`)
			col.CreateRule = rolesMemberOrLinkRule
			col.UpdateRule = types.Pointer(`(@request.auth.id != "" && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.congregation ?= congregation) || (@request.headers.link_id != "" && @collection.assignments:link.id ?= @request.headers.link_id && @collection.assignments:link.map ?= map)`)
			if err := app.Save(col); err != nil {
				return err
			}
		}

		// address_options
		{
			col, err := app.FindCollectionByNameOrId("address_options")
			if err != nil {
				return err
			}
			col.ListRule = types.Pointer(`(@request.auth.id != "" || (@request.headers.link_id != "" && @collection.assignments:link.id ?= @request.headers.link_id)) && @request.query.filter:isset = true && @request.query.fields:isset = true && @request.query.filter ~ "map="`)
			col.ViewRule = types.Pointer(`(@request.auth.id != "" || (@request.headers.link_id != "" && @collection.assignments:link.id ?= @request.headers.link_id)) && @request.query.filter:isset = true && @request.query.fields:isset = true && @request.query.filter ~ "map="`)
			col.CreateRule = rolesMemberOrLinkRule
			col.DeleteRule = rolesMemberOrLinkRule
			if err := app.Save(col); err != nil {
				return err
			}
		}

		// maps
		{
			col, err := app.FindCollectionByNameOrId("maps")
			if err != nil {
				return err
			}
			col.ListRule = types.Pointer(`@request.auth.id != "" && @request.query.filter:isset = true && @request.query.filter ~ "territory=" && @request.query.fields:isset = true && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.congregation ?= congregation`)
			col.ViewRule = types.Pointer(`(@request.auth.id != "" && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.congregation ?= congregation) || (@request.headers.link_id != "" && @collection.assignments:link.id ?= @request.headers.link_id && @collection.assignments:link.map ?= id)`)
			col.UpdateRule = rolesAdminRule
			col.DeleteRule = rolesAdminRule
			if err := app.Save(col); err != nil {
				return err
			}
		}

		// territories
		{
			col, err := app.FindCollectionByNameOrId("territories")
			if err != nil {
				return err
			}
			col.ListRule = types.Pointer(`@request.auth.id != "" && @request.query.filter:isset = true && @request.query.filter ~ "congregation=" && @request.query.fields:isset = true && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.congregation ?= congregation`)
			col.CreateRule = rolesAdminRule
			col.UpdateRule = rolesAdminRule
			col.DeleteRule = rolesAdminRule
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
			col.ViewRule = types.Pointer(`(@request.auth.id != "" && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.congregation ?= id) || (@request.headers.link_id != "" && @collection.assignments:link.id ?= @request.headers.link_id && @collection.assignments:link.congregation ?= id)`)
			col.UpdateRule = types.Pointer(`@request.auth.id != "" && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.congregation ?= id && @collection.roles:access.role ?= 'administrator'`)
			if err := app.Save(col); err != nil {
				return err
			}
		}

		// messages
		{
			col, err := app.FindCollectionByNameOrId("messages")
			if err != nil {
				return err
			}
			col.ListRule = types.Pointer(`((@request.auth.id != "" && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.congregation ?= congregation) || (@request.headers.link_id != "" && @collection.assignments:link.id ?= @request.headers.link_id && @collection.assignments:link.map ?= map)) && @request.query.filter:isset = true && @request.query.filter ~ "map=" && @request.query.fields:isset = true`)
			col.CreateRule = rolesMemberOrLinkRule
			col.UpdateRule = rolesAdminRule
			col.DeleteRule = rolesAdminRule
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
			col.ListRule = types.Pointer(`((@request.auth.id != "" && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.congregation ?= congregation) || (@request.headers.link_id != "" && @collection.assignments:link.id ?= @request.headers.link_id && @collection.assignments:link.congregation ?= congregation)) && @request.query.filter:isset = true && @request.query.filter ~ "congregation=" && @request.query.fields:isset = true`)
			col.ViewRule = types.Pointer(`@request.auth.id != "" && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.congregation ?= congregation`)
			if err := app.Save(col); err != nil {
				return err
			}
		}

		// roles
		{
			col, err := app.FindCollectionByNameOrId("roles")
			if err != nil {
				return err
			}
			col.ListRule = types.Pointer(`@request.auth.id != "" && @request.query.filter:isset = true && (@request.query.filter ~ "user=" || @request.query.filter ~ "congregation=") && @request.query.fields:isset = true && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.congregation ?= congregation`)
			col.CreateRule = rolesAdminRule
			col.UpdateRule = rolesAdminRule
			col.DeleteRule = rolesAdminRule
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
			col.ListRule = types.Pointer(`@request.auth.id != "" && @request.query.filter:isset = true && (@request.query.filter ~ "email~" || @request.query.filter ~ "name~") && @request.query.fields:isset = true && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.role ?= 'administrator' && verified = true`)
			col.ViewRule = types.Pointer(`@request.auth.id != "" && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.role ?= 'administrator'`)
			if err := app.Save(col); err != nil {
				return err
			}
		}

		// assignments
		{
			col, err := app.FindCollectionByNameOrId("assignments")
			if err != nil {
				return err
			}
			col.ListRule = types.Pointer(`@request.auth.id != "" && @request.query.filter:isset = true && (@request.query.filter ~ "map=" || @request.query.filter ~ "user=") && @request.query.fields:isset = true && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.congregation ?= congregation && expiry_date > @now`)
			col.ViewRule = types.Pointer(`((@request.auth.id != "" && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.congregation ?= congregation) || (@request.headers.link_id != "" && @request.headers.link_id = id)) && expiry_date > @now`)
			col.CreateRule = rolesAdminOrConductorRule
			col.DeleteRule = rolesAdminOrConductorRule
			if err := app.Save(col); err != nil {
				return err
			}
		}

		return nil
	})
}
