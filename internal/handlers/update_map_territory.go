package handlers

import (
	"net/http"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

type UpdateMapTerritoryRequest struct {
	Map          string `json:"map"`
	NewTerritory string `json:"new_territory"`
}

// HandleMapTerritoryUpdate moves a map and all its addresses to a new territory
// in one transaction, then recalculates aggregates for both territories.
//
// The destination territory must belong to the same congregation as the map:
// without that check an administrator could file their map under another
// congregation's territory, which leaves the map's congregation and territory
// disagreeing and folds its addresses into the other congregation's territory
// progress (ProcessTerritoryAggregates sums maps by territory).
func HandleMapTerritoryUpdate(e *core.RequestEvent, app core.App) error {
	data := UpdateMapTerritoryRequest{}
	if err := e.BindBody(&data); err != nil {
		return apis.NewBadRequestError("Invalid request body", nil)
	}
	if data.Map == "" {
		return apis.NewBadRequestError("map is required", nil)
	}
	if data.NewTerritory == "" {
		return apis.NewBadRequestError("new_territory is required", nil)
	}

	mapDetails, err := fetchMapData(app, data.Map)
	if err != nil {
		return apis.NewNotFoundError("Error fetching map details", nil)
	}

	congregation := mapDetails.GetString("congregation")
	if !AuthorizeByRole(app, e.Auth.Id, congregation, "administrator") {
		return apis.NewForbiddenError("Administrator access required", nil)
	}

	// One message for both a missing and a foreign territory, so the response
	// doesn't reveal whether another congregation's territory id exists.
	if getTerritoryCongregation(app, data.NewTerritory) != congregation {
		return apis.NewBadRequestError("Invalid destination territory", nil)
	}

	// Taken from the record rather than the request body: the caller does not get
	// to choose which territory gets its aggregates recalculated.
	oldTerritory := mapDetails.GetString("territory")
	newTerritory := data.NewTerritory

	addressRecords, err := fetchAddressesByMap(app, data.Map)
	if err != nil {
		return apis.NewNotFoundError("Error fetching address", nil)
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
		return newServerError(err)
	}

	ProcessTerritoryAggregates(oldTerritory, app)
	ProcessTerritoryAggregates(newTerritory, app)

	return e.String(http.StatusOK, "Map territory updated successfully")
}
