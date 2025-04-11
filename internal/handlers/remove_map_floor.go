package handlers

import (
	"net/http"

	"github.com/getsentry/sentry-go"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// countFloorsInMap counts the number of distinct floors in a map
func countFloorsInMap(app *pocketbase.PocketBase, mapId string) (int, error) {
	floors := struct {
		Count int `db:"count"`
	}{}
	query := app.DB().NewQuery("SELECT COUNT(DISTINCT floor) as count FROM addresses WHERE map = {:map}")
	err := query.Bind(dbx.Params{"map": mapId}).One(&floors)
	return floors.Count, err
}

// HandleRemoveMapFloor handles the removal of a floor from a map.
// It ensures that there is more than one floor before allowing the deletion.
// If the floor count is greater than one, it fetches the addresses associated with the floor
// and deletes them within a transaction. After successful deletion, it processes map aggregates.
//
// Parameters:
//   - e: A pointer to the core.RequestEvent containing the request information.
//   - app: A pointer to the pocketbase.PocketBase instance.
//
// Returns:
//   - error: An error if the operation fails, otherwise nil.
func HandleRemoveMapFloor(e *core.RequestEvent, app *pocketbase.PocketBase) error {
	requestInfo, _ := e.RequestInfo()
	data := requestInfo.Body
	floor := data["floor"].(float64)
	mapId := data["map"].(string)

	// count floors and ensure that there is more than one floor
	floorCount, err := countFloorsInMap(app, mapId)
	if err != nil {
		sentry.CaptureException(err)
		return apis.NewNotFoundError("Error counting floors", nil)
	}

	if floorCount <= 1 {
		return apis.NewBadRequestError("Cannot delete the last floor", nil)
	}

	addresses, err := fetchMapAddressCodes(app, mapId, int(floor))
	if err != nil {
		sentry.CaptureException(err)
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
		sentry.CaptureException(err)
		return apis.NewNotFoundError("Transaction failed", nil)
	}
	ProcessMapAggregates(mapId, app)

	return e.String(http.StatusOK, "Map floor deleted successfully")
}
