//go:build testdata

package setup

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// raceMapAddresses is large enough that a recalculation takes long enough for
// two of them to overlap; on a five-address seed map the window is too narrow
// for the lost update to ever show.
const raceMapAddresses = 2000

// raceRounds is a probability budget, not a constant of nature. A single round
// reproduces the lost update roughly a third of the time, so the run as a whole
// fails well over 99% of the time when the recalculation is unsynchronised.
const raceRounds = 10

// raceBatch is how many addresses change at once — a busy map worked by several
// publishers at the same time.
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
// at one map and checks the stored aggregate against a direct count.
//
// ProcessMapAggregates counts the map and then writes the result as two separate
// steps. Without synchronisation two of them interleave, the slower one writes a
// count it read before the other's changes landed, and the map keeps a wrong
// progress figure until the next unrelated update recalculates it.
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
