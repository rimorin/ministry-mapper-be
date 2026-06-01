package setup

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

func TestHandleResetTerritory(t *testing.T) {
	adminToken, err := generateToken("admin@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	conductorToken, err := generateToken("conductor@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	betaAdminToken, err := generateToken("admin@beta.test")
	if err != nil {
		t.Fatal(err)
	}

	scenarios := []tests.ApiScenario{
		{
			Name:   "admin from different congregation cannot reset territory (403)",
			Method: http.MethodPost,
			URL:    "/territory/reset",
			Body:   strings.NewReader(`{"territory":"testterralpha01"}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": betaAdminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Administrator or conductor access required."`},
		},
		{
			Name:   "no auth is rejected with 401",
			Method: http.MethodPost,
			URL:    "/territory/reset",
			Body:   strings.NewReader(`{"territory":"testterralpha01"}`),
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  401,
			ExpectedContent: []string{`"status":401`},
		},
		{
			Name:   "conductor can reset territory",
			Method: http.MethodPost,
			URL:    "/territory/reset",
			Body:   strings.NewReader(`{"territory":"testterralpha01"}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"Territory reset successfully"`},
		},
		{
			Name:   "admin can reset territory and all addresses revert to not_done",
			Method: http.MethodPost,
			URL:    "/territory/reset",
			Body:   strings.NewReader(`{"territory":"testterralpha01"}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"Territory reset successfully"`},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				records, err := app.FindRecordsByFilter(
					"addresses",
					"territory = 'testterralpha01' && status != 'not_done'",
					"", 0, 0,
				)
				if err != nil {
					t.Fatalf("query failed: %v", err)
				}
				if len(records) != 0 {
					t.Errorf("expected all addresses to be not_done after reset, found %d with other status", len(records))
				}

				for _, addressID := range []string{"testalpha01a003", "testalpha01a004", "testalpha01a005"} {
					addr, err := app.FindRecordById("addresses", addressID)
					if err != nil {
						t.Fatalf("failed to fetch %s: %v", addressID, err)
					}
					if got := addr.GetString("status"); got != "not_done" {
						t.Errorf("%s status: want not_done, got %q", addressID, got)
					}
				}

				mapRecord, err := app.FindRecordById("maps", "testmapalpha01a")
				if err != nil {
					t.Fatalf("failed to fetch map: %v", err)
				}
				if got := mapRecord.GetInt("progress"); got != 0 {
					t.Errorf("expected map progress 0 after territory reset, got %d", got)
				}
				var aggs map[string]interface{}
				if err := json.Unmarshal([]byte(mapRecord.GetString("aggregates")), &aggs); err != nil {
					t.Fatalf("failed to parse map aggregates: %v", err)
				}
				for field, want := range map[string]int{"notDone": 2, "done": 0, "notHome": 0, "invalid": 0, "dnc": 0} {
					if got := int(aggs[field].(float64)); got != want {
						t.Errorf("map aggregates.%s: want %d, got %d", field, want, got)
					}
				}

				territory, err := app.FindRecordById("territories", "testterralpha01")
				if err != nil {
					t.Fatalf("failed to fetch territory: %v", err)
				}
				if got := territory.GetInt("progress"); got != 0 {
					t.Errorf("expected territory progress 0 after reset, got %d", got)
				}
			},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestHandleDeleteTerritory(t *testing.T) {
	adminToken, err := generateToken("admin@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	conductorToken, err := generateToken("conductor@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	betaAdminToken, err := generateToken("admin@beta.test")
	if err != nil {
		t.Fatal(err)
	}

	scenarios := []tests.ApiScenario{
		{
			Name:   "admin from different congregation cannot delete territory (403)",
			Method: http.MethodPost,
			URL:    "/territory/delete",
			Body:   strings.NewReader(`{"territory":"testterralpha02"}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": betaAdminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Administrator or conductor access required."`},
		},
		{
			Name:   "no auth is rejected with 401",
			Method: http.MethodPost,
			URL:    "/territory/delete",
			Body:   strings.NewReader(`{"territory":"testterralpha02"}`),
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  401,
			ExpectedContent: []string{`"status":401`},
		},
		{
			Name:   "conductor can delete territory",
			Method: http.MethodPost,
			URL:    "/territory/delete",
			Body:   strings.NewReader(`{"territory":"testterralpha02"}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"Territory deleted successfully"`},
		},
		{
			Name:   "missing territory ID returns 400",
			Method: http.MethodPost,
			URL:    "/territory/delete",
			Body:   strings.NewReader(`{}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  400,
			ExpectedContent: []string{`"Missing territory ID."`},
		},
		{
			Name:   "admin can delete territory and it no longer exists",
			Method: http.MethodPost,
			URL:    "/territory/delete",
			Body:   strings.NewReader(`{"territory":"testterralpha02"}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"Territory deleted successfully"`},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				_, err := app.FindRecordById("territories", "testterralpha02")
				if err == nil {
					t.Error("expected territory testterralpha02 to be deleted, but it still exists")
				}
			},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestHandleTerritoryQuicklink(t *testing.T) {
	conductorToken, err := generateToken("conductor@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	readonlyToken, err := generateToken("readonly@alpha.test")
	if err != nil {
		t.Fatal(err)
	}

	scenarios := []tests.ApiScenario{
		{
			Name:   "no auth is rejected with 401",
			Method: http.MethodPost,
			URL:    "/territory/link",
			Body:   strings.NewReader(`{"territory":"testterralpha01","coordinates":{"lat":1.3521,"lng":103.8198},"publisher":"Test Publisher"}`),
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  401,
			ExpectedContent: []string{`"status":401`},
		},
		{
			Name:   "readonly user can access quicklink",
			Method: http.MethodPost,
			URL:    "/territory/link",
			Body:   strings.NewReader(`{"territory":"testterralpha01","coordinates":{"lat":1.3521,"lng":103.8198},"publisher":"Test Publisher"}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": readonlyToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"linkId"`},
		},
		{
			Name:   "conductor can get quicklink with valid territory",
			Method: http.MethodPost,
			URL:    "/territory/link",
			Body:   strings.NewReader(`{"territory":"testterralpha01","coordinates":{"lat":1.3521,"lng":103.8198},"publisher":"Test Publisher"}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"linkId"`, `"mapName"`, `"progress"`},
		},
		{
			Name:   "missing territory field returns 400",
			Method: http.MethodPost,
			URL:    "/territory/link",
			Body:   strings.NewReader(`{"coordinates":{"lat":1.3521,"lng":103.8198}}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  400,
			ExpectedContent: []string{`"Territory ID is required."`},
		},
		{
			Name:   "missing coordinates returns 400",
			Method: http.MethodPost,
			URL:    "/territory/link",
			Body:   strings.NewReader(`{"territory":"testterralpha01"}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  400,
			ExpectedContent: []string{`"Coordinates are required."`},
		},
		{
			Name:   "missing lat key returns 400",
			Method: http.MethodPost,
			URL:    "/territory/link",
			Body:   strings.NewReader(`{"territory":"testterralpha01","coordinates":{"lng":103.8198}}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  400,
			ExpectedContent: []string{`"Both lat and long coordinates are required."`},
		},
		{
			Name:   "missing lng key returns 400",
			Method: http.MethodPost,
			URL:    "/territory/link",
			Body:   strings.NewReader(`{"territory":"testterralpha01","coordinates":{"lat":1.3521}}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  400,
			ExpectedContent: []string{`"Both lat and long coordinates are required."`},
		},
		{
			Name:   "non-numeric lat returns 400",
			Method: http.MethodPost,
			URL:    "/territory/link",
			Body:   strings.NewReader(`{"territory":"testterralpha01","coordinates":{"lat":"north","lng":103.8198}}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  400,
			ExpectedContent: []string{`"Invalid latitude value."`},
		},
		{
			Name:   "non-numeric lng returns 400",
			Method: http.MethodPost,
			URL:    "/territory/link",
			Body:   strings.NewReader(`{"territory":"testterralpha01","coordinates":{"lat":1.3521,"lng":"east"}}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  400,
			ExpectedContent: []string{`"Invalid longitude value."`},
		},
		{
			Name:   "missing publisher returns 400",
			Method: http.MethodPost,
			URL:    "/territory/link",
			Body:   strings.NewReader(`{"territory":"testterralpha01","coordinates":{"lat":1.3521,"lng":103.8198}}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  400,
			ExpectedContent: []string{`"Publisher is required."`},
		},
		{
			Name:   "empty publisher returns 400",
			Method: http.MethodPost,
			URL:    "/territory/link",
			Body:   strings.NewReader(`{"territory":"testterralpha01","coordinates":{"lat":1.3521,"lng":103.8198},"publisher":""}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  400,
			ExpectedContent: []string{`"Publisher is required."`},
		},
		{
			Name:   "nonexistent territory returns 404",
			Method: http.MethodPost,
			URL:    "/territory/link",
			Body:   strings.NewReader(`{"territory":"doesnotexist","coordinates":{"lat":1.3521,"lng":103.8198},"publisher":"Test Publisher"}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  404,
			ExpectedContent: []string{`"status":404`},
		},
		{
			// testterralpha02 has testmapalphrich1 which has real coordinates, progress=40
			// and aggregates. Calling with user at the same location ensures it wins,
			// and the response must contain all documented fields.
			Name:   "response contains all required fields",
			Method: http.MethodPost,
			URL:    "/territory/link",
			Body:   strings.NewReader(`{"territory":"testterralpha02","coordinates":{"lat":1.234,"lng":103.456},"publisher":"Test Publisher"}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"linkId"`, `"mapName"`, `"progress"`, `"not_done"`, `"not_home"`, `"coordinates"`, `"assignees"`},
		},
		{
			Name:   "request with publisher field succeeds",
			Method: http.MethodPost,
			URL:    "/territory/link",
			Body:   strings.NewReader(`{"territory":"testterralpha01","coordinates":{"lat":1.3521,"lng":103.8198},"publisher":"Alice"}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"linkId"`},
		},

		// --- Map selection scenarios ---

		{
			// Map A (testmapalpha01a) has 1 active normal assignment; Map B (testmapalpha01b)
			// has 0. B is ~100m further away but wins because assignment count takes priority.
			Name:   "map selection: fewer assignments beats closer map",
			Method: http.MethodPost,
			URL:    "/territory/link",
			Body:   strings.NewReader(`{"territory":"testterralpha01","coordinates":{"lat":1.3521,"lng":103.8198},"publisher":"Test Publisher"}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				// Give map A (Blk 100A) one active normal assignment.
				col, err := app.FindCollectionByNameOrId("assignments")
				if err != nil {
					t.Fatal(err)
				}
				rec := core.NewRecord(col)
				rec.Set("map", "testmapalpha01a")
				rec.Set("congregation", "testcongalpha01")
				rec.Set("user", "testuseralpha02")
				rec.Set("type", "normal")
				rec.Set("publisher", "Busy Publisher")
				rec.Set("expiry_date", time.Now().UTC().Add(24*time.Hour).Format("2006-01-02 15:04:05.000Z"))
				if err := app.SaveNoValidate(rec); err != nil {
					t.Fatal(err)
				}
				// Place A at user position (0 m), B at ~100 m north.
				a, _ := app.FindRecordById("maps", "testmapalpha01a")
				a.Set("coordinates", map[string]float64{"lat": 1.3521, "lng": 103.8198})
				app.SaveNoValidate(a)

				b, _ := app.FindRecordById("maps", "testmapalpha01b")
				b.Set("coordinates", map[string]float64{"lat": 1.3530, "lng": 103.8198})
				app.SaveNoValidate(b)
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"mapName":"Blk 100B"`},
		},
		{
			// Both maps have 0 assignments. Map A (Blk 100A) is at user position (0 m,
			// progress 80); Map B (Blk 100B) is ~1100 m away (progress 20). The gap is
			// well beyond the 50 m proximity band, so A wins on distance alone.
			Name:   "map selection: closer map wins when gap exceeds 50 m",
			Method: http.MethodPost,
			URL:    "/territory/link",
			Body:   strings.NewReader(`{"territory":"testterralpha01","coordinates":{"lat":1.3521,"lng":103.8198},"publisher":"Test Publisher"}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				a, _ := app.FindRecordById("maps", "testmapalpha01a")
				a.Set("coordinates", map[string]float64{"lat": 1.3521, "lng": 103.8198})
				a.Set("progress", 80)
				app.SaveNoValidate(a)

				b, _ := app.FindRecordById("maps", "testmapalpha01b")
				b.Set("coordinates", map[string]float64{"lat": 1.3620, "lng": 103.8198})
				b.Set("progress", 20)
				app.SaveNoValidate(b)
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"mapName":"Blk 100A"`},
		},
		{
			// Both maps have 0 assignments. Map A (Blk 100A) is at user position (0 m,
			// progress 80); Map B (Blk 100B) is ~30 m away (progress 20). Both fall within
			// the 50 m proximity band, so lower progress wins and B is selected.
			Name:   "map selection: lower progress wins when within 50 m band",
			Method: http.MethodPost,
			URL:    "/territory/link",
			Body:   strings.NewReader(`{"territory":"testterralpha01","coordinates":{"lat":1.3521,"lng":103.8198},"publisher":"Test Publisher"}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				a, _ := app.FindRecordById("maps", "testmapalpha01a")
				a.Set("coordinates", map[string]float64{"lat": 1.3521, "lng": 103.8198})
				a.Set("progress", 80)
				app.SaveNoValidate(a)

				b, _ := app.FindRecordById("maps", "testmapalpha01b")
				b.Set("coordinates", map[string]float64{"lat": 1.35237, "lng": 103.8198}) // ~30 m
				b.Set("progress", 20)
				app.SaveNoValidate(b)
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"mapName":"Blk 100B"`},
		},
		{
			// Three-map chain: |A-B| ≈ 40 m, |B-C| ≈ 45 m, |A-C| ≈ 85 m.
			// C (Single Code Blk) is closest but has highest progress.
			// B (Blk 100B) and C are both within the 50 m band; B has lower progress → B wins.
			// A (Blk 100A) is outside the band entirely.
			Name:   "map selection: three-map chain selects correct mid-range map",
			Method: http.MethodPost,
			URL:    "/territory/link",
			Body:   strings.NewReader(`{"territory":"testterralpha01","coordinates":{"lat":1.3521,"lng":103.8198},"publisher":"Test Publisher"}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				// A at ~300 m, lowest progress.
				a, _ := app.FindRecordById("maps", "testmapalpha01a")
				a.Set("coordinates", map[string]float64{"lat": 1.354803, "lng": 103.8198})
				a.Set("progress", 10)
				app.SaveNoValidate(a)

				// B at ~260 m, mid progress — expected winner.
				b, _ := app.FindRecordById("maps", "testmapalpha01b")
				b.Set("coordinates", map[string]float64{"lat": 1.354442, "lng": 103.8198})
				b.Set("progress", 30)
				app.SaveNoValidate(b)

				// C at ~215 m, highest progress.
				c, _ := app.FindRecordById("maps", "testmapalphsc01")
				c.Set("coordinates", map[string]float64{"lat": 1.354037, "lng": 103.8198})
				c.Set("progress", 50)
				app.SaveNoValidate(c)
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"mapName":"Blk 100B"`},
		},

		// --- Co-assignee scenarios ---

		{
			// A named co-assignee already on the winning map must appear in assignees.
			// All four maps are given one active normal assignment so count is tied at 1;
			// testmapalpha01a wins the distance tiebreak and its assignee "Alice" is returned.
			Name:   "assignees: active named co-assignee appears in response",
			Method: http.MethodPost,
			URL:    "/territory/link",
			Body:   strings.NewReader(`{"territory":"testterralpha01","coordinates":{"lat":1.3521,"lng":103.8198},"publisher":"Test Publisher"}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				col, err := app.FindCollectionByNameOrId("assignments")
				if err != nil {
					t.Fatal(err)
				}
				activeExpiry := time.Now().UTC().Add(24 * time.Hour).Format("2006-01-02 15:04:05.000Z")

				// Give the three other maps one active normal assignment each so all
				// maps tie on count and distance decides.
				for _, mapID := range []string{"testmapalpha01b", "testmapalphsc01", "testmapalphcf01"} {
					rec := core.NewRecord(col)
					rec.Set("map", mapID)
					rec.Set("congregation", "testcongalpha01")
					rec.Set("user", "testuseralpha02")
					rec.Set("type", "normal")
					rec.Set("publisher", "Other")
					rec.Set("expiry_date", activeExpiry)
					app.SaveNoValidate(rec)
				}

				// Alice is already working testmapalpha01a.
				alice := core.NewRecord(col)
				alice.Set("map", "testmapalpha01a")
				alice.Set("congregation", "testcongalpha01")
				alice.Set("user", "testuseralpha01")
				alice.Set("type", "normal")
				alice.Set("publisher", "Alice")
				alice.Set("expiry_date", activeExpiry)
				app.SaveNoValidate(alice)

				// Place testmapalpha01a at user position (0 m) to win the distance tiebreak.
				a, _ := app.FindRecordById("maps", "testmapalpha01a")
				a.Set("coordinates", map[string]float64{"lat": 1.3521, "lng": 103.8198})
				app.SaveNoValidate(a)
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"assignees":["Alice"]`},
		},
		{
			// An existing assignment with an empty publisher must be returned as "slip-XXXX"
			// (first 4 characters of its assignment ID) rather than an empty string.
			Name:   "assignees: anonymous co-assignee shown as slip-XXXX",
			Method: http.MethodPost,
			URL:    "/territory/link",
			Body:   strings.NewReader(`{"territory":"testterralpha01","coordinates":{"lat":1.3521,"lng":103.8198},"publisher":"Test Publisher"}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				col, err := app.FindCollectionByNameOrId("assignments")
				if err != nil {
					t.Fatal(err)
				}
				activeExpiry := time.Now().UTC().Add(24 * time.Hour).Format("2006-01-02 15:04:05.000Z")

				for _, mapID := range []string{"testmapalpha01b", "testmapalphsc01", "testmapalphcf01"} {
					rec := core.NewRecord(col)
					rec.Set("map", mapID)
					rec.Set("congregation", "testcongalpha01")
					rec.Set("user", "testuseralpha02")
					rec.Set("type", "normal")
					rec.Set("publisher", "Other")
					rec.Set("expiry_date", activeExpiry)
					app.SaveNoValidate(rec)
				}

				// Anonymous co-assignee: empty publisher → must become "slip-XXXX".
				anon := core.NewRecord(col)
				anon.Set("map", "testmapalpha01a")
				anon.Set("congregation", "testcongalpha01")
				anon.Set("user", "testuseralpha01")
				anon.Set("type", "normal")
				anon.Set("publisher", "")
				anon.Set("expiry_date", activeExpiry)
				app.SaveNoValidate(anon)

				a, _ := app.FindRecordById("maps", "testmapalpha01a")
				a.Set("coordinates", map[string]float64{"lat": 1.3521, "lng": 103.8198})
				app.SaveNoValidate(a)
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"slip-`},
		},
		{
			// An expired co-assignee must not appear in the assignees list.
			// Bob's assignment has a past expiry date; only the new quicklink assignment
			// is active on the map, but that one is excluded by design (it's the caller's).
			Name:   "assignees: expired co-assignee excluded from response",
			Method: http.MethodPost,
			URL:    "/territory/link",
			Body:   strings.NewReader(`{"territory":"testterralpha01","coordinates":{"lat":1.3521,"lng":103.8198},"publisher":"Test Publisher"}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				col, err := app.FindCollectionByNameOrId("assignments")
				if err != nil {
					t.Fatal(err)
				}

				// Bob's assignment is expired — must not appear in assignees.
				bob := core.NewRecord(col)
				bob.Set("map", "testmapalpha01a")
				bob.Set("congregation", "testcongalpha01")
				bob.Set("user", "testuseralpha01")
				bob.Set("type", "normal")
				bob.Set("publisher", "Bob")
				bob.Set("expiry_date", "2000-01-01 00:00:00.000Z")
				app.SaveNoValidate(bob)

				// Place testmapalpha01a at user position so it wins on distance
				// (all maps have 0 active normal assignments; Bob's is expired).
				a, _ := app.FindRecordById("maps", "testmapalpha01a")
				a.Set("coordinates", map[string]float64{"lat": 1.3521, "lng": 103.8198})
				app.SaveNoValidate(a)
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"assignees":[]`},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}
