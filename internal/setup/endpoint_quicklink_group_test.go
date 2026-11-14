//go:build testdata

package setup

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"ganigeorgiev.com/pocketbase/pocketbase/apis"
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"ganigeorgiev.com/pocketbase/pocketbase/tests"
)

// tests.ApiScenario sends one request per app. A group requesting links needs
// several against the same one, so these tests drive the router directly.
func serveQuicklinks(t *testing.T, app *tests.TestApp, fn func(post func(publisher string) *httptest.ResponseRecorder)) {
	t.Helper()
	token, err := generateToken("conductor@alpha.test")
	if err != nil {
		t.Fatal(err)
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
		fn(func(publisher string) *httptest.ResponseRecorder {
			body := fmt.Sprintf(`{"territory":"testterralpha01","coordinates":{"lat":1.3521,"lng":103.8198},"publisher":%q}`, publisher)
			req := httptest.NewRequest(http.MethodPost, "/territory/link", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", token)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			return rec
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// A group tapping the quicklink at the same moment must be spread over the maps
// exactly as if they had tapped one after another.
func TestQuicklinkConcurrentRequestsSpreadEvenly(t *testing.T) {
	// testterralpha01 has four maps, none with an active assignment.
	const mapCount, perMap, rounds = 4, 3, 5

	for round := range rounds {
		app := setupTestApp(t)
		serveQuicklinks(t, app, func(post func(string) *httptest.ResponseRecorder) {
			codes := make([]int, mapCount*perMap)
			start := make(chan struct{})
			var wg sync.WaitGroup
			for i := range codes {
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-start
					codes[i] = post("Test Publisher").Code
				}()
			}
			close(start)
			wg.Wait()

			for i, code := range codes {
				if code != http.StatusOK {
					t.Errorf("round %d request %d: status %d, want 200", round, i, code)
				}
			}

			links := []struct {
				Map   string `db:"map"`
				Count int    `db:"count"`
			}{}
			err := app.DB().NewQuery(`
				SELECT a.map AS map, COUNT(*) AS count
				FROM assignments a
				JOIN maps m ON m.id = a.map
				WHERE m.territory = 'testterralpha01' AND a.type = 'normal'
				GROUP BY a.map
			`).All(&links)
			if err != nil {
				t.Fatal(err)
			}
			if len(links) != mapCount {
				t.Errorf("round %d: links landed on %d maps, want %d: %+v", round, len(links), mapCount, links)
			}
			for _, l := range links {
				if l.Count != perMap {
					t.Errorf("round %d: map %s has %d links, want %d", round, l.Map, l.Count, perMap)
				}
			}

			var logged struct {
				Count int `db:"count"`
			}
			if err := app.DB().NewQuery(`SELECT COUNT(*) AS count FROM assignments_log WHERE action = 'assigned'`).One(&logged); err != nil {
				t.Fatal(err)
			}
			if logged.Count != len(codes) {
				t.Errorf("round %d: %d assignments_log rows, want %d", round, logged.Count, len(codes))
			}
		})
		app.Cleanup()
	}
}

// A conductor mints links for a group of eight, one after another, standing at
// block A: the common way links are created in production. A and B are within
// 50 m, C is 100 m away and D is 400 m away. The group is spread over the three
// maps within reach, each new link seeing the ones before it, and D is left alone.
func TestQuicklinkGroupSessionThroughEndpoint(t *testing.T) {
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

	serveQuicklinks(t, app, func(post func(string) *httptest.ResponseRecorder) {
		var handedOut []string
		for i := range 8 {
			rec := post(fmt.Sprintf("Publisher %d", i+1))
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
		if !slices.Equal(handedOut, want) {
			t.Errorf("maps handed out:\n got  %v\n want %v", handedOut, want)
		}
	})
}
