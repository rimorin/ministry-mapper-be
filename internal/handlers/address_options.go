package handlers

import (
	"log"
	"slices"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// insertAddressOption creates a single row in address_options for the given
// address/option/congregation triple. Used by all address-creation paths as
// part of the Phase 1 dual-write strategy.
func insertAddressOption(txApp core.App, addressId, optionId, congregation string) error {
	collection, err := txApp.FindCachedCollectionByNameOrId("address_options")
	if err != nil {
		return err
	}
	rec := core.NewRecord(collection)
	rec.Set("address", addressId)
	rec.Set("option", optionId)
	rec.Set("congregation", congregation)
	return txApp.Save(rec)
}

// HandleAddressTypeSync is the OnRecordAfterUpdateSuccess("addresses") hook
// that keeps address_options in sync with addresses.type (Phase 1 dual-write).
//
// It fires only when the type set actually changes — status, notes, sequence,
// and other field updates skip this entirely. Uses sorted copies for
// order-insensitive comparison since the frontend may send types in any order.
//
// All deletions use app.Delete() (not raw SQL) so realtime events fire for
// each removed row. New rows are inserted via app.Save() for the same reason.
func HandleAddressTypeSync(e *core.RecordEvent) error {
	oldType := e.Record.Original().GetStringSlice("type")
	newType := e.Record.GetStringSlice("type")

	oldSorted := append([]string{}, oldType...)
	newSorted := append([]string{}, newType...)
	slices.Sort(oldSorted)
	slices.Sort(newSorted)

	if slices.Equal(oldSorted, newSorted) {
		return e.Next()
	}

	collection, err := e.App.FindCachedCollectionByNameOrId("address_options")
	if err != nil {
		log.Printf("warning: address_options collection not found: %v", err)
		return e.Next()
	}

	addressId := e.Record.Id
	congId := e.Record.GetString("congregation")

	syncErr := e.App.RunInTransaction(func(txApp core.App) error {
		existingRows, err := txApp.FindRecordsByFilter(
			"address_options", "address = {:address}",
			"", 0, 0, dbx.Params{"address": addressId},
		)
		if err != nil {
			return err
		}
		for _, row := range existingRows {
			if err := txApp.Delete(row); err != nil {
				return err
			}
		}
		for _, optID := range newType {
			rec := core.NewRecord(collection)
			rec.Set("address", addressId)
			rec.Set("option", optID)
			rec.Set("congregation", congId)
			if err := txApp.Save(rec); err != nil {
				return err
			}
		}
		return nil
	})

	if syncErr != nil {
		log.Printf("warning: failed to sync address_options for %s: %v", addressId, syncErr)
	}
	return e.Next()
}
