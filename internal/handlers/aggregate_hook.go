package handlers

import (
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/routine"
)

// HandleAddressAggregateUpdate triggers an async map aggregate recalculation
// whenever a field-worker updates an address status or not_home_tries.
//
// Guards (in cheapest-first order):
//  1. map field is empty — address is not associated with a map; nothing to recalc.
//  2. app.Store flag "bulk_reset:<mapID>" is set — reset handlers set this before
//     their transaction and clear it (via defer) after calling ProcessMapAggregates
//     explicitly, so per-address hook firing is suppressed to avoid N redundant recalcs.
//  3. Neither status nor not_home_tries changed — irrelevant field update (notes,
//     updated_by, etc.); skip to avoid unnecessary DB work.
//
// It should be called from OnRecordAfterUpdateSuccess("addresses").
func HandleAddressAggregateUpdate(e *core.RecordEvent) {
	mapID := e.Record.GetString("map")
	if mapID == "" {
		return
	}

	if e.App.Store().Has("bulk_reset:" + mapID) {
		return
	}

	statusChanged := e.Record.Original().GetString("status") != e.Record.GetString("status")
	triesChanged := e.Record.Original().GetInt("not_home_tries") != e.Record.GetInt("not_home_tries")
	if !statusChanged && !triesChanged {
		return
	}

	appRef := e.App
	routine.FireAndForget(func() {
		if err := ProcessMapAggregates(mapID, appRef); err != nil {
			appRef.Logger().Error("aggregate hook failed", "map", mapID, "err", err)
		}
	})
}
