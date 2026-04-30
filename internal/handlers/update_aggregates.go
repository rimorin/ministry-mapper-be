package handlers

import (
	"log"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

type Aggregates struct {
	NotDone          int `db:"not_done"`
	Done             int `db:"done"`
	NotHomeMaxTries  int `db:"not_home_max_tries"`
	NotHomeLessTries int `db:"not_home_less_tries"`
	Dnc              int `db:"dnc"`
	Invalid          int `db:"invalid"`
}

// ProcessMapAggregates updates the aggregate data for a given map in the PocketBase application.
// It calculates various status counts for addresses associated with the map and updates the map record
// with these aggregates and the progress percentage.
//
// Parameters:
//   - mapID: The ID of the map to process aggregates for. Must not be empty.
//   - app: The PocketBase application instance.
//   - resetTerritoryAggregates: Optional boolean parameter to determine whether to reset territory aggregates.
//     Defaults to true if not provided.
//
// Returns:
//   - error: An error if any issues occur during processing, otherwise nil.
func ProcessMapAggregates(mapID string, app core.App, resetTerritoryAggregates ...bool) error {
	if mapID == "" {
		return apis.NewBadRequestError("Map ID is required", nil)
	}

	aggregates := Aggregates{}
	err := app.DB().NewQuery(`
        SELECT
			COALESCE(SUM(CASE WHEN a.status = 'not_done' THEN 1 ELSE 0 END), 0) AS not_done,
			COALESCE(SUM(CASE WHEN a.status = 'done' THEN 1 ELSE 0 END), 0) AS done,
			COALESCE(SUM(CASE WHEN a.status = 'not_home' AND a.not_home_tries >= c.max_tries THEN 1 ELSE 0 END), 0) AS not_home_max_tries,
			COALESCE(SUM(CASE WHEN a.status = 'not_home' AND a.not_home_tries < c.max_tries THEN 1 ELSE 0 END), 0) AS not_home_less_tries,
			COALESCE(SUM(CASE WHEN a.status = 'do_not_call' THEN 1 ELSE 0 END), 0) AS dnc,
			COALESCE(SUM(CASE WHEN a.status = 'invalid' THEN 1 ELSE 0 END), 0) AS invalid
        FROM addresses a
        LEFT JOIN congregations c ON a.congregation = c.id
        WHERE EXISTS (
            SELECT 1
            FROM address_options ao
            JOIN options o ON ao.option = o.id
            WHERE ao.address = a.id
            AND ao.map = {:map}
            AND o.is_countable = TRUE
        )
        AND a.status IN ('done', 'not_done', 'do_not_call', 'invalid', 'not_home')
        AND a.map = {:map}
    `).Bind(dbx.Params{"map": mapID}).One(&aggregates)
	if err != nil {
		log.Printf("Error finding records by filter for mapID %s: %v", mapID, err)
		return err
	}

	total := aggregates.Done + aggregates.NotDone + aggregates.NotHomeMaxTries + aggregates.NotHomeLessTries

	donePercentage := 0
	if total > 0 {
		donePercentage = int(float64(aggregates.Done+aggregates.NotHomeMaxTries) / float64(total) * 100)
	}

	amap := map[string]interface{}{
		"notDone": aggregates.NotDone,
		"done":    aggregates.Done,
		"notHome": aggregates.NotHomeLessTries,
		"invalid": aggregates.Invalid,
		"dnc":     aggregates.Dnc,
	}

	mapRecord, err := app.FindRecordById("maps", mapID)
	if err != nil {
		log.Printf("Error finding map record by ID %s: %v", mapID, err)
		return err
	}

	mapRecord.Set("aggregates", amap)
	mapRecord.Set("progress", donePercentage)

	if err := app.SaveNoValidate(mapRecord); err != nil {
		log.Printf("Error saving map record for mapID %s: %v", mapID, err)
		return err
	}

	// Set default value for resetTerritory
	reset := true
	if len(resetTerritoryAggregates) > 0 {
		reset = resetTerritoryAggregates[0]
	}

	if reset {
		// mapRecord is already loaded above — extract territory directly to avoid
		// a redundant FindRecordById("maps") call inside ResetMapTerritory.
		if territoryID, ok := mapRecord.Get("territory").(string); ok && territoryID != "" {
			ProcessTerritoryAggregates(territoryID, app)
		}
	}

	return nil
}

// ProcessTerritoryAggregates processes the aggregate data for a given territory and updates the progress percentage.
//
// Parameters:
//   - territoryID: The ID of the territory to process aggregates for.
//   - app: The PocketBase application instance.
//
// Returns:
//   - error: An error if any occurs during the processing of aggregates or updating the territory record.
//
// The function performs the following steps:
//  1. Logs the start of the processing for the given territoryID.
//  2. Executes a SQL query to calculate various aggregate counts (not_done, done, not_home_max_tries, not_home_less_tries, dnc, invalid) for the given territory.
//  3. Calculates the total count of relevant statuses and computes the done percentage.
//  4. Retrieves the territory record by its ID.
//  5. Updates the territory record with the calculated progress percentage.
//  6. Saves the updated territory record.
//  7. Logs the completion of the update with the progress percentage.
func ProcessTerritoryAggregates(territoryID string, app core.App) error {
	aggregates := Aggregates{}
	err := app.DB().NewQuery(`
		SELECT
			COALESCE(SUM(CASE WHEN a.status = 'not_done' THEN 1 ELSE 0 END), 0) AS not_done,
			COALESCE(SUM(CASE WHEN a.status = 'done' THEN 1 ELSE 0 END), 0) AS done,
			COALESCE(SUM(CASE WHEN a.status = 'not_home' AND a.not_home_tries >= c.max_tries THEN 1 ELSE 0 END), 0) AS not_home_max_tries,
			COALESCE(SUM(CASE WHEN a.status = 'not_home' AND a.not_home_tries < c.max_tries THEN 1 ELSE 0 END), 0) AS not_home_less_tries,
			COALESCE(SUM(CASE WHEN a.status = 'do_not_call' THEN 1 ELSE 0 END), 0) AS dnc,
			COALESCE(SUM(CASE WHEN a.status = 'invalid' THEN 1 ELSE 0 END), 0) AS invalid
		FROM addresses a
		LEFT JOIN congregations c ON a.congregation = c.id
		WHERE EXISTS (
			SELECT 1
			FROM address_options ao
			JOIN options o ON ao.option = o.id
			WHERE ao.address = a.id
			AND ao.congregation = a.congregation
			AND o.is_countable = TRUE
		)
		AND a.status IN ('done', 'not_done', 'do_not_call', 'invalid', 'not_home')
		AND a.territory = {:territory}
	`).Bind(dbx.Params{"territory": territoryID}).One(&aggregates)
	if err != nil {
		log.Printf("Error finding records by filter for territoryID %s: %v", territoryID, err)
		return err
	}

	total := aggregates.Done + aggregates.NotDone + aggregates.NotHomeMaxTries + aggregates.NotHomeLessTries

	donePercentage := 0
	if total > 0 {
		donePercentage = int(float64(aggregates.Done+aggregates.NotHomeMaxTries) / float64(total) * 100)
	}

	territoryRecord, err := app.FindRecordById("territories", territoryID)
	if err != nil {
		log.Printf("Error finding territory record by ID %s: %v", territoryID, err)
		return err
	}

	territoryRecord.Set("progress", donePercentage)

	if err := app.SaveNoValidate(territoryRecord); err != nil {
		log.Printf("Error saving territory record for territoryID %s: %v", territoryID, err)
		return err
	}

	return nil
}
