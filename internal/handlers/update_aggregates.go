package handlers

import (
	"encoding/json"
	"log"
	"math"
	"sync"
	"sync/atomic"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/routine"
)

type Aggregates struct {
	NotDone          int `db:"not_done"`
	Done             int `db:"done"`
	NotHomeMaxTries  int `db:"not_home_max_tries"`
	NotHomeLessTries int `db:"not_home_less_tries"`
	Dnc              int `db:"dnc"`
	Invalid          int `db:"invalid"`
}

// storedAggregates is the shape written to maps.aggregates. The field names are
// part of the contract: ProcessTerritoryAggregates reads completed and total out
// of it with json_extract, and the frontend reads the rest.
type storedAggregates struct {
	NotDone   int `json:"notDone"`
	Done      int `json:"done"`
	NotHome   int `json:"notHome"`
	Invalid   int `json:"invalid"`
	Dnc       int `json:"dnc"`
	Completed int `json:"completed"`
	Total     int `json:"total"`
}

// aggregateJob coordinates recalculations of a single map or territory.
//
// run serialises them. Counting and storing are two separate steps, so two
// concurrent recalculations interleave and the one that finishes last stores a
// count it read before the other's changes landed.
//
// pending collapses the queue. A recalculation counts the map as it is at the
// moment it runs, so several queued ones would all reach the same answer; at
// most one waits behind the lock and the rest are dropped. The slot is released
// on acquiring the lock, before counting starts, so an update arriving during a
// recalculation still queues the next one.
//
// The jobs live in app.Store(), not a package-level map, so they are scoped to
// the app instance; RunInTransaction shallow-clones the app, so a transactional
// clone shares them.
type aggregateJob struct {
	run     sync.Mutex
	pending atomic.Int32
}

func aggregateJobFor(app core.App, key string) *aggregateJob {
	return app.Store().GetOrSet("aggregate_job:"+key, func() any {
		return &aggregateJob{}
	}).(*aggregateJob)
}

// ProcessMapAggregates recalculates a map's status counts and progress percentage.
// resetTerritoryAggregates (default true) controls whether the map's territory
// aggregates are also recalculated.
func ProcessMapAggregates(mapID string, app core.App, resetTerritoryAggregates ...bool) error {
	if mapID == "" {
		return apis.NewBadRequestError("Map ID is required", nil)
	}

	job := aggregateJobFor(app, "map:"+mapID)
	job.run.Lock()
	defer job.run.Unlock()

	return processMapAggregates(mapID, app, resetTerritoryAggregates...)
}

// ScheduleMapAggregates recalculates the map in the background and drops the
// request when one is already queued for it. Used by the per-address hook, where
// a map worked by several publishers would otherwise trigger one full recount
// per address updated.
func ScheduleMapAggregates(mapID string, app core.App) {
	if mapID == "" {
		return
	}

	job := aggregateJobFor(app, "map:"+mapID)
	if !job.pending.CompareAndSwap(0, 1) {
		// The queued one has not started counting yet, and the address row is
		// already committed by the time this hook runs, so it will be included.
		return
	}

	routine.FireAndForget(func() {
		job.run.Lock()
		job.pending.Store(0)
		defer job.run.Unlock()

		if err := processMapAggregates(mapID, app); err != nil {
			app.Logger().Error("aggregate recalculation failed", "map", mapID, "err", err)
		}
	})
}

// processMapAggregates is ProcessMapAggregates without the lock. Callers must
// already hold the map's job lock.
func processMapAggregates(mapID string, app core.App, resetTerritoryAggregates ...bool) error {
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

	next := storedAggregates{
		NotDone:   aggregates.NotDone,
		Done:      aggregates.Done,
		NotHome:   aggregates.NotHomeLessTries,
		Invalid:   aggregates.Invalid,
		Dnc:       aggregates.Dnc,
		Completed: aggregates.Done + aggregates.NotHomeMaxTries,
		Total:     total,
	}

	mapRecord, err := app.FindRecordById("maps", mapID)
	if err != nil {
		log.Printf("Error finding map record by ID %s: %v", mapID, err)
		return err
	}

	var current storedAggregates
	if raw := mapRecord.GetString("aggregates"); raw != "" {
		// A value that will not parse is treated as different, so it gets rewritten.
		_ = json.Unmarshal([]byte(raw), &current)
	}
	if current == next && mapRecord.GetInt("progress") == donePercentage {
		// Nothing moved: skip the write, the realtime event it would broadcast,
		// and the territory rollup that follows it.
		return nil
	}

	mapRecord.Set("aggregates", next)
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
	job := aggregateJobFor(app, "territory:"+territoryID)
	job.run.Lock()
	defer job.run.Unlock()

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

	if territoryRecord.GetInt("progress") == donePercentage {
		return nil
	}

	territoryRecord.Set("progress", donePercentage)

	if err := app.SaveNoValidate(territoryRecord); err != nil {
		log.Printf("Error saving territory record for territoryID %s: %v", territoryID, err)
		return err
	}

	return nil
}
