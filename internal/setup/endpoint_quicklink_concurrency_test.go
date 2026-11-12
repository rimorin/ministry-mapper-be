//go:build testdata

package setup

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"ganigeorgiev.com/pocketbase/pocketbase/apis"
	"ganigeorgiev.com/pocketbase/pocketbase/core"
)

// A group tapping the quicklink at the same moment must be spread over the maps
// exactly as if they had tapped one after another. tests.ApiScenario sends one
// request per app, so this drives the router directly.
func TestQuicklinkConcurrentRequestsSpreadEvenly(t *testing.T) {
	token, err := generateToken("conductor@alpha.test")
	if err != nil {
		t.Fatal(err)
	}

	// testterralpha01 has four maps, none with an active assignment.
	const (
		mapCount = 4
		perMap   = 3
		rounds   = 5
		body     = `{"territory":"testterralpha01","coordinates":{"lat":1.3521,"lng":103.8198},"publisher":"Test Publisher"}`
	)

	for round := 0; round < rounds; round++ {
		app := setupTestApp(t)

		router, err := apis.NewRouter(app)
		if err != nil {
			t.Fatal(err)
		}
		serveEvent := new(core.ServeEvent)
		serveEvent.App = app
		serveEvent.Router = router

		err = app.OnServe().Trigger(serveEvent, func(e *core.ServeEvent) error {
			mux, err := e.Router.BuildMux()
			if err != nil {
				return err
			}

			codes := make([]int, mapCount*perMap)
			start := make(chan struct{})
			var wg sync.WaitGroup
			for i := range codes {
				wg.Add(1)
				go func() {
					defer wg.Done()
					req := httptest.NewRequest(http.MethodPost, "/territory/link", strings.NewReader(body))
					req.Header.Set("Content-Type", "application/json")
					req.Header.Set("Authorization", token)
					rec := httptest.NewRecorder()
					<-start
					mux.ServeHTTP(rec, req)
					codes[i] = rec.Code
				}()
			}
			close(start)
			wg.Wait()

			for i, code := range codes {
				if code != http.StatusOK {
					t.Errorf("round %d request %d: status %d, want 200", round, i, code)
				}
			}

			perMapCounts := []struct {
				Map   string `db:"map"`
				Count int    `db:"count"`
			}{}
			err = app.DB().NewQuery(`
				SELECT a.map AS map, COUNT(*) AS count
				FROM assignments a
				JOIN maps m ON m.id = a.map
				WHERE m.territory = 'testterralpha01' AND a.type = 'normal'
				GROUP BY a.map
			`).All(&perMapCounts)
			if err != nil {
				return err
			}
			if len(perMapCounts) != mapCount {
				t.Errorf("round %d: links landed on %d maps, want %d: %+v", round, len(perMapCounts), mapCount, perMapCounts)
			}
			for _, c := range perMapCounts {
				if c.Count != perMap {
					t.Errorf("round %d: map %s has %d links, want %d", round, c.Map, c.Count, perMap)
				}
			}

			var logged struct {
				Count int `db:"count"`
			}
			if err := app.DB().NewQuery(`SELECT COUNT(*) AS count FROM assignments_log WHERE action = 'assigned'`).One(&logged); err != nil {
				return err
			}
			if logged.Count != len(codes) {
				t.Errorf("round %d: %d assignments_log rows, want %d", round, logged.Count, len(codes))
			}
			return nil
		})
		app.Cleanup()
		if err != nil {
			t.Fatal(err)
		}
	}
}

// A conductor mints links for a group of eight, one after another, standing at
// block A: the common way links are created in production. A and B are within
// 50 m, C is 100 m away and D is 400 m away. The group is spread over the three
// maps within reach, each new link seeing the ones before it, and D is left alone.
func TestQuicklinkGroupSessionThroughEndpoint(t *testing.T) {
	token, err := generateToken("conductor@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	app := setupTestApp(t)
	defer app.Cleanup()

	pins := map[string]float64{
		"testmapalpha01a": 1.3521,   // where the conductor stands
		"testmapalpha01b": 1.35237,  // ~30 m
		"testmapalphsc01": 1.3530,   // ~100 m
		"testmapalphcf01": 1.355697, // ~400 m
	}
	for id, lat := range pins {
		rec, err := app.FindRecordById("maps", id)
		if err != nil {
			t.Fatal(err)
		}
		rec.Set("coordinates", map[string]float64{"lat": lat, "lng": 103.8198})
		if err := app.SaveNoValidate(rec); err != nil {
			t.Fatal(err)
		}
	}

	router, err := apis.NewRouter(app)
	if err != nil {
		t.Fatal(err)
	}
	serveEvent := new(core.ServeEvent)
	serveEvent.App = app
	serveEvent.Router = router

	err = app.OnServe().Trigger(serveEvent, func(e *core.ServeEvent) error {
		mux, err := e.Router.BuildMux()
		if err != nil {
			return err
		}

		var handedOut []string
		for i := range 8 {
			body := fmt.Sprintf(`{"territory":"testterralpha01","coordinates":{"lat":1.3521,"lng":103.8198},"publisher":"Publisher %d"}`, i+1)
			req := httptest.NewRequest(http.MethodPost, "/territory/link", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", token)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("request %d: status %d: %s", i+1, rec.Code, rec.Body.String())
			}
			var out struct {
				MapName   string   `json:"mapName"`
				Assignees []string `json:"assignees"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
			handedOut = append(handedOut, out.MapName)
			// From the fourth link on, the map is shared and the response names who is there.
			if wantShared := i >= 3; wantShared != (len(out.Assignees) > 0) {
				t.Errorf("request %d: assignees = %v, shared map expected: %v", i+1, out.Assignees, wantShared)
			}
		}

		want := []string{
			"Blk 100A", "Blk 100B", "Single Code Blk",
			"Blk 100A", "Blk 100B", "Single Code Blk",
			"Blk 100A", "Blk 100B",
		}
		if fmt.Sprint(handedOut) != fmt.Sprint(want) {
			t.Errorf("maps handed out:\n got  %v\n want %v", handedOut, want)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
