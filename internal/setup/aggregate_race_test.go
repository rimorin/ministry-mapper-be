//go:build testdata

package setup

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ministry-mapper/internal/handlers"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// raceMapAddresses makes a recalculation slow enough for two to overlap; the
// five-address seed map is far too small for the lost update to ever show.
const raceMapAddresses = 2000

// raceRounds is a probability budget: one round reproduces the lost update
// about a third of the time, so ten put the run above 99%.
const raceRounds = 10

// raceBatch is a map worked by several publishers at once.
const raceBatch = 30

func seedRaceMap(t testing.TB, app *tests.TestApp) (mapID string, addressIDs []string) {
	t.Helper()

	mapID = "racemapalpha001"
	now := time.Now().UTC().Format("2006-01-02 15:04:05.000Z")

	err := app.RunInTransaction(func(txApp core.App) error {
		if _, err := txApp.DB().NewQuery(`
			INSERT INTO maps (id, congregation, territory, sequence, code, description, progress, type, created, updated)
			VALUES ({:id}, 'testcongalpha01', 'testterralpha01', 900, 'RACE', 'Race Block', 0, 'single', {:now}, {:now})
		`).Bind(dbx.Params{"id": mapID, "now": now}).Execute(); err != nil {
			return err
		}
		for i := 0; i < raceMapAddresses; i++ {
			id := fmt.Sprintf("raceaddr%07d", i)
			addressIDs = append(addressIDs, id)
			if _, err := txApp.DB().NewQuery(`
				INSERT INTO addresses (id, congregation, territory, map, floor, code, status, sequence, not_home_tries, created, updated)
				VALUES ({:id}, 'testcongalpha01', 'testterralpha01', {:map}, 1, {:code}, 'not_done', {:seq}, 0, {:now}, {:now})
			`).Bind(dbx.Params{"id": id, "map": mapID, "code": fmt.Sprint(i), "seq": i, "now": now}).Execute(); err != nil {
				return err
			}
			if _, err := txApp.DB().NewQuery(`
				INSERT INTO address_options (id, address, option, congregation, map, created, updated)
				VALUES ({:id}, {:addr}, 'testoptialpha01', 'testcongalpha01', {:map}, {:now}, {:now})
			`).Bind(dbx.Params{"id": fmt.Sprintf("raceopt%08d", i), "addr": id, "map": mapID, "now": now}).Execute(); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("seed race map: %v", err)
	}
	return mapID, addressIDs
}

// actualDone counts the rows directly — the answer the stored aggregate should hold.
func actualDone(t testing.TB, app *tests.TestApp, mapID string) int {
	t.Helper()
	var row struct {
		V int `db:"v"`
	}
	if err := app.DB().NewQuery(`SELECT COUNT(*) AS v FROM addresses WHERE map = {:map} AND status = 'done'`).
		Bind(dbx.Params{"map": mapID}).One(&row); err != nil {
		t.Fatalf("count addresses: %v", err)
	}
	return row.V
}

func storedDone(t testing.TB, app *tests.TestApp, mapID string) int {
	t.Helper()
	record, err := app.FindRecordById("maps", mapID)
	if err != nil {
		t.Fatalf("find map: %v", err)
	}
	var stored struct {
		Done int `json:"done"`
	}
	raw := record.GetString("aggregates")
	if raw == "" {
		return -1
	}
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		t.Fatalf("unmarshal aggregates %q: %v", raw, err)
	}
	return stored.Done
}

// waitQuiescent blocks until the map record has stopped being written for a
// short settle window, i.e. every fire-and-forget recalculation has finished.
func waitQuiescent(t testing.TB, app *tests.TestApp, mapID string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	var last string
	stable := 0
	for time.Now().Before(deadline) {
		record, err := app.FindRecordById("maps", mapID)
		if err != nil {
			t.Fatalf("find map: %v", err)
		}
		current := record.GetString("updated")
		if current == last {
			if stable++; stable >= 5 {
				return
			}
		} else {
			stable = 0
			last = current
		}
		time.Sleep(40 * time.Millisecond)
	}
	t.Fatal("map aggregates never settled")
}

// TestAggregateConcurrency_StoredMatchesActual drives concurrent status changes
// at one map and checks the stored aggregate against a direct count. It fails
// without aggregateJob.run.
func TestAggregateConcurrency_StoredMatchesActual(t *testing.T) {
	app := setupTestApp(t)
	defer app.Cleanup()

	mapID, addressIDs := seedRaceMap(t, app)

	for round := 1; round <= raceRounds; round++ {
		status := "done"
		if round%2 == 0 {
			status = "not_done"
		}

		var wg sync.WaitGroup
		for _, id := range addressIDs[:raceBatch] {
			wg.Add(1)
			go func(addressID string) {
				defer wg.Done()
				record, err := app.FindRecordById("addresses", addressID)
				if err != nil {
					return
				}
				record.Set("status", status)
				if err := app.SaveNoValidate(record); err != nil {
					t.Errorf("save address %s: %v", addressID, err)
				}
			}(id)
		}
		wg.Wait()
		waitQuiescent(t, app, mapID)

		want := actualDone(t, app, mapID)
		got := storedDone(t, app, mapID)
		if got != want {
			t.Fatalf("round %d: stored aggregate is stale — maps.aggregates.done = %d, actual rows with status 'done' = %d (drift %+d)",
				round, got, want, got-want)
		}
	}
}

// flipConcurrently changes the status of the first raceBatch addresses at the
// same time, the way several publishers working one map do.
func flipConcurrently(t testing.TB, app *tests.TestApp, addressIDs []string, status string) {
	t.Helper()
	var wg sync.WaitGroup
	for _, id := range addressIDs[:raceBatch] {
		wg.Add(1)
		go func(addressID string) {
			defer wg.Done()
			record, err := app.FindRecordById("addresses", addressID)
			if err != nil {
				return
			}
			record.Set("status", status)
			if err := app.SaveNoValidate(record); err != nil {
				t.Errorf("save address %s: %v", addressID, err)
			}
		}(id)
	}
	wg.Wait()
}

// TestAggregateCoalescing_CollapsesRedundantRecalculations checks that a burst of
// status changes on one map does not produce one map write, and one realtime
// event, per address.
func TestAggregateCoalescing_CollapsesRedundantRecalculations(t *testing.T) {
	app := setupTestApp(t)
	defer app.Cleanup()

	mapID, addressIDs := seedRaceMap(t, app)

	// Prime the stored aggregates so the burst below is measured against a
	// map that is already up to date.
	if err := handlers.ProcessMapAggregates(mapID, app); err != nil {
		t.Fatalf("prime aggregates: %v", err)
	}

	var mapWrites atomic.Int64
	app.OnRecordAfterUpdateSuccess("maps").BindFunc(func(e *core.RecordEvent) error {
		if e.Record.Id == mapID {
			mapWrites.Add(1)
		}
		return e.Next()
	})

	flipConcurrently(t, app, addressIDs, "done")

	// Settle on write activity rather than on the updated timestamp: a
	// recalculation that finds nothing changed writes nothing at all.
	stable := 0
	last := int64(-1)
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		current := mapWrites.Load()
		if current == last {
			if stable++; stable >= 8 {
				break
			}
		} else {
			stable = 0
			last = current
		}
		time.Sleep(40 * time.Millisecond)
	}

	writes := mapWrites.Load()
	t.Logf("%d address changes produced %d map writes", raceBatch, writes)

	if writes == 0 {
		t.Fatal("no map write at all — the burst should have moved the aggregate once")
	}
	if writes > raceBatch/3 {
		t.Errorf("recalculations were not collapsed: %d address changes produced %d map writes", raceBatch, writes)
	}

	if got, want := storedDone(t, app, mapID), actualDone(t, app, mapID); got != want {
		t.Errorf("stored aggregate wrong after burst: got %d, want %d", got, want)
	}
}
