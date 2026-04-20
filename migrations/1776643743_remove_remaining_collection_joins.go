package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// Remove all remaining @collection.assignments joins from security rules.
// Authorization is now fully handled by Go hooks:
//   - OnRealtimeSubscribeRequest: validates link-id at subscribe time
//   - OnRecordsListRequest: validates link-id on REST list calls
//   - OnRecordViewRequest: validates link-id on REST view calls
//
// PB's built-in subscription filter check (realtimeCanAccessRecord) scopes
// SSE events via the subscriber's filter (e.g. map="X").
//
// Rules changed:
//   - addresses LIST: remove @collection.assignments link join
//   - address_options LIST: remove @collection.assignments link join
//   - address_options VIEW: remove @collection.assignments link join
//   - maps VIEW: remove @collection.assignments link join
func init() {
	m.Register(func(app core.App) error {
		// addresses LIST
		{
			col, err := app.FindCollectionByNameOrId("addresses")
			if err != nil {
				return err
			}
			col.ListRule = types.Pointer(`(@request.auth.id != "" || @request.headers.link_id != "") && @request.query.filter:isset = true && @request.query.fields:isset = true && @request.query.filter ~ "map="`)
			if err := app.Save(col); err != nil {
				return err
			}
		}

		// address_options LIST + VIEW
		{
			col, err := app.FindCollectionByNameOrId("address_options")
			if err != nil {
				return err
			}
			col.ListRule = types.Pointer(`(@request.auth.id != "" || @request.headers.link_id != "") && @request.query.filter:isset = true && @request.query.fields:isset = true && @request.query.filter ~ "map="`)
			col.ViewRule = types.Pointer(`(@request.auth.id != "" || @request.headers.link_id != "") && @request.query.filter:isset = true && @request.query.fields:isset = true && @request.query.filter ~ "map="`)
			if err := app.Save(col); err != nil {
				return err
			}
		}

		// maps VIEW
		{
			col, err := app.FindCollectionByNameOrId("maps")
			if err != nil {
				return err
			}
			col.ViewRule = types.Pointer(`@request.auth.id != "" || @request.headers.link_id != ""`)
			if err := app.Save(col); err != nil {
				return err
			}
		}

		return nil
	}, func(app core.App) error {
		// DOWN: restore original rules with @collection.assignments joins

		// addresses LIST
		{
			col, err := app.FindCollectionByNameOrId("addresses")
			if err != nil {
				return err
			}
			col.ListRule = types.Pointer(`// PB Limitation: Reduce role joins for registered users as addresses are huge
(@request.auth.id != "" || (@request.headers.link_id != "" && @collection.assignments:link.id ?= @request.headers.link_id)) && @request.query.filter:isset = true && @request.query.fields:isset = true && @request.query.filter ~ "map="`)
			if err := app.Save(col); err != nil {
				return err
			}
		}

		// address_options LIST + VIEW
		{
			col, err := app.FindCollectionByNameOrId("address_options")
			if err != nil {
				return err
			}
			col.ListRule = types.Pointer(`(@request.auth.id != "" || (@request.headers.link_id != "" && @collection.assignments:link.id ?= @request.headers.link_id)) && @request.query.filter:isset = true && @request.query.fields:isset = true && @request.query.filter ~ "map="`)
			col.ViewRule = types.Pointer(`(@request.auth.id != "" || (@request.headers.link_id != "" && @collection.assignments:link.id ?= @request.headers.link_id)) && @request.query.filter:isset = true && @request.query.fields:isset = true && @request.query.filter ~ "map="`)
			if err := app.Save(col); err != nil {
				return err
			}
		}

		// maps VIEW
		{
			col, err := app.FindCollectionByNameOrId("maps")
			if err != nil {
				return err
			}
			col.ViewRule = types.Pointer(`(@request.auth.id != "" || (@request.headers.link_id != "" && @collection.assignments:link.id ?= @request.headers.link_id && @collection.assignments:link.map ?= id))`)
			if err := app.Save(col); err != nil {
				return err
			}
		}

		return nil
	})
}
