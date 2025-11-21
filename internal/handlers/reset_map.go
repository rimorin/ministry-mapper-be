package handlers

import (
	"net/http"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// HandleResetMap handles the reset of a map by updating the status of addresses associated with the map.
// It fetches the addresses with a specific map ID and status of 'not_home' or 'done', and resets their status to 'not_done' and 'not_home_tries' to 0.
// The function runs within a transaction to ensure atomicity.
// If any error occurs during the process, it returns an appropriate error message.
// Finally, it processes map aggregates and returns a success response.
//
// Parameters:
// - e: A pointer to core.RequestEvent containing the request information.
// - app: A pointer to pocketbase.PocketBase instance for database operations.
//
// Returns:
// - error: An error object if any error occurs during the process, otherwise nil.
func HandleResetMap(e *core.RequestEvent, app *pocketbase.PocketBase) error {
	requestInfo, _ := e.RequestInfo()
	userName := e.Auth.Get("name").(string)
	data := requestInfo.Body
	mapId := data["map"].(string)
	records, err := app.FindRecordsByFilter("addresses", "map = {:id} && (status = 'not_home' || status = 'done')", "", 0, 0, dbx.Params{"id": mapId})

	if err != nil {
		return apis.NewNotFoundError("Error fetching addresses", nil)
	}

	err = app.RunInTransaction(func(txApp core.App) error {
		for _, record := range records {
			record.Set("status", "not_done")
			record.Set("not_home_tries", 0)
			record.Set("updated_by", userName)
			if err := txApp.Save(record); err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		return apis.NewNotFoundError("Error resetting map", nil)
	}

	ProcessMapAggregates(mapId, app)
	return e.JSON(http.StatusOK, "Map reset successfully")
}

// ResetMapTerritory processes the territory aggregates for a given map
func ResetMapTerritory(mapId string, app *pocketbase.PocketBase) error {
	mapDetails, err := app.FindRecordById("maps", mapId)
	if err != nil {
		return apis.NewNotFoundError("Error fetching map details", nil)
	}
	ProcessTerritoryAggregates(mapDetails.Get("territory").(string), app)
	return nil
}

// HandleResetTerritory handles the reset of a territory by updating the status of addresses
// and processing the related maps and territory aggregates.
//
// Parameters:
//   - c: A pointer to core.RequestEvent containing the request context.
//   - app: A pointer to pocketbase.PocketBase instance.
//
// Returns:
//   - error: An error object if an error occurs, otherwise nil.
//
// The function performs the following steps:
//  1. Retrieves the territory ID from the request body.
//  2. Fetches addresses associated with the territory that have a status of 'not_home' or 'done'.
//  3. Runs a transaction to update the status of these addresses to 'not_done' and resets the 'not_home_tries' count.
//  4. Fetches maps associated with the territory.
//  5. Processes map aggregates for each map.
//  6. Processes territory aggregates.
//  7. Returns a JSON response indicating the success of the operation.
func HandleResetTerritory(c *core.RequestEvent, app *pocketbase.PocketBase) error {
	requestInfo, _ := c.RequestInfo()
	userName := c.Auth.Get("name").(string)
	data := requestInfo.Body
	territoryId := data["territory"].(string)
	records, err := app.FindRecordsByFilter("addresses", "territory = {:id} && (status = 'not_home' || status = 'done')", "", 0, 0, dbx.Params{"id": territoryId})

	if err != nil {
		return apis.NewNotFoundError("Error fetching addresses", nil)
	}

	err = app.RunInTransaction(func(txApp core.App) error {
		for _, record := range records {
			record.Set("status", "not_done")
			record.Set("not_home_tries", 0)
			record.Set("updated_by", userName)
			if err := txApp.Save(record); err != nil {
				return err
			}
		}
		return c.Next()
	})

	if err != nil {
		return apis.NewNotFoundError("Error resetting territory", nil)
	}

	maps, err := app.FindRecordsByFilter("maps", "territory = {:id}", "", 0, 0, dbx.Params{"id": territoryId})

	if err != nil {
		return apis.NewNotFoundError("Error fetching maps", nil)
	}

	for _, maprecord := range maps {
		ProcessMapAggregates(maprecord.Id, app, false)
	}

	ProcessTerritoryAggregates(territoryId, app)

	return c.JSON(http.StatusOK, "Territory reset successfully")
}
