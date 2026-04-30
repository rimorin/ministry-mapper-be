package jobs

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// territoryAggregate holds the batched aggregate result for a single territory.
type territoryAggregate struct {
	Territory        string `db:"territory"`
	NotDone          int    `db:"not_done"`
	Done             int    `db:"done"`
	NotHomeMaxTries  int    `db:"not_home_max_tries"`
	NotHomeLessTries int    `db:"not_home_less_tries"`
	Dnc              int    `db:"dnc"`
	Invalid          int    `db:"invalid"`
}

// mapAggregate holds the batched aggregate result for a single map.
type mapAggregate struct {
	Map              string `db:"map"`
	NotDone          int    `db:"not_done"`
	Done             int    `db:"done"`
	NotHomeMaxTries  int    `db:"not_home_max_tries"`
	NotHomeLessTries int    `db:"not_home_less_tries"`
	Dnc              int    `db:"dnc"`
	Invalid          int    `db:"invalid"`
}

// buildInClause builds a named-parameter IN clause from a slice of string IDs.
// Each param is named "<prefix><i>" (e.g. "m0", "m1") to avoid collisions when
// both map and territory clauses are built in the same function.
// Returns the comma-separated placeholder string and the bound params map.
func buildInClause(ids []string, prefix string) (string, dbx.Params) {
	params := dbx.Params{}
	placeholders := make([]string, len(ids))
	for i, id := range ids {
		key := fmt.Sprintf("%s%d", prefix, i)
		placeholders[i] = "{:" + key + "}"
		params[key] = id
	}
	return strings.Join(placeholders, ", "), params
}

// updateTerritoryAggregates recalculates map and territory aggregate stats for
// any maps/territories that had address status changes in the past
// timeIntervalMinutes window. It uses addresses_log as the change-detection
// source, which is written on every address status update and carries purpose-
// built composite indexes on (map, created) and (territory, created).
//
// Map aggregates (progress + aggregate counts) and territory aggregates
// (progress) are updated in two sequential batched SQL queries — no concurrent
// goroutines, no write contention.
func updateTerritoryAggregates(app core.App, timeIntervalMinutes int) error {
	log.Printf("Starting territory aggregates update (interval: %d minutes)", timeIntervalMinutes)

	cutoff := time.Now().UTC().Add(time.Duration(-timeIntervalMinutes) * time.Minute)

	// Find distinct map IDs with recent address status changes.
	// Uses covering index idx_BOxiQNfT5i (map, created).
	mapIDs := []struct {
		ID string `db:"id"`
	}{}
	err := app.DB().NewQuery(`
		SELECT DISTINCT map AS id
		FROM addresses_log
		WHERE created > {:cutoff} AND map != ''
	`).Bind(dbx.Params{"cutoff": cutoff}).All(&mapIDs)
	if err != nil {
		log.Println("Error fetching dirty maps from addresses_log:", err)
		return err
	}

	// Find distinct territory IDs with recent address status changes.
	// Uses covering index idx_RQh6UExsxs (territory, created).
	territoryIDs := []struct {
		ID string `db:"id"`
	}{}
	err = app.DB().NewQuery(`
		SELECT DISTINCT territory AS id
		FROM addresses_log
		WHERE created > {:cutoff} AND territory != ''
	`).Bind(dbx.Params{"cutoff": cutoff}).All(&territoryIDs)
	if err != nil {
		log.Println("Error fetching dirty territories from addresses_log:", err)
		return err
	}

	if len(mapIDs) == 0 && len(territoryIDs) == 0 {
		log.Println("Completed: No maps or territories to update")
		return nil
	}

	log.Printf("Processing %d maps and %d territories\n", len(mapIDs), len(territoryIDs))

	// -------------------------------------------------------------------------
	// Step 1: Batch-update map aggregates for dirty maps.
	// Single SQL query grouped by map; idx_Fx581hd (map, status) used for
	// address scan; PK lookups for the EXISTS join and congregations JOIN.
	//
	// IDs are processed in chunks of 500 to stay well under SQLite's 999
	// bound-variable limit (SQLITE_MAX_VARIABLE_NUMBER).
	// -------------------------------------------------------------------------
	if len(mapIDs) > 0 {
		mapResultIndex := make(map[string]*mapAggregate, len(mapIDs))
		for chunkStart := 0; chunkStart < len(mapIDs); chunkStart += 500 {
			chunkEnd := chunkStart + 500
			if chunkEnd > len(mapIDs) {
				chunkEnd = len(mapIDs)
			}
			chunk := mapIDs[chunkStart:chunkEnd]

			chunkIDs := make([]string, len(chunk))
			for i, item := range chunk { chunkIDs[i] = item.ID }
			mapInClause, mapParams := buildInClause(chunkIDs, "m")

			var chunkResults []mapAggregate
			err = app.DB().NewQuery(fmt.Sprintf(`
				SELECT
					a.map,
					COALESCE(SUM(CASE WHEN a.status = 'not_done' THEN 1 ELSE 0 END), 0) AS not_done,
					COALESCE(SUM(CASE WHEN a.status = 'done' THEN 1 ELSE 0 END), 0) AS done,
					COALESCE(SUM(CASE WHEN a.status = 'not_home' AND a.not_home_tries >= c.max_tries THEN 1 ELSE 0 END), 0) AS not_home_max_tries,
					COALESCE(SUM(CASE WHEN a.status = 'not_home' AND a.not_home_tries < c.max_tries THEN 1 ELSE 0 END), 0) AS not_home_less_tries,
					COALESCE(SUM(CASE WHEN a.status = 'do_not_call' THEN 1 ELSE 0 END), 0) AS dnc,
					COALESCE(SUM(CASE WHEN a.status = 'invalid' THEN 1 ELSE 0 END), 0) AS invalid
				FROM addresses a
				LEFT JOIN congregations c ON a.congregation = c.id
				WHERE a.map IN (%s)
				AND EXISTS (
					SELECT 1
					FROM address_options ao
					JOIN options o ON ao.option = o.id
					WHERE ao.address = a.id
					AND ao.map = a.map
					AND o.is_countable = TRUE
				)
				AND a.status IN ('done', 'not_done', 'do_not_call', 'invalid', 'not_home')
				GROUP BY a.map
			`, mapInClause)).Bind(mapParams).All(&chunkResults)
			if err != nil {
				log.Printf("Error running batched map aggregate query (chunk %d-%d): %v", chunkStart, chunkEnd, err)
				return err
			}
			for i := range chunkResults {
				mapResultIndex[chunkResults[i].Map] = &chunkResults[i]
			}
		}

		var mapErrors int
		for _, m := range mapIDs {
			aggregates := map[string]interface{}{
				"notDone": 0,
				"done":    0,
				"notHome": 0,
				"dnc":     0,
				"invalid": 0,
			}
			progress := 0
			if agg, ok := mapResultIndex[m.ID]; ok {
				aggregates["notDone"] = agg.NotDone
				aggregates["done"] = agg.Done
				aggregates["notHome"] = agg.NotHomeLessTries
				aggregates["dnc"] = agg.Dnc
				aggregates["invalid"] = agg.Invalid
				total := agg.Done + agg.NotDone + agg.NotHomeMaxTries + agg.NotHomeLessTries
				if total > 0 {
					progress = int(float64(agg.Done+agg.NotHomeMaxTries) / float64(total) * 100)
				}
			}

			mapRecord, err := app.FindRecordById("maps", m.ID)
			if err != nil {
				log.Printf("Error finding map record %s: %v", m.ID, err)
				mapErrors++
				continue
			}
			mapRecord.Set("aggregates", aggregates)
			mapRecord.Set("progress", progress)
			if err := app.SaveNoValidate(mapRecord); err != nil {
				log.Printf("Error saving map record %s: %v", m.ID, err)
				mapErrors++
				continue
			}
		}

		if mapErrors > 0 {
			log.Printf("Map aggregates completed with %d errors out of %d maps", mapErrors, len(mapIDs))
		} else {
			log.Printf("Map aggregates updated for %d maps", len(mapIDs))
		}
	}

	// -------------------------------------------------------------------------
	// Step 2: Batch-update territory aggregates for dirty territories.
	// IDs are also chunked in batches of 500 for the same reason as Step 1.
	// -------------------------------------------------------------------------
	if len(territoryIDs) > 0 {
		territoryResultIndex := make(map[string]*territoryAggregate, len(territoryIDs))
		for chunkStart := 0; chunkStart < len(territoryIDs); chunkStart += 500 {
			chunkEnd := chunkStart + 500
			if chunkEnd > len(territoryIDs) {
				chunkEnd = len(territoryIDs)
			}
			chunk := territoryIDs[chunkStart:chunkEnd]

			chunkIDs := make([]string, len(chunk))
			for i, item := range chunk { chunkIDs[i] = item.ID }
			inClause, params := buildInClause(chunkIDs, "t")

			var chunkResults []territoryAggregate
			err = app.DB().NewQuery(fmt.Sprintf(`
				SELECT
					a.territory,
					COALESCE(SUM(CASE WHEN a.status = 'not_done' THEN 1 ELSE 0 END), 0) AS not_done,
					COALESCE(SUM(CASE WHEN a.status = 'done' THEN 1 ELSE 0 END), 0) AS done,
					COALESCE(SUM(CASE WHEN a.status = 'not_home' AND a.not_home_tries >= c.max_tries THEN 1 ELSE 0 END), 0) AS not_home_max_tries,
					COALESCE(SUM(CASE WHEN a.status = 'not_home' AND a.not_home_tries < c.max_tries THEN 1 ELSE 0 END), 0) AS not_home_less_tries,
					COALESCE(SUM(CASE WHEN a.status = 'do_not_call' THEN 1 ELSE 0 END), 0) AS dnc,
					COALESCE(SUM(CASE WHEN a.status = 'invalid' THEN 1 ELSE 0 END), 0) AS invalid
				FROM addresses a
				LEFT JOIN congregations c ON a.congregation = c.id
				WHERE a.territory IN (%s)
				AND EXISTS (
					SELECT 1
					FROM address_options ao
					JOIN options o ON ao.option = o.id
					WHERE ao.address = a.id
					AND ao.map = a.map
					AND o.is_countable = TRUE
				)
				AND a.status IN ('done', 'not_done', 'do_not_call', 'invalid', 'not_home')
				GROUP BY a.territory
			`, inClause)).Bind(params).All(&chunkResults)
			if err != nil {
				log.Printf("Error running batched territory aggregate query (chunk %d-%d): %v", chunkStart, chunkEnd, err)
				return err
			}
			for i := range chunkResults {
				territoryResultIndex[chunkResults[i].Territory] = &chunkResults[i]
			}
		}

		var updateErrors int
		for _, t := range territoryIDs {
			progress := 0
			if agg, ok := territoryResultIndex[t.ID]; ok {
				total := agg.Done + agg.NotDone + agg.NotHomeMaxTries + agg.NotHomeLessTries
				if total > 0 {
					progress = int(float64(agg.Done+agg.NotHomeMaxTries) / float64(total) * 100)
				}
			}

			territoryRecord, err := app.FindRecordById("territories", t.ID)
			if err != nil {
				log.Printf("Error finding territory record %s: %v", t.ID, err)
				updateErrors++
				continue
			}
			territoryRecord.Set("progress", progress)
			if err := app.SaveNoValidate(territoryRecord); err != nil {
				log.Printf("Error saving territory record %s: %v", t.ID, err)
				updateErrors++
				continue
			}
		}

		if updateErrors > 0 {
			log.Printf("Territory aggregates completed with %d errors", updateErrors)
		} else {
			log.Println("Territory aggregates update completed")
		}
	}

	return nil
}
