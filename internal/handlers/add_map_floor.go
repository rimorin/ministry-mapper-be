package handlers

import (
	"net/http"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// HandleMapFloor handles the addition of a new floor to a map in the PocketBase application.
// It fetches the necessary map data, determines the new floor number, and updates the addresses
// associated with the map to reflect the new floor.
//
// Parameters:
//   - e: A pointer to the core.RequestEvent containing the request information.
//   - app: A pointer to the pocketbase.PocketBase application instance.
//
// Returns:
//   - error: An error if any issues occur during the process, otherwise nil.
//
// The function performs the following steps:
//  1. Retrieves the request information and extracts the request body data.
//  2. Fetches the map data and default congregation option based on the map ID.
//  3. Determines the new floor number based on the add_higher flag.
//  4. Fetches the address codes associated with the map and the current floor.
//  5. Runs a transaction to update the addresses with the new floor number and other details.
//  6. Processes the map aggregates and returns a success message.
func HandleMapFloor(e *core.RequestEvent, app *pocketbase.PocketBase) error {
	requestInfo, _ := e.RequestInfo()
	data := requestInfo.Body
	add_higher := data["add_higher"].(bool)
	mapId := data["map"].(string)

	mapData, err := fetchMapData(app, mapId)
	if err != nil {
		return apis.NewNotFoundError("Error fetching map data", nil)
	}

	defaultType, err := fetchDefaultCongregationOption(app, mapData.Get("congregation").(string))
	if err != nil {
		return apis.NewNotFoundError("Error fetching default code", nil)
	}

	var floor int
	if add_higher {
		floor, err = fetchMapMaxFloor(app, mapId)
	} else {
		floor, err = fetchMapLowestFloor(app, mapId)
	}
	if err != nil {
		return apis.NewNotFoundError("Error fetching floor", nil)
	}

	addresses, err := fetchMapAddressCodes(app, mapId, floor)
	if err != nil {
		return apis.NewNotFoundError("Error fetching addresses", nil)
	}

	err = app.RunInTransaction(func(txApp core.App) error {
		if add_higher {
			floor++
		} else {
			floor--
			if floor == 0 {
				floor = -1
			}
		}

		for _, address := range addresses {
			collection, _ := app.FindCollectionByNameOrId("addresses")
			record := core.NewRecord(collection)
			record.Set("code", address.Get("code"))
			record.Set("floor", floor)
			record.Set("type", defaultType.Id)
			record.Set("congregation", address.Get("congregation"))
			record.Set("map", mapId)
			record.Set("status", "not_done")
			record.Set("territory", address.Get("territory"))
			record.Set("sequence", address.Get("sequence"))

			if err := txApp.Save(record); err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		return apis.NewNotFoundError("Error updating map floor", nil)
	}
	ProcessMapAggregates(mapId, app)

	return e.String(http.StatusOK, "Map floor updated successfully")
}
