package handlers

import (
	"net/http"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// HandleMapTerritoryUpdate moves a map and all its addresses to a new territory
// in one transaction, then recalculates aggregates for both territories.
func HandleMapTerritoryUpdate(e *core.RequestEvent, app core.App) error {
	requestInfo, _ := e.RequestInfo()
	data := requestInfo.Body
	mapId := data["map"].(string)
	oldTerritory := data["old_territory"].(string)
	newTerritory := data["new_territory"].(string)

	addressRecords, err := fetchAddressesByMap(app, mapId)
	if err != nil {
		return apis.NewNotFoundError("Error fetching address", nil)
	}

	mapDetails, err := fetchMapData(app, mapId)
	if err != nil {
		return apis.NewNotFoundError("Error fetching map details", nil)
	}

	if !AuthorizeByRole(app, e.Auth.Id, mapDetails.GetString("congregation"), "administrator") {
		return apis.NewForbiddenError("Administrator access required", nil)
	}

	err = app.RunInTransaction(func(txApp core.App) error {
		for _, addressRecord := range addressRecords {
			addressRecord.Set("territory", newTerritory)
			if err := txApp.SaveNoValidate(addressRecord); err != nil {
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
