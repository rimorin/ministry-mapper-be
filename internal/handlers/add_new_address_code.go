package handlers

import (
	"net/http"

	"github.com/getsentry/sentry-go"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// HandleMapAdd handles the addition of a new address code to a map.
// It performs the following steps:
// 1. Extracts the request information and retrieves the code and map ID from the request body.
// 2. Checks if the address code already exists for the given map ID. If it does, returns a bad request error.
// 3. Fetches the floors associated with the map ID. If an error occurs, returns a not found error.
// 4. Fetches the maximum sequence number for the map ID. If an error occurs, returns a not found error.
// 5. Fetches the map data for the map ID. If an error occurs, returns a not found error.
// 6. Fetches the default congregation option for the map's congregation. If an error occurs, returns a not found error.
// 7. Runs a transaction to insert a new address record for each floor associated with the map ID.
//   - Sets the code, congregation, floor, map ID, type, status, territory, and sequence for each record.
//   - If an error occurs during the transaction, returns the error.
//
// 8. Processes map aggregates for the map ID.
// 9. Returns a success message if all records are inserted successfully.
//
// Parameters:
// - e: The request event containing the request information.
// - app: The PocketBase application instance.
//
// Returns:
// - An error if any step fails, otherwise nil.
func HandleMapAdd(e *core.RequestEvent, app *pocketbase.PocketBase) error {
	requestInfo, _ := e.RequestInfo()
	data := requestInfo.Body
	code := data["code"].(string)
	mapId := data["map"].(string)

	_, err := fetchAddressByCode(app, code, mapId)
	if err == nil {
		return apis.NewBadRequestError("Code already exists", nil)
	}

	floors, err := fetchMapFloors(app, mapId)
	if err != nil {
		sentry.CaptureException(err)
		return apis.NewNotFoundError("Error fetching floors", nil)
	}

	sequence, err := fetchMapMaxSequence(app, mapId)
	if err != nil {
		sentry.CaptureException(err)
		return apis.NewNotFoundError("Error fetching sequence", nil)
	}

	mapData, err := fetchMapData(app, mapId)
	if err != nil {
		sentry.CaptureException(err)
		return apis.NewNotFoundError("Error fetching map data", nil)
	}

	defaultCode, err := fetchDefaultCongregationOption(app, mapData.Get("congregation").(string))
	if err != nil {
		sentry.CaptureException(err)
		return apis.NewNotFoundError("Error fetching default code", nil)
	}

	err = app.RunInTransaction(func(txApp core.App) error {
		collection, _ := app.FindCollectionByNameOrId("addresses")
		for _, floor := range floors {
			record := core.NewRecord(collection)
			record.Set("code", code)
			record.Set("congregation", mapData.Get("congregation"))
			record.Set("floor", floor)
			record.Set("map", mapId)
			record.Set("type", defaultCode.Id)
			record.Set("status", "not_done")
			record.Set("territory", mapData.Get("territory"))
			record.Set("sequence", sequence+1)

			if err := txApp.Save(record); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		sentry.CaptureException(err)
		return apis.NewNotFoundError("Error inserting records", nil)
	}
	ProcessMapAggregates(mapId, app)
	return e.String(http.StatusOK, "Records inserted successfully")
}
