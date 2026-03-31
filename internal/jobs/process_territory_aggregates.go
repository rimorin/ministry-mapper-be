package jobs

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
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

// updateTerritoryAggregates updates the territory aggregates based on the specified time interval.
// It finds territories whose maps were recently updated and recalculates their progress
// percentages using a single batched SQL query instead of per-territory queries.
//
// Parameters:
// - app: A pointer to the PocketBase application instance.
// - timeIntervalMinutes: The time interval in minutes to look back for updated territories.
//
// Returns:
// - error: An error if any occurs during the process, otherwise nil.
func updateTerritoryAggregates(app *pocketbase.PocketBase, timeIntervalMinutes int) error {
	log.Printf("Starting territory aggregates update (interval: %d minutes)", timeIntervalMinutes)

	timeBuffer := time.Duration(-timeIntervalMinutes) * time.Minute
	cutoff := time.Now().UTC().Add(timeBuffer)

	// Find distinct territory IDs with recently updated maps.
	territoryIDs := []struct {
		ID string `db:"id"`
	}{}
	err := app.DB().Select("territories.id").Distinct(true).From("territories").
		InnerJoin("maps", dbx.NewExp("maps.territory = territories.id")).
		Where(dbx.NewExp("maps.updated > {:updated}", dbx.Params{"updated": cutoff})).
		All(&territoryIDs)
	if err != nil {
		log.Println("Error fetching territories:", err)
		return err
	}

	if len(territoryIDs) == 0 {
		log.Println("Completed: No territories found during query")
		return nil
	}

	// Collect IDs and build individual bind params for the IN clause.
	// dbx.NewQuery doesn't expand slice params, so we create {:t0}, {:t1}, etc.
	params := dbx.Params{}
	placeholders := make([]string, len(territoryIDs))
	for i, t := range territoryIDs {
		key := fmt.Sprintf("t%d", i)
		placeholders[i] = "{:" + key + "}"
		params[key] = t.ID
	}
	inClause := strings.Join(placeholders, ", ")

	log.Printf("Processing %d territories\n", len(territoryIDs))

	// Run a single aggregate query for all target territories at once.
	var results []territoryAggregate
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
	`, inClause)).Bind(params).All(&results)
	if err != nil {
		log.Printf("Error running batched aggregate query: %v", err)
		return err
	}

	// Index results by territory ID for quick lookup.
	resultMap := make(map[string]*territoryAggregate, len(results))
	for i := range results {
		resultMap[results[i].Territory] = &results[i]
	}

	// Update each territory record. Territories absent from results
	// (e.g. zero countable addresses) get progress = 0.
	var updateErrors int
	for _, t := range territoryIDs {
		progress := 0
		if agg, ok := resultMap[t.ID]; ok {
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
		if err := app.Save(territoryRecord); err != nil {
			log.Printf("Error saving territory record %s: %v", t.ID, err)
			updateErrors++
			continue
		}

		log.Printf("Updated territory record %s with progress %d%%", t.ID, progress)
	}

	if updateErrors > 0 {
		log.Printf("Territory aggregates update completed with %d errors", updateErrors)
	} else {
		log.Println("Territory aggregates update completed")
	}
	return nil
}
