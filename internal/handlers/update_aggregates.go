package handlers

import (
	"log"
	"math"
	"sync"

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

// aggregateLock returns the lock that serialises recalculations of a single map
// or territory.
//
// Counting and storing are two separate steps: the aggregate query runs against
// the concurrent read pool, then the result is saved. Two recalculations of the
// same map therefore interleave, and the one that finishes last stores a count it
// read before the other's changes landed — leaving a wrong progress figure until
// some later update happens to recalculate it again.
//
// The locks live in app.Store() rather than a package-level map so they are
// scoped to the app instance, which matters for tests that build one app per
// case. RunInTransaction shallow-clones the app, so the transactional clone
// shares the same store and therefore the same locks. One mutex per map and
// territory is retained for the process lifetime, which is a few bytes each.
func aggregateLock(app core.App, key string) *sync.Mutex {
	return app.Store().GetOrSet("aggregate_lock:"+key, func() any {
		return &sync.Mutex{}
	}).(*sync.Mutex)
}

// ProcessMapAggregates recalculates a map's status counts and progress percentage.
// resetTerritoryAggregates (default true) controls whether the map's territory
// aggregates are also recalculated.
func ProcessMapAggregates(mapID string, app core.App, resetTerritoryAggregates ...bool) error {
	if mapID == "" {
		return apis.NewBadRequestError("Map ID is required", nil)
	}

	lock := aggregateLock(app, "map:"+mapID)
	lock.Lock()
	defer lock.Unlock()

	aggregates := Aggregates{}
	err := app.DB().NewQuery(`
        SELECT
			COALESCE(SUM(CASE WHEN a.status = 'not_done' THEN 1 ELSE 0 END), 0) AS not_done,
			COALESCE(SUM(CASE WHEN a.status = 'done' THEN 1 ELSE 0 END), 0) AS done,
			COALESCE(SUM(CASE WHEN a.status = 'not_home' AND c.max_tries > 0 AND a.not_home_tries >= c.max_tries THEN 1 ELSE 0 END), 0) AS not_home_max_tries,
			COALESCE(SUM(CASE WHEN a.status = 'not_home' AND (c.max_tries <= 0 OR a.not_home_tries < c.max_tries) THEN 1 ELSE 0 END), 0) AS not_home_less_tries,
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
		donePercentage = int(math.Round(float64(aggregates.Done+aggregates.NotHomeMaxTries) / float64(total) * 100))
	}

	amap := map[string]interface{}{
		"notDone":   aggregates.NotDone,
		"done":      aggregates.Done,
		"notHome":   aggregates.NotHomeLessTries,
		"invalid":   aggregates.Invalid,
		"dnc":       aggregates.Dnc,
		"completed": aggregates.Done + aggregates.NotHomeMaxTries,
		"total":     total,
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

	reset := true
	if len(resetTerritoryAggregates) > 0 {
		reset = resetTerritoryAggregates[0]
	}

	if reset {
		if territoryID, ok := mapRecord.Get("territory").(string); ok && territoryID != "" {
			ProcessTerritoryAggregates(territoryID, app)
		}
	}

	return nil
}

// ProcessTerritoryAggregates recalculates a territory's progress percentage
// by summing the completed/total values stored in each map's aggregates.
func ProcessTerritoryAggregates(territoryID string, app core.App) error {
	lock := aggregateLock(app, "territory:"+territoryID)
	lock.Lock()
	defer lock.Unlock()

	progress := struct {
		Completed int `db:"completed"`
		Total     int `db:"total"`
	}{}
	err := app.DB().NewQuery(`
		SELECT
			COALESCE(SUM(json_extract(COALESCE(NULLIF(aggregates, ''), '{}'), '$.completed')), 0) AS completed,
			COALESCE(SUM(json_extract(COALESCE(NULLIF(aggregates, ''), '{}'), '$.total')), 0) AS total
		FROM maps
		WHERE territory = {:territory}
	`).Bind(dbx.Params{"territory": territoryID}).One(&progress)
	if err != nil {
		log.Printf("Error finding records by filter for territoryID %s: %v", territoryID, err)
		return err
	}

	donePercentage := 0
	if progress.Total > 0 {
		donePercentage = int(math.Round(float64(progress.Completed) / float64(progress.Total) * 100))
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
