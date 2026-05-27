package handlers

import (
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/routine"
)

// HandleAddressAggregateUpdate triggers an async map aggregate recalculation
// whenever a field-worker updates an address status or not_home_tries.
//
// Guards (in cheapest-first order):
//  1. source == "bulk_reset" — set by reset handlers during batch resets; they
//     call ProcessMapAggregates explicitly at the end, so per-address hook firing
//     is suppressed to avoid N redundant recalcs.
//  2. Neither status nor not_home_tries changed — irrelevant field update (notes,
//     updated_by, etc.); skip to avoid unnecessary DB work.
//  3. map field is empty — address is not associated with a map; nothing to recalc.
//
// It should be called from OnRecordAfterUpdateSuccess("addresses").
func HandleAddressAggregateUpdate(e *core.RecordEvent) {
	// Only suppress for bulk_reset — that is the only update-time source value.
	// All other source values (app, admin, map_init, floor_copy) are creation-time
	// markers and must still trigger recalc on field-worker updates.
	if e.Record.GetString("source") == "bulk_reset" {
		return
	}

	statusChanged := e.Record.Original().GetString("status") != e.Record.GetString("status")
	triesChanged := e.Record.Original().GetInt("not_home_tries") != e.Record.GetInt("not_home_tries")
	if !statusChanged && !triesChanged {
		return
	}

	mapID := e.Record.GetString("map")
	if mapID == "" {
		return
	}

	appRef := e.App
	routine.FireAndForget(func() {
		if err := ProcessMapAggregates(mapID, appRef); err != nil {
			appRef.Logger().Error("aggregate hook failed", "map", mapID, "err", err)
		}
	})
}
