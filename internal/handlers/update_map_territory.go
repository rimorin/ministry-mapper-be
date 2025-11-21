package handlers

import (
	"net/http"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// HandleMapTerritoryUpdate handles the update of a map territory.
// It updates the territory of all addresses associated with the map and the map details itself.
// The update is performed within a single transaction to ensure atomicity.
//
// Parameters:
//   - e: A pointer to the core.RequestEvent containing the request information.
//   - app: A pointer to the pocketbase.PocketBase application instance.
//
// Returns:
//   - error: An error if the update fails, otherwise nil.
//
// The function performs the following steps:
//  1. Extracts the map ID, old territory, and new territory from the request body.
//  2. Fetches the existing address records and map details associated with the map ID.
//  3. Updates all addresses and map details within a single transaction to the new territory.
//  4. Processes territory aggregates for both the old and new territories.
//  5. Returns a success message if the update is successful, otherwise returns an error.
func HandleMapTerritoryUpdate(e *core.RequestEvent, app *pocketbase.PocketBase) error {
	requestInfo, _ := e.RequestInfo()
	data := requestInfo.Body
	mapId := data["map"].(string)
	oldTerritory := data["old_territory"].(string)
	newTerritory := data["new_territory"].(string)

	// Fetch the existing address records and map details
	addressRecords, err := fetchAddressesByMap(app, mapId)
	if err != nil {
		return apis.NewNotFoundError("Error fetching address", nil)
	}

	mapDetails, err := fetchMapData(app, mapId)
	if err != nil {
		return apis.NewNotFoundError("Error fetching map details", nil)
	}

	// Update all addresses and map details within a single transaction
	err = app.RunInTransaction(func(txApp core.App) error {
		for _, addressRecord := range addressRecords {
			addressRecord.Set("territory", newTerritory)
			if err := txApp.Save(addressRecord); err != nil {
				return err
			}
		}
		mapDetails.Set("territory", newTerritory)
		if err := txApp.Save(mapDetails); err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		return apis.NewApiError(500, "Error updating map territory", nil)
	}

	ProcessTerritoryAggregates(oldTerritory, app)
	ProcessTerritoryAggregates(newTerritory, app)

	return e.String(http.StatusOK, "Map territory updated successfully")
}
