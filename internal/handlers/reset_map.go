package handlers

import (
	"log"
	"net/http"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// HandleResetMap resets a map's 'not_home' and 'done' addresses back to 'not_done'
// and recalculates aggregates afterwards.
func HandleResetMap(e *core.RequestEvent, app core.App) error {
	requestInfo, _ := e.RequestInfo()
	userName := e.Auth.Get("name").(string)
	data := requestInfo.Body
	mapId := data["map"].(string)

	mapData, err := fetchMapData(app, mapId)
	if err != nil {
		return apis.NewNotFoundError("Map not found", nil)
	}

	if !AuthorizeByRole(app, e.Auth.Id, mapData.GetString("congregation"), "administrator") {
		return apis.NewForbiddenError("Administrator access required", nil)
	}

	records, err := app.FindRecordsByFilter("addresses", "map = {:id} && (status = 'not_home' || status = 'done')", "", 0, 0, dbx.Params{"id": mapId})

	if err != nil {
		return apis.NewNotFoundError("Error fetching addresses", nil)
	}

	// Suppress per-address aggregate hook fires during the batch. The flag is
	// checked by HandleAddressAggregateUpdate; defer clears it after the explicit
	// ProcessMapAggregates call below so field-worker updates resume normally.
	flagKey := "bulk_reset:" + mapId
	app.Store().Set(flagKey, true)
	defer app.Store().Remove(flagKey)

	err = app.RunInTransaction(func(txApp core.App) error {
		for _, record := range records {
			record.Set("status", "not_done")
			record.Set("not_home_tries", 0)
			record.Set("updated_by", userName)
			if err := txApp.SaveNoValidate(record); err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		return newServerError(err)
	}

	if err := ProcessMapAggregates(mapId, app); err != nil {
		log.Printf("Error recalculating aggregates for map %s: %v", mapId, err)
	}

	return e.JSON(http.StatusOK, "Map reset successfully")
}

// ResetMapTerritory processes the territory aggregates for a given map
func ResetMapTerritory(mapId string, app core.App) error {
	mapDetails, err := app.FindRecordById("maps", mapId)
	if err != nil {
		return apis.NewNotFoundError("Error fetching map details", nil)
	}
	ProcessTerritoryAggregates(mapDetails.Get("territory").(string), app)
	return nil
}

// HandleResetTerritory resets a territory's 'not_home' and 'done' addresses back
// to 'not_done', then recalculates aggregates for each affected map and the territory.
func HandleResetTerritory(c *core.RequestEvent, app core.App) error {
	requestInfo, _ := c.RequestInfo()
	userName := c.Auth.Get("name").(string)
	data := requestInfo.Body
	territoryId := data["territory"].(string)

	territory, err := app.FindRecordById("territories", territoryId)
	if err != nil {
		return apis.NewNotFoundError("Territory not found", nil)
	}

	if !AuthorizeByRole(app, c.Auth.Id, territory.GetString("congregation"), "administrator", "conductor") {
		return apis.NewForbiddenError("Administrator or conductor access required", nil)
	}

	records, err := app.FindRecordsByFilter("addresses", "territory = {:id} && (status = 'not_home' || status = 'done')", "", 0, 0, dbx.Params{"id": territoryId})

	if err != nil {
		return apis.NewNotFoundError("Error fetching addresses", nil)
	}

	// Collect affected map IDs before the transaction so Store flags can be set
	// for all of them upfront, suppressing per-address aggregate hook fires.
	mapIDSet := make(map[string]bool)
	for _, record := range records {
		if mapID, ok := record.Get("map").(string); ok && mapID != "" {
			mapIDSet[mapID] = true
		}
	}
	for mapID := range mapIDSet {
		app.Store().Set("bulk_reset:"+mapID, true)
	}
	defer func() {
		for mapID := range mapIDSet {
			app.Store().Remove("bulk_reset:" + mapID)
		}
	}()

	err = app.RunInTransaction(func(txApp core.App) error {
		for _, record := range records {
			record.Set("status", "not_done")
			record.Set("not_home_tries", 0)
			record.Set("updated_by", userName)
			if err := txApp.SaveNoValidate(record); err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		return newServerError(err)
	}

	// Recalculate per-map aggregates for every affected map (skip cascading
	// territory recalc per map; we do it once below).
	for mapID := range mapIDSet {
		if err := ProcessMapAggregates(mapID, app, false); err != nil {
			log.Printf("Error recalculating aggregates for map %s: %v", mapID, err)
		}
	}

	if err := ProcessTerritoryAggregates(territoryId, app); err != nil {
		log.Printf("Error recalculating territory aggregates for %s: %v", territoryId, err)
	}

	return c.JSON(http.StatusOK, "Territory reset successfully")
}
