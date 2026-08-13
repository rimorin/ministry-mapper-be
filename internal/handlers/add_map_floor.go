package handlers

import (
	"fmt"
	"net/http"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// AddMapFloorRequest uses a pointer for AddHigher so an omitted field is
// rejected rather than silently defaulting to "add a floor below".
type AddMapFloorRequest struct {
	AddHigher *bool  `json:"add_higher"`
	Map       string `json:"map"`
}

// HandleMapFloor adds a new floor to a map by copying the address codes of the
// current highest (or lowest, per add_higher) floor onto the new floor.
func HandleMapFloor(e *core.RequestEvent, app core.App) error {
	data := AddMapFloorRequest{}
	if err := e.BindBody(&data); err != nil {
		return apis.NewBadRequestError("Invalid request body", nil)
	}
	if data.Map == "" {
		return apis.NewBadRequestError("map is required", nil)
	}
	if data.AddHigher == nil {
		return apis.NewBadRequestError("add_higher is required", nil)
	}

	add_higher := *data.AddHigher
	mapId := data.Map

	mapData, err := fetchMapData(app, mapId)
	if err != nil {
		return apis.NewNotFoundError("Error fetching map data", nil)
	}

	if !AuthorizeByRole(app, e.Auth.Id, mapData.GetString("congregation"), "administrator") {
		return apis.NewForbiddenError("Administrator access required", nil)
	}

	defaultType, err := fetchDefaultCongregationOption(app, mapData.GetString("congregation"))
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

		aoCollection, err := txApp.FindCachedCollectionByNameOrId("address_options")
		if err != nil {
			return err
		}

		addressCollection, err := txApp.FindCachedCollectionByNameOrId("addresses")
		if err != nil {
			return err
		}

		createdBy := e.Auth.GetString("name")

		for _, address := range addresses {
			record := core.NewRecord(addressCollection)
			record.Set("code", address.Get("code"))
			record.Set("floor", floor)
			record.Set("congregation", address.Get("congregation"))
			record.Set("map", mapId)
			record.Set("status", "not_done")
			record.Set("territory", address.Get("territory"))
			record.Set("sequence", address.Get("sequence"))
			record.Set("source", "floor_copy")
			record.Set("created_by", createdBy)

			if err := txApp.SaveNoValidate(record); err != nil {
				return err
			}
			aoRec := core.NewRecord(aoCollection)
			aoRec.Set("address", record.Id)
			aoRec.Set("option", defaultType.Id)
			aoRec.Set("congregation", fmt.Sprintf("%v", address.Get("congregation")))
			aoRec.Set("map", mapId)
			if err := txApp.SaveNoValidate(aoRec); err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		return newServerError(err)
	}
	ProcessMapAggregates(mapId, app)

	return e.String(http.StatusOK, "Map floor updated successfully")
}
