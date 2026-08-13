package handlers

import (
	"net/http"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// countFloorsInMap counts the number of distinct floors in a map
func countFloorsInMap(app core.App, mapId string) (int, error) {
	floors := struct {
		Count int `db:"count"`
	}{}
	query := app.DB().NewQuery("SELECT COUNT(DISTINCT floor) as count FROM addresses WHERE map = {:map}")
	err := query.Bind(dbx.Params{"map": mapId}).One(&floors)
	return floors.Count, err
}

// RemoveMapFloorRequest uses a pointer for Floor so an omitted field is
// rejected rather than defaulting to 0, which is never a real floor — floors
// skip 0 and jump from 1 to -1.
type RemoveMapFloorRequest struct {
	Floor *int   `json:"floor"`
	Map   string `json:"map"`
}

// HandleRemoveMapFloor deletes all addresses on a floor, refusing to remove the
// last remaining floor. Map aggregates are recalculated afterwards.
func HandleRemoveMapFloor(e *core.RequestEvent, app core.App) error {
	data := RemoveMapFloorRequest{}
	if err := e.BindBody(&data); err != nil {
		return apis.NewBadRequestError("Invalid request body", nil)
	}
	if data.Map == "" {
		return apis.NewBadRequestError("map is required", nil)
	}
	if data.Floor == nil {
		return apis.NewBadRequestError("floor is required", nil)
	}

	floor := *data.Floor
	mapId := data.Map

	mapData, err := fetchMapData(app, mapId)
	if err != nil {
		return apis.NewNotFoundError("Error fetching map data", nil)
	}

	if !AuthorizeByRole(app, e.Auth.Id, mapData.GetString("congregation"), "administrator") {
		return apis.NewForbiddenError("Administrator access required", nil)
	}

	floorCount, err := countFloorsInMap(app, mapId)
	if err != nil {
		return apis.NewNotFoundError("Error counting floors", nil)
	}

	if floorCount <= 1 {
		return apis.NewBadRequestError("Cannot delete the last floor", nil)
	}

	addresses, err := fetchMapAddressCodes(app, mapId, floor)
	if err != nil {
		return apis.NewNotFoundError("Error fetching addresses", nil)
	}

	err = app.RunInTransaction(func(txApp core.App) error {
		for _, address := range addresses {
			if err := txApp.Delete(address); err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		return newServerError(err)
	}
	ProcessMapAggregates(mapId, app)

	return e.String(http.StatusOK, "Map floor deleted successfully")
}
