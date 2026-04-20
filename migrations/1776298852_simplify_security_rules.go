package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// Simplify security rules by removing @collection.roles and @collection.assignments
// relational joins from create/update/delete rules. Authorization hooks handle the
// relational checks instead.
func init() {
	m.Register(func(app core.App) error {
		authOnly := types.Pointer(`@request.auth.id != ""`)
		authOrLink := types.Pointer(`@request.auth.id != "" || @request.headers.link_id != ""`)

		// addresses: simplify create and update rules
		{
			col, err := app.FindCollectionByNameOrId("addresses")
			if err != nil {
				return err
			}
			col.CreateRule = authOrLink
			col.UpdateRule = authOrLink
			// DeleteRule stays nil (blocked)
			if err := app.Save(col); err != nil {
				return err
			}
		}

		// address_options: simplify create and delete rules
		{
			col, err := app.FindCollectionByNameOrId("address_options")
			if err != nil {
				return err
			}
			col.CreateRule = authOrLink
			col.DeleteRule = authOrLink
			// UpdateRule stays nil (blocked)
			if err := app.Save(col); err != nil {
				return err
			}
		}

		// assignments: simplify create and delete rules
		{
			col, err := app.FindCollectionByNameOrId("assignments")
			if err != nil {
				return err
			}
			col.CreateRule = authOnly
			col.DeleteRule = authOnly
			// UpdateRule stays nil (blocked)
			if err := app.Save(col); err != nil {
				return err
			}
		}

		// maps: simplify update and delete rules
		{
			col, err := app.FindCollectionByNameOrId("maps")
			if err != nil {
				return err
			}
			// CreateRule stays nil (blocked)
			col.UpdateRule = authOnly
			col.DeleteRule = authOnly
			if err := app.Save(col); err != nil {
				return err
			}
		}

		// messages: simplify create, update, and delete rules
		{
			col, err := app.FindCollectionByNameOrId("messages")
			if err != nil {
				return err
			}
			col.CreateRule = authOrLink
			col.UpdateRule = authOnly
			col.DeleteRule = authOnly
			if err := app.Save(col); err != nil {
				return err
			}
		}

		// roles: simplify create, update, and delete rules
		{
			col, err := app.FindCollectionByNameOrId("roles")
			if err != nil {
				return err
			}
			col.CreateRule = authOnly
			col.UpdateRule = authOnly
			col.DeleteRule = authOnly
			if err := app.Save(col); err != nil {
				return err
			}
		}

		// territories: simplify create, update, and delete rules
		{
			col, err := app.FindCollectionByNameOrId("territories")
			if err != nil {
				return err
			}
			col.CreateRule = authOnly
			col.UpdateRule = authOnly
			col.DeleteRule = authOnly
			if err := app.Save(col); err != nil {
				return err
			}
		}

		// congregations: simplify update rule
		{
			col, err := app.FindCollectionByNameOrId("congregations")
			if err != nil {
				return err
			}
			// CreateRule stays nil (blocked)
			col.UpdateRule = authOnly
			// DeleteRule stays nil (blocked)
			if err := app.Save(col); err != nil {
				return err
			}
		}

		return nil
	}, func(app core.App) error {
		// DOWN: restore original rules with @collection.roles and @collection.assignments joins

		rolesAdminRule := types.Pointer(`@request.auth.id != "" && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.congregation ?= congregation && @collection.roles:access.role ?= 'administrator'`)
		rolesAdminOrConductorRule := types.Pointer(`@request.auth.id != "" && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.congregation ?= congregation && (@collection.roles:access.role ?= 'administrator' || @collection.roles:access.role ?= 'conductor')`)
		rolesMemberOrLinkRule := types.Pointer(`(@request.auth.id != "" && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.congregation ?= congregation) || (@request.headers.link_id != "" && @collection.assignments:link.id ?= @request.headers.link_id && @collection.assignments:link.map ?= map)`)

		// addresses
		{
			col, err := app.FindCollectionByNameOrId("addresses")
			if err != nil {
				return err
			}
			col.CreateRule = rolesMemberOrLinkRule
			col.UpdateRule = rolesMemberOrLinkRule
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
			col.CreateRule = rolesMemberOrLinkRule
			col.DeleteRule = rolesMemberOrLinkRule
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
			col.CreateRule = rolesAdminOrConductorRule
			col.DeleteRule = rolesAdminOrConductorRule
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
			col.UpdateRule = rolesAdminRule
			col.DeleteRule = rolesAdminRule
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
			col.CreateRule = rolesMemberOrLinkRule
			col.UpdateRule = rolesAdminRule
			col.DeleteRule = rolesAdminRule
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
			col.CreateRule = rolesAdminRule
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
			col.UpdateRule = types.Pointer(`@request.auth.id != "" && @collection.roles:access.user ?= @request.auth.id && @collection.roles:access.congregation ?= id && @collection.roles:access.role ?= 'administrator'`)
			if err := app.Save(col); err != nil {
				return err
			}
		}

		return nil
	})
}
