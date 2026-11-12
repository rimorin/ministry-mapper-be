//go:build testdata

package setup

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"ganigeorgiev.com/pocketbase/dbx"
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"ganigeorgiev.com/pocketbase/pocketbase/tests"
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
	betaAdminToken, err := generateToken("admin@beta.test")
	if err != nil {
		t.Fatal(err)
	}

	quicklinkBody := `{"territory":"testterralpha01","coordinates":{"lat":1.3521,"lng":103.8198},"publisher":"Test Publisher"}`
	quicklinkHeaders := map[string]string{
		"Content-Type":  "application/json",
		"Authorization": conductorToken,
	}
	// The four maps of testterralpha01.
	const mapA, mapB, mapC, mapD = "testmapalpha01a", "testmapalpha01b", "testmapalphsc01", "testmapalphcf01"
	atUser := map[string]float64{"lat": 1.3521, "lng": 103.8198}
	near30m := map[string]float64{"lat": 1.35237, "lng": 103.8198}
	away100m := map[string]float64{"lat": 1.3530, "lng": 103.8198}
	remaining := func(notDone, notHome int) map[string]any {
		return map[string]any{"notDone": notDone, "notHome": notHome, "done": 5, "dnc": 0, "invalid": 0, "completed": 5, "total": notDone + notHome + 5}
	}
	setMap := func(t testing.TB, app *tests.TestApp, id string, fields map[string]any) {
		rec, err := app.FindRecordById("maps", id)
		if err != nil {
			t.Fatal(err)
		}
		for k, v := range fields {
			rec.Set(k, v)
		}
		if err := app.SaveNoValidate(rec); err != nil {
			t.Fatal(err)
		}
	}
	assign := func(t testing.TB, app *tests.TestApp, mapID string) {
		col, err := app.FindCollectionByNameOrId("assignments")
		if err != nil {
			t.Fatal(err)
		}
		rec := core.NewRecord(col)
		rec.Set("map", mapID)
		rec.Set("congregation", "testcongalpha01")
		rec.Set("user", "testuseralpha02")
		rec.Set("type", "normal")
		rec.Set("publisher", "Busy Publisher")
		rec.Set("expiry_date", time.Now().UTC().Add(24*time.Hour).Format("2006-01-02 15:04:05.000Z"))
		if err := app.SaveNoValidate(rec); err != nil {
			t.Fatal(err)
		}
	}
	// Saving through the app fires the hook that drops the cached settings, which
	// otherwise outlive each scenario's fresh database.
	setThreshold := func(t testing.TB, app *tests.TestApp, threshold int) {
		cong, err := app.FindRecordById("congregations", "testcongalpha01")
		if err != nil {
			t.Fatal(err)
		}
		cong.Set("completion_threshold", threshold)
		if err := app.Save(cong); err != nil {
			t.Fatal(err)
		}
	}
	// A is idle, where the caller stands and at progressA with addresses left; B is
	// further away, has a publisher and plenty left. C and D are done.
	nearlyDoneVersusBusy := func(t testing.TB, app *tests.TestApp, progressA int) {
		setMap(t, app, mapA, map[string]any{"coordinates": atUser, "progress": progressA, "aggregates": remaining(4, 2)})
		setMap(t, app, mapB, map[string]any{"coordinates": away100m, "progress": 40, "aggregates": remaining(30, 0)})
		setMap(t, app, mapC, map[string]any{"coordinates": atUser, "progress": 100, "aggregates": remaining(0, 0)})
		setMap(t, app, mapD, map[string]any{"coordinates": atUser, "progress": 100, "aggregates": remaining(0, 0)})
		assign(t, app, mapB)
	}
	hoursAgo := func(h float64) string {
		return time.Now().UTC().Add(-time.Duration(h * float64(time.Hour))).Format("2006-01-02 15:04:05.000Z")
	}
	// Backdates every link on the map, as if minted h hours ago.
	ageLinks := func(t testing.TB, app *tests.TestApp, mapID string, h float64) {
		_, err := app.DB().NewQuery("UPDATE assignments SET created = {:at} WHERE map = {:map}").
			Bind(dbx.Params{"at": hoursAgo(h), "map": mapID}).Execute()
		if err != nil {
			t.Fatal(err)
		}
	}
	// Leaves the map with a single worked address h hours ago, or none when h < 0.
	setLastWork := func(t testing.TB, app *tests.TestApp, mapID string, h float64) {
		if _, err := app.DB().NewQuery("DELETE FROM addresses_log WHERE map = {:map}").Bind(dbx.Params{"map": mapID}).Execute(); err != nil {
			t.Fatal(err)
		}
		if h < 0 {
			return
		}
		_, err := app.DB().NewQuery(`
			INSERT INTO addresses_log (id, map, congregation, old_status, new_status, created, updated)
			VALUES ({:id}, {:map}, 'testcongalpha01', 'not_done', 'done', {:at}, {:at})
		`).Bind(dbx.Params{"id": "worklog" + mapID[len(mapID)-8:], "map": mapID, "at": hoursAgo(h)}).Execute()
		if err != nil {
			t.Fatal(err)
		}
	}
	setExpiryHours := func(t testing.TB, app *tests.TestApp, hours float64) {
		cong, err := app.FindRecordById("congregations", "testcongalpha01")
		if err != nil {
			t.Fatal(err)
		}
		cong.Set("expiry_hours", hours)
		if err := app.Save(cong); err != nil {
			t.Fatal(err)
		}
	}
	// A is where the caller stands and holds one link, minted linkAge hours ago,
	// with its last worked address workAge hours ago (none when negative). B is
	// idle, ~100 m away. C and D are in use. If A's link still counts the caller
	// is sent to B; if it has gone stale they get A.
	oneLinkOnNearestMap := func(t testing.TB, app *tests.TestApp, linkAge, workAge float64) {
		for _, id := range []string{mapA, mapB, mapC, mapD} {
			setLastWork(t, app, id, -1)
		}
		setMap(t, app, mapA, map[string]any{"coordinates": atUser})
		setMap(t, app, mapB, map[string]any{"coordinates": away100m})
		setMap(t, app, mapC, map[string]any{"coordinates": away100m})
		setMap(t, app, mapD, map[string]any{"coordinates": away100m})
		assign(t, app, mapA)
		ageLinks(t, app, mapA, linkAge)
		setLastWork(t, app, mapA, workAge)
		assign(t, app, mapC)
		assign(t, app, mapD)
	}
	// A is where the caller stands and holds one link from an earlier session,
	// minted linkAge hours ago under the congregation's expiryHours, with the last
	// address on A worked workAge hours ago (none when negative). B is idle, ~100 m
	// away; C and D are in use. The caller gets B while A's link still counts
	// towards its workload, and A once it no longer does.
	earlierSessionLink := func(t testing.TB, app *tests.TestApp, expiryHours, linkAge, workAge float64) {
		setExpiryHours(t, app, expiryHours)
		oneLinkOnNearestMap(t, app, linkAge, workAge)
		_, err := app.DB().NewQuery("UPDATE assignments SET expiry_date = {:at} WHERE map = {:map}").
			Bind(dbx.Params{"at": hoursAgo(linkAge - expiryHours), "map": mapA}).Execute()
		if err != nil {
			t.Fatal(err)
		}
	}
	// The link must be minted on the map the response names.
	linkedMap := func(t testing.TB, app *tests.TestApp, res *http.Response) string {
		raw, _ := io.ReadAll(res.Body)
		var out struct {
			LinkID string `json:"linkId"`
		}
		if err := json.Unmarshal(raw, &out); err != nil || out.LinkID == "" {
			t.Fatalf("no linkId in response: %v body=%s", err, raw)
		}
		assignment, err := app.FindRecordById("assignments", out.LinkID)
		if err != nil {
			t.Fatal(err)
		}
		return assignment.GetString("map")
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
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				assignment, err := app.FindFirstRecordByFilter("assignments", "user = 'testuseralpha03'")
				if err != nil {
					t.Fatalf("expected the quicklink assignment: %v", err)
				}
				logs, err := app.FindRecordsByFilter("assignments_log",
					"assignment = {:id}", "", 0, 0, dbx.Params{"id": assignment.Id})
				if err != nil {
					t.Fatal(err)
				}
				if len(logs) != 1 {
					t.Fatalf("expected 1 assignments_log entry for the quicklink, got %d", len(logs))
				}
				if got := logs[0].GetString("action"); got != "assigned" {
					t.Errorf("action: want assigned, got %q", got)
				}
				if got := logs[0].GetString("changed_by"); got != "testuseralpha03" {
					t.Errorf("changed_by: want testuseralpha03, got %q", got)
				}
				if got := logs[0].GetString("publisher"); got != "Test Publisher" {
					t.Errorf("publisher: want Test Publisher, got %q", got)
				}
				for _, field := range []string{"user_email", "changed_by_email"} {
					if got := logs[0].GetString(field); got != "readonly@alpha.test" {
						t.Errorf("%s: want readonly@alpha.test, got %q", field, got)
					}
				}
			},
		},
		{
			// A minted link-id is a bearer credential for the map: it authorizes
			// /link/map and /address/update with no JWT at all. A user with no role
			// in the territory's congregation must not be able to obtain one.
			Name:   "readonly user cannot quicklink another congregation's territory (403)",
			Method: http.MethodPost,
			URL:    "/territory/link",
			Body:   strings.NewReader(`{"territory":"testterrbeta001","coordinates":{"lat":1.3521,"lng":103.8198},"publisher":"Intruder"}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": readonlyToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Unauthorized."`},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				assignments, err := app.FindRecordsByFilter("assignments",
					"user = 'testuseralpha03'", "", 0, 0, nil)
				if err != nil {
					t.Fatal(err)
				}
				if len(assignments) != 0 {
					t.Errorf("expected no assignment to be minted, got %d", len(assignments))
				}
			},
		},
		{
			Name:   "conductor cannot quicklink another congregation's territory (403)",
			Method: http.MethodPost,
			URL:    "/territory/link",
			Body:   strings.NewReader(`{"territory":"testterrbeta001","coordinates":{"lat":1.3521,"lng":103.8198},"publisher":"Intruder"}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Unauthorized."`},
		},
		{
			Name:   "beta admin cannot quicklink an alpha territory (403)",
			Method: http.MethodPost,
			URL:    "/territory/link",
			Body:   strings.NewReader(`{"territory":"testterralpha01","coordinates":{"lat":1.3521,"lng":103.8198},"publisher":"Intruder"}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": betaAdminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Unauthorized."`},
		},
		{
			Name:   "beta admin can quicklink their own territory",
			Method: http.MethodPost,
			URL:    "/territory/link",
			Body:   strings.NewReader(`{"territory":"testterrbeta001","coordinates":{"lat":1.3521,"lng":103.8198},"publisher":"Beta Publisher"}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": betaAdminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"linkId"`},
		},
		{
			Name:   "unknown territory returns 404",
			Method: http.MethodPost,
			URL:    "/territory/link",
			Body:   strings.NewReader(`{"territory":"doesnotexist001","coordinates":{"lat":1.3521,"lng":103.8198},"publisher":"Test Publisher"}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  404,
			ExpectedContent: []string{`"Territory not found."`},
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
			Name:            "whitespace-only publisher returns 400",
			Method:          http.MethodPost,
			URL:             "/territory/link",
			Body:            strings.NewReader(`{"territory":"testterralpha01","coordinates":{"lat":1.3521,"lng":103.8198},"publisher":"   "}`),
			Headers:         quicklinkHeaders,
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  400,
			ExpectedContent: []string{`"Publisher is required."`},
		},
		{
			Name:            "publisher name is stored without surrounding spaces",
			Method:          http.MethodPost,
			URL:             "/territory/link",
			Body:            strings.NewReader(`{"territory":"testterralpha01","coordinates":{"lat":1.3521,"lng":103.8198},"publisher":"  Alice  "}`),
			Headers:         quicklinkHeaders,
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"linkId"`},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				if _, err := app.FindFirstRecordByFilter("assignments", "publisher = 'Alice'"); err != nil {
					t.Errorf("expected an assignment for publisher 'Alice': %v", err)
				}
			},
		},
		{
			Name:            "latitude above 90 returns 400",
			Method:          http.MethodPost,
			URL:             "/territory/link",
			Body:            strings.NewReader(`{"territory":"testterralpha01","coordinates":{"lat":90.01,"lng":103.8198},"publisher":"Test Publisher"}`),
			Headers:         quicklinkHeaders,
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  400,
			ExpectedContent: []string{`"Invalid latitude value."`},
		},
		{
			Name:            "longitude below -180 returns 400",
			Method:          http.MethodPost,
			URL:             "/territory/link",
			Body:            strings.NewReader(`{"territory":"testterralpha01","coordinates":{"lat":1.3521,"lng":-180.01},"publisher":"Test Publisher"}`),
			Headers:         quicklinkHeaders,
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  400,
			ExpectedContent: []string{`"Invalid longitude value."`},
		},
		{
			Name:            "coordinates on the edge of the valid range are accepted",
			Method:          http.MethodPost,
			URL:             "/territory/link",
			Body:            strings.NewReader(`{"territory":"testterralpha01","coordinates":{"lat":-90,"lng":180},"publisher":"Test Publisher"}`),
			Headers:         quicklinkHeaders,
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"linkId"`},
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
			// testterralpha02 has testmapalphrich1 which has real coordinates, progress=100
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

		{
			// A is finished, idle and right where the caller stands; B still has work
			// but is further away and already has a publisher. C and D are finished too.
			Name:    "map selection: finished map is not offered while another has work",
			Method:  http.MethodPost,
			URL:     "/territory/link",
			Body:    strings.NewReader(quicklinkBody),
			Headers: quicklinkHeaders,
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				setMap(t, app, mapA, map[string]any{"coordinates": atUser, "progress": 100, "aggregates": remaining(0, 0)})
				setMap(t, app, mapB, map[string]any{"coordinates": away100m, "progress": 40, "aggregates": remaining(2, 1)})
				setMap(t, app, mapC, map[string]any{"coordinates": atUser, "progress": 100, "aggregates": remaining(0, 0)})
				setMap(t, app, mapD, map[string]any{"coordinates": atUser, "progress": 100, "aggregates": remaining(0, 0)})
				assign(t, app, mapB)
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"mapName":"Blk 100B"`, `"not_done":2`, `"not_home":1`, `"assignees":["Busy Publisher"]`},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				if got := linkedMap(t, app, res); got != mapB {
					t.Errorf("link minted on %s, want %s", got, mapB)
				}
			},
		},
		{
			// Only revisits are left on B; that is still work.
			Name:    "map selection: map with only not-home addresses left still counts as unfinished",
			Method:  http.MethodPost,
			URL:     "/territory/link",
			Body:    strings.NewReader(quicklinkBody),
			Headers: quicklinkHeaders,
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				setMap(t, app, mapA, map[string]any{"coordinates": atUser, "progress": 100, "aggregates": remaining(0, 0)})
				setMap(t, app, mapB, map[string]any{"coordinates": away100m, "progress": 80, "aggregates": remaining(0, 1)})
				setMap(t, app, mapC, map[string]any{"coordinates": atUser, "progress": 100, "aggregates": remaining(0, 0)})
				setMap(t, app, mapD, map[string]any{"coordinates": atUser, "progress": 100, "aggregates": remaining(0, 0)})
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"mapName":"Blk 100B"`},
		},
		{
			// Nothing left anywhere: the caller still gets a link, to the closest idle map.
			Name:    "map selection: fully finished territory still returns a map",
			Method:  http.MethodPost,
			URL:     "/territory/link",
			Body:    strings.NewReader(quicklinkBody),
			Headers: quicklinkHeaders,
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				setMap(t, app, mapA, map[string]any{"coordinates": away100m, "progress": 100, "aggregates": remaining(0, 0)})
				setMap(t, app, mapB, map[string]any{"coordinates": atUser, "progress": 100, "aggregates": remaining(0, 0)})
				setMap(t, app, mapC, map[string]any{"coordinates": atUser, "progress": 100, "aggregates": remaining(0, 0)})
				setMap(t, app, mapD, map[string]any{"coordinates": away100m, "progress": 100, "aggregates": remaining(0, 0)})
				assign(t, app, mapC)
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"mapName":"Blk 100B"`},
		},
		{
			// A (10 left, untouched) is where the caller stands; B (100 left, half done)
			// is ~30 m away, inside the band. The larger amount of work wins.
			Name:    "map selection: more remaining work wins within 50 m band",
			Method:  http.MethodPost,
			URL:     "/territory/link",
			Body:    strings.NewReader(quicklinkBody),
			Headers: quicklinkHeaders,
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				setMap(t, app, mapA, map[string]any{"coordinates": atUser, "progress": 0, "aggregates": remaining(10, 0)})
				setMap(t, app, mapB, map[string]any{"coordinates": near30m, "progress": 50, "aggregates": remaining(90, 10)})
				setMap(t, app, mapC, map[string]any{"coordinates": away100m, "progress": 0, "aggregates": remaining(500, 0)})
				setMap(t, app, mapD, map[string]any{"coordinates": away100m, "progress": 0, "aggregates": remaining(500, 0)})
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"mapName":"Blk 100B"`},
		},
		{
			// No map in the territory has a pin. The request must still succeed.
			Name:    "map selection: territory without any coordinates still returns a map",
			Method:  http.MethodPost,
			URL:     "/territory/link",
			Body:    strings.NewReader(quicklinkBody),
			Headers: quicklinkHeaders,
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				setMap(t, app, mapA, map[string]any{"coordinates": nil, "aggregates": remaining(3, 0)})
				setMap(t, app, mapB, map[string]any{"coordinates": nil, "aggregates": remaining(9, 0)})
				setMap(t, app, mapC, map[string]any{"coordinates": nil, "aggregates": remaining(3, 0)})
				setMap(t, app, mapD, map[string]any{"coordinates": nil, "aggregates": remaining(3, 0)})
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"mapName":"Blk 100B"`},
		},
		{
			// B is the only idle map and has no pin; the located maps each have a
			// publisher. Workload decides, so B is handed out.
			Name:    "map selection: idle map without coordinates beats located busy maps",
			Method:  http.MethodPost,
			URL:     "/territory/link",
			Body:    strings.NewReader(quicklinkBody),
			Headers: quicklinkHeaders,
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				setMap(t, app, mapB, map[string]any{"coordinates": nil})
				for _, id := range []string{mapA, mapC, mapD} {
					setMap(t, app, id, map[string]any{"coordinates": atUser})
					assign(t, app, id)
				}
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"mapName":"Blk 100B"`},
		},
		{
			// With equal workload a map without a pin ranks behind any located one,
			// however far that one is.
			Name:    "map selection: located map beats map without coordinates on equal workload",
			Method:  http.MethodPost,
			URL:     "/territory/link",
			Body:    strings.NewReader(quicklinkBody),
			Headers: quicklinkHeaders,
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				setMap(t, app, mapA, map[string]any{"coordinates": nil, "progress": 0})
				setMap(t, app, mapB, map[string]any{"coordinates": map[string]float64{"lat": 1.40, "lng": 103.9}, "progress": 80})
				setMap(t, app, mapC, map[string]any{"coordinates": nil, "progress": 0})
				setMap(t, app, mapD, map[string]any{"coordinates": nil, "progress": 0})
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"mapName":"Blk 100B"`},
		},
		{
			// Three callers in a row from the same spot are spread over three maps,
			// nearest first, instead of piling onto one.
			Name:    "map selection: consecutive requests spread across maps",
			Method:  http.MethodPost,
			URL:     "/territory/link",
			Body:    strings.NewReader(quicklinkBody),
			Headers: quicklinkHeaders,
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				setMap(t, app, mapA, map[string]any{"coordinates": atUser})
				setMap(t, app, mapB, map[string]any{"coordinates": away100m})
				setMap(t, app, mapC, map[string]any{"coordinates": map[string]float64{"lat": 1.3540, "lng": 103.8198}})
				setMap(t, app, mapD, map[string]any{"coordinates": map[string]float64{"lat": 1.3550, "lng": 103.8198}})
				// Stand in for two earlier quicklinks, which would have taken A then B.
				assign(t, app, mapA)
				assign(t, app, mapB)
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"mapName":"Single Code Blk"`},
		},

		{
			Name:    "completion threshold: map just below the threshold is still offered",
			Method:  http.MethodPost,
			URL:     "/territory/link",
			Body:    strings.NewReader(quicklinkBody),
			Headers: quicklinkHeaders,
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				setThreshold(t, app, 90)
				nearlyDoneVersusBusy(t, app, 89)
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"mapName":"Blk 100A"`},
		},
		{
			Name:    "completion threshold: map at the threshold is set aside for one with work",
			Method:  http.MethodPost,
			URL:     "/territory/link",
			Body:    strings.NewReader(quicklinkBody),
			Headers: quicklinkHeaders,
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				setThreshold(t, app, 90)
				nearlyDoneVersusBusy(t, app, 90)
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"mapName":"Blk 100B"`},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				if got := linkedMap(t, app, res); got != mapB {
					t.Errorf("link minted on %s, want %s", got, mapB)
				}
			},
		},
		{
			Name:    "completion threshold: map at 97% is set aside for one with work",
			Method:  http.MethodPost,
			URL:     "/territory/link",
			Body:    strings.NewReader(quicklinkBody),
			Headers: quicklinkHeaders,
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				setThreshold(t, app, 90)
				nearlyDoneVersusBusy(t, app, 97)
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"mapName":"Blk 100B"`},
		},
		{
			// The same 97% map is offered once the congregation asks for full coverage.
			Name:    "completion threshold: raising it to 100 offers the 97% map again",
			Method:  http.MethodPost,
			URL:     "/territory/link",
			Body:    strings.NewReader(quicklinkBody),
			Headers: quicklinkHeaders,
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				setThreshold(t, app, 100)
				nearlyDoneVersusBusy(t, app, 97)
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"mapName":"Blk 100A"`},
		},
		{
			// A lower threshold sets aside more: 70% is finished at a threshold of 60.
			Name:    "completion threshold: lowering it sets aside a map that was offered before",
			Method:  http.MethodPost,
			URL:     "/territory/link",
			Body:    strings.NewReader(quicklinkBody),
			Headers: quicklinkHeaders,
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				setThreshold(t, app, 60)
				nearlyDoneVersusBusy(t, app, 70)
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"mapName":"Blk 100B"`},
		},
		{
			// Every map is past the threshold, so the leftovers are offered again:
			// idle before busy, then nearest, then most left. Runs last of this group
			// so the process-wide settings cache is left at the seeded threshold.
			Name:    "completion threshold: territory entirely past it still returns a map",
			Method:  http.MethodPost,
			URL:     "/territory/link",
			Body:    strings.NewReader(quicklinkBody),
			Headers: quicklinkHeaders,
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				setThreshold(t, app, 90)
				setMap(t, app, mapA, map[string]any{"coordinates": atUser, "progress": 99, "aggregates": remaining(1, 0)})
				setMap(t, app, mapB, map[string]any{"coordinates": near30m, "progress": 95, "aggregates": remaining(8, 2)})
				setMap(t, app, mapC, map[string]any{"coordinates": atUser, "progress": 91, "aggregates": remaining(40, 0)})
				setMap(t, app, mapD, map[string]any{"coordinates": away100m, "progress": 92, "aggregates": remaining(40, 0)})
				assign(t, app, mapC)
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"mapName":"Blk 100B"`, `"not_done":8`, `"not_home":2`},
		},

		{
			// Links last 24 h in the seed, so one stops counting after 6 h without
			// activity: half the lifetime, capped at 6 h.
			Name:    "stale links: old link on a map nobody has worked is not counted",
			Method:  http.MethodPost,
			URL:     "/territory/link",
			Body:    strings.NewReader(quicklinkBody),
			Headers: quicklinkHeaders,
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				oneLinkOnNearestMap(t, app, 13, -1)
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"mapName":"Blk 100A"`},
		},
		{
			Name:    "stale links: old link on a map last worked before the window is not counted",
			Method:  http.MethodPost,
			URL:     "/territory/link",
			Body:    strings.NewReader(quicklinkBody),
			Headers: quicklinkHeaders,
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				oneLinkOnNearestMap(t, app, 13, 13)
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"mapName":"Blk 100A"`},
		},
		{
			Name:    "stale links: old link on a map worked recently still counts",
			Method:  http.MethodPost,
			URL:     "/territory/link",
			Body:    strings.NewReader(quicklinkBody),
			Headers: quicklinkHeaders,
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				oneLinkOnNearestMap(t, app, 13, 1)
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"mapName":"Blk 100B"`},
		},
		{
			Name:    "stale links: fresh link counts before any address is worked",
			Method:  http.MethodPost,
			URL:     "/territory/link",
			Body:    strings.NewReader(quicklinkBody),
			Headers: quicklinkHeaders,
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				oneLinkOnNearestMap(t, app, 1, -1)
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"mapName":"Blk 100B"`},
		},
		{
			Name:    "stale links: link just inside the 6 h cap still counts",
			Method:  http.MethodPost,
			URL:     "/territory/link",
			Body:    strings.NewReader(quicklinkBody),
			Headers: quicklinkHeaders,
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				oneLinkOnNearestMap(t, app, 5, -1)
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"mapName":"Blk 100B"`},
		},
		{
			// Seven hours is inside half of the 24 h lifetime but past the cap.
			Name:    "stale links: link just past the 6 h cap is not counted",
			Method:  http.MethodPost,
			URL:     "/territory/link",
			Body:    strings.NewReader(quicklinkBody),
			Headers: quicklinkHeaders,
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				oneLinkOnNearestMap(t, app, 7, -1)
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"mapName":"Blk 100A"`},
		},
		{
			// With links lasting 6 h the window is 3 h, so 4 h without activity is stale.
			Name:    "stale links: window is half the congregation's expiry hours",
			Method:  http.MethodPost,
			URL:     "/territory/link",
			Body:    strings.NewReader(quicklinkBody),
			Headers: quicklinkHeaders,
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				setExpiryHours(t, app, 6)
				oneLinkOnNearestMap(t, app, 4, 4)
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"mapName":"Blk 100A"`},
		},
		{
			Name:    "stale links: same link counts within the shorter window",
			Method:  http.MethodPost,
			URL:     "/territory/link",
			Body:    strings.NewReader(quicklinkBody),
			Headers: quicklinkHeaders,
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				setExpiryHours(t, app, 6)
				oneLinkOnNearestMap(t, app, 2, -1)
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"mapName":"Blk 100B"`},
		},
		{
			// Runs last of this group so the process-wide settings cache is left at the seeded 24 h.
			Name:    "stale links: same age counts again under the 24 h default",
			Method:  http.MethodPost,
			URL:     "/territory/link",
			Body:    strings.NewReader(quicklinkBody),
			Headers: quicklinkHeaders,
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				setExpiryHours(t, app, 24)
				oneLinkOnNearestMap(t, app, 4, 4)
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"mapName":"Blk 100B"`},
		},
		{
			// A, C and D are where the caller stands and all in use; B is idle but 2 km away.
			Name:    "detour limit: caller shares the nearby map instead of walking 2 km to an idle one",
			Method:  http.MethodPost,
			URL:     "/territory/link",
			Body:    strings.NewReader(quicklinkBody),
			Headers: quicklinkHeaders,
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				setMap(t, app, mapA, map[string]any{"coordinates": atUser})
				setMap(t, app, mapB, map[string]any{"coordinates": map[string]float64{"lat": 1.370086, "lng": 103.8198}})
				setMap(t, app, mapC, map[string]any{"coordinates": atUser})
				setMap(t, app, mapD, map[string]any{"coordinates": atUser})
				assign(t, app, mapA)
				for _, id := range []string{mapC, mapC, mapD, mapD} {
					assign(t, app, id)
				}
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"mapName":"Blk 100A"`},
		},
		{
			Name:    "detour limit: idle map 250 m away is still preferred over sharing",
			Method:  http.MethodPost,
			URL:     "/territory/link",
			Body:    strings.NewReader(quicklinkBody),
			Headers: quicklinkHeaders,
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				setMap(t, app, mapA, map[string]any{"coordinates": atUser})
				setMap(t, app, mapB, map[string]any{"coordinates": map[string]float64{"lat": 1.354348, "lng": 103.8198}})
				setMap(t, app, mapC, map[string]any{"coordinates": atUser})
				setMap(t, app, mapD, map[string]any{"coordinates": atUser})
				for _, id := range []string{mapA, mapC, mapD} {
					assign(t, app, id)
				}
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"mapName":"Blk 100B"`},
		},

		// --- Sessions through the day ---
		//
		// Production links are minted in two waves, mid-morning and evening, about
		// ten hours apart, and the map is typically last worked under two hours
		// after its link was minted. Congregations run 24, 12, 6, 4 and 3 hour links.

		{
			// Half of 24 h would keep the morning link counting; the 6 h cap does not.
			Name:    "sessions: 24 h links, evening group gets the map the morning publisher left",
			Method:  http.MethodPost,
			URL:     "/territory/link",
			Body:    strings.NewReader(quicklinkBody),
			Headers: quicklinkHeaders,
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				earlierSessionLink(t, app, 24, 10, 8.2)
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"mapName":"Blk 100A"`},
		},
		{
			Name:    "sessions: 24 h links, morning publisher back at work in the evening counts again",
			Method:  http.MethodPost,
			URL:     "/territory/link",
			Body:    strings.NewReader(quicklinkBody),
			Headers: quicklinkHeaders,
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				earlierSessionLink(t, app, 24, 10, 0.2)
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"mapName":"Blk 100B"`},
		},
		{
			// Activity renews the window: last worked five hours ago, link ten hours old.
			Name:    "sessions: 24 h links, map worked through the day keeps its link counting",
			Method:  http.MethodPost,
			URL:     "/territory/link",
			Body:    strings.NewReader(quicklinkBody),
			Headers: quicklinkHeaders,
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				earlierSessionLink(t, app, 24, 10, 5)
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"mapName":"Blk 100B"`},
		},
		{
			// The link is still valid for two more hours but its 6 h window has passed.
			Name:    "sessions: 12 h links, evening group gets the map the morning publisher left",
			Method:  http.MethodPost,
			URL:     "/territory/link",
			Body:    strings.NewReader(quicklinkBody),
			Headers: quicklinkHeaders,
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				earlierSessionLink(t, app, 12, 10, 8.2)
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"mapName":"Blk 100A"`},
		},
		{
			Name:    "sessions: 12 h links, morning link counts again when the map is still being worked",
			Method:  http.MethodPost,
			URL:     "/territory/link",
			Body:    strings.NewReader(quicklinkBody),
			Headers: quicklinkHeaders,
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				earlierSessionLink(t, app, 12, 10, 0.5)
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"mapName":"Blk 100B"`},
		},
		{
			Name:    "sessions: 6 h links, morning link has expired by the evening",
			Method:  http.MethodPost,
			URL:     "/territory/link",
			Body:    strings.NewReader(quicklinkBody),
			Headers: quicklinkHeaders,
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				earlierSessionLink(t, app, 6, 10, 8.2)
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"mapName":"Blk 100A"`},
		},
		{
			Name:    "sessions: 3 h links, link from two hours ago with no work since is stale",
			Method:  http.MethodPost,
			URL:     "/territory/link",
			Body:    strings.NewReader(quicklinkBody),
			Headers: quicklinkHeaders,
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				earlierSessionLink(t, app, 3, 2, 1.8)
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"mapName":"Blk 100A"`},
		},
		{
			Name:    "sessions: 3 h links, link from an hour ago still counts",
			Method:  http.MethodPost,
			URL:     "/territory/link",
			Body:    strings.NewReader(quicklinkBody),
			Headers: quicklinkHeaders,
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				earlierSessionLink(t, app, 3, 1, -1)
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"mapName":"Blk 100B"`},
		},
		{
			// The known cost of a 6 h window: the publisher left hours ago.
			Name:    "sessions: 24 h links, afternoon group is still steered away five hours on",
			Method:  http.MethodPost,
			URL:     "/territory/link",
			Body:    strings.NewReader(quicklinkBody),
			Headers: quicklinkHeaders,
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				earlierSessionLink(t, app, 24, 5, 3.2)
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"mapName":"Blk 100B"`},
		},
		{
			// Runs last of this group so the process-wide settings cache is left at the seeded 24 h.
			Name:    "sessions: 24 h links, yesterday's link is stale the next morning",
			Method:  http.MethodPost,
			URL:     "/territory/link",
			Body:    strings.NewReader(quicklinkBody),
			Headers: quicklinkHeaders,
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				earlierSessionLink(t, app, 24, 23, 21)
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"mapName":"Blk 100A"`},
		},

		// --- Shapes found in production data ---

		{
			Name:    "production data: personal link does not make a map look busy",
			Method:  http.MethodPost,
			URL:     "/territory/link",
			Body:    strings.NewReader(quicklinkBody),
			Headers: quicklinkHeaders,
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				setMap(t, app, mapA, map[string]any{"coordinates": atUser})
				setMap(t, app, mapB, map[string]any{"coordinates": away100m})
				setMap(t, app, mapC, map[string]any{"coordinates": away100m})
				setMap(t, app, mapD, map[string]any{"coordinates": away100m})
				assign(t, app, mapA)
				if _, err := app.DB().NewQuery("UPDATE assignments SET type = 'personal' WHERE map = {:map}").Bind(dbx.Params{"map": mapA}).Execute(); err != nil {
					t.Fatal(err)
				}
				assign(t, app, mapC)
				assign(t, app, mapD)
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"mapName":"Blk 100A"`},
		},
		{
			// Older maps store notDone and notHome as 0 beside a correct completed/total.
			Name:    "production data: map with stale per-status counts but work left is offered",
			Method:  http.MethodPost,
			URL:     "/territory/link",
			Body:    strings.NewReader(quicklinkBody),
			Headers: quicklinkHeaders,
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				setMap(t, app, mapA, map[string]any{"coordinates": atUser, "progress": 0, "aggregates": map[string]any{"notDone": 0, "notHome": 0, "dnc": 0, "invalid": 0, "done": 0, "completed": 0, "total": 360}})
				setMap(t, app, mapB, map[string]any{"coordinates": away100m, "progress": 40, "aggregates": remaining(30, 0)})
				setMap(t, app, mapC, map[string]any{"coordinates": away100m, "progress": 100, "aggregates": remaining(0, 0)})
				setMap(t, app, mapD, map[string]any{"coordinates": away100m, "progress": 100, "aggregates": remaining(0, 0)})
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"mapName":"Blk 100A"`},
		},
		{
			Name:    "production data: map storing only completed and total is ranked by what is left",
			Method:  http.MethodPost,
			URL:     "/territory/link",
			Body:    strings.NewReader(quicklinkBody),
			Headers: quicklinkHeaders,
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				setMap(t, app, mapA, map[string]any{"coordinates": atUser, "progress": 10, "aggregates": remaining(9, 0)})
				setMap(t, app, mapB, map[string]any{"coordinates": near30m, "progress": 30, "aggregates": map[string]any{"completed": 12, "total": 40}})
				setMap(t, app, mapC, map[string]any{"coordinates": away100m, "progress": 0, "aggregates": remaining(500, 0)})
				setMap(t, app, mapD, map[string]any{"coordinates": away100m, "progress": 0, "aggregates": remaining(500, 0)})
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"mapName":"Blk 100B"`},
		},
		{
			Name:    "production data: newly created map without aggregates is offered",
			Method:  http.MethodPost,
			URL:     "/territory/link",
			Body:    strings.NewReader(quicklinkBody),
			Headers: quicklinkHeaders,
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				setMap(t, app, mapA, map[string]any{"coordinates": atUser, "progress": 0, "aggregates": nil})
				setMap(t, app, mapB, map[string]any{"coordinates": away100m, "progress": 100, "aggregates": remaining(0, 0)})
				setMap(t, app, mapC, map[string]any{"coordinates": away100m, "progress": 95, "aggregates": remaining(5, 0)})
				setMap(t, app, mapD, map[string]any{"coordinates": away100m, "progress": 100, "aggregates": remaining(0, 0)})
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"mapName":"Blk 100A"`},
		},
		{
			Name:    "production data: territory past the threshold offers a map with addresses left, not a done one",
			Method:  http.MethodPost,
			URL:     "/territory/link",
			Body:    strings.NewReader(quicklinkBody),
			Headers: quicklinkHeaders,
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				setMap(t, app, mapA, map[string]any{"coordinates": atUser, "progress": 100, "aggregates": remaining(0, 0)})
				setMap(t, app, mapB, map[string]any{"coordinates": away100m, "progress": 96, "aggregates": remaining(3, 1)})
				setMap(t, app, mapC, map[string]any{"coordinates": atUser, "progress": 100, "aggregates": remaining(0, 0)})
				setMap(t, app, mapD, map[string]any{"coordinates": atUser, "progress": 100, "aggregates": remaining(0, 0)})
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"mapName":"Blk 100B"`},
		},
		{
			// Identical pin, size and progress: the map first in the territory's order wins.
			Name:    "production data: maps of one block sharing a pin are handed out in territory order",
			Method:  http.MethodPost,
			URL:     "/territory/link",
			Body:    strings.NewReader(quicklinkBody),
			Headers: quicklinkHeaders,
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				for i, id := range []string{mapD, mapC, mapB, mapA} {
					setMap(t, app, id, map[string]any{"coordinates": atUser, "progress": 0, "aggregates": remaining(96, 0), "sequence": i + 1})
				}
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"mapName":"Multi Floor Blk"`},
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
