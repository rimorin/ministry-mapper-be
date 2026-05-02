package setup

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"ministry-mapper/internal/jobs"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

func TestDomainHook_LogAddressStatusChange(t *testing.T) {
	adminToken, err := generateToken("admin@alpha.test")
	if err != nil {
		t.Fatal(err)
	}

	scenarios := []tests.ApiScenario{
		{
			// testalpha01a001 is not_done; patching to not_home creates a log entry
			Name:   "status change creates addresses_log entry",
			Method: http.MethodPatch,
			URL:    "/api/collections/addresses/records/testalpha01a001",
			Body:   strings.NewReader(`{"status":"not_home","updated_by":"Admin","notes":""}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"testalpha01a001"`},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				logs, err := app.FindRecordsByFilter(
					"addresses_log",
					"address = 'testalpha01a001'",
					"", 0, 0,
				)
				if err != nil {
					t.Fatalf("failed to query addresses_log: %v", err)
				}
				if len(logs) == 0 {
					t.Error("expected at least one addresses_log entry, found none")
				}
				if len(logs) > 0 {
					if logs[0].GetString("old_status") != "not_done" {
						t.Errorf("expected old_status 'not_done', got %q", logs[0].GetString("old_status"))
					}
					if logs[0].GetString("new_status") != "not_home" {
						t.Errorf("expected new_status 'not_home', got %q", logs[0].GetString("new_status"))
					}
				}
			},
		},
		{
			// Patching with same status must not create a log entry
			Name:   "no status change does not create addresses_log entry",
			Method: http.MethodPatch,
			URL:    "/api/collections/addresses/records/testalpha01a001",
			Body:   strings.NewReader(`{"status":"not_done","updated_by":"Admin","notes":""}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"testalpha01a001"`},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				logs, err := app.FindRecordsByFilter(
					"addresses_log",
					"address = 'testalpha01a001'",
					"", 0, 0,
				)
				if err != nil {
					t.Fatalf("failed to query addresses_log: %v", err)
				}
				if len(logs) != 0 {
					t.Errorf("expected no addresses_log entries for same-status patch, found %d", len(logs))
				}
			},
		},
		{
			// testalpha01a003 is not_home with not_home_tries=0; incrementing tries
			// must create a log entry so the batch aggregate job picks up the change.
			Name:   "not_home_tries increment creates addresses_log entry",
			Method: http.MethodPatch,
			URL:    "/api/collections/addresses/records/testalpha01a003",
			Body:   strings.NewReader(`{"status":"not_home","not_home_tries":1,"updated_by":"Admin","notes":""}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"testalpha01a003"`},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				logs, err := app.FindRecordsByFilter(
					"addresses_log",
					"address = 'testalpha01a003'",
					"", 0, 0,
				)
				if err != nil {
					t.Fatalf("failed to query addresses_log: %v", err)
				}
				if len(logs) == 0 {
					t.Error("expected addresses_log entry when not_home_tries increments, found none")
				}
			},
		},
		{
			// Patching a not_home address with the same tries value must not create a log entry.
			Name:   "same not_home_tries does not create addresses_log entry",
			Method: http.MethodPatch,
			URL:    "/api/collections/addresses/records/testalpha01a003",
			Body:   strings.NewReader(`{"status":"not_home","not_home_tries":0,"updated_by":"Admin","notes":""}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"testalpha01a003"`},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				logs, err := app.FindRecordsByFilter(
					"addresses_log",
					"address = 'testalpha01a003'",
					"", 0, 0,
				)
				if err != nil {
					t.Fatalf("failed to query addresses_log: %v", err)
				}
				if len(logs) != 0 {
					t.Errorf("expected no addresses_log entries for same not_home_tries patch, found %d", len(logs))
				}
			},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestDomainHook_HandleRoleDelete_LastRole(t *testing.T) {
	adminToken, err := generateToken("admin@alpha.test")
	if err != nil {
		t.Fatal(err)
	}

	scenarios := []tests.ApiScenario{
		{
			// testrolexcng01c is the only role for testuseralpha03 (readonly@alpha.test)
			// after deleting it, unprovisioned_since must be set
			Name:   "deleting last role stamps unprovisioned_since on user",
			Method: http.MethodDelete,
			URL:    "/api/collections/roles/records/testrolexcng01c",
			Headers: map[string]string{
				"Authorization": adminToken,
			},
			TestAppFactory: setupTestApp,
			ExpectedStatus: 204,
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				user, err := app.FindRecordById("users", "testuseralpha03")
				if err != nil {
					t.Fatalf("failed to fetch user: %v", err)
				}
				if user.Get("unprovisioned_since") == nil {
					t.Error("expected unprovisioned_since to be set after last role deletion, but it is nil")
				}
			},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

// TestDomainHook_AggregateFullChain verifies the full path from a not_home_tries
// increment through to the map progress field being recomputed by the batch job.
//
// Seed state for testmapalpha01a (only countable addresses):
//   testalpha01a003: not_home, tries=0, max_tries=3
//   testalpha01a004: not_home, tries=0, max_tries=3
//
// The custom factory pre-sets testalpha01a004 to tries=3 (already at max).
// The test PATCHes testalpha01a003 to tries=3, which (after the fix) writes an
// addresses_log entry. RunAggregates then picks that up and recomputes: both
// countable addresses are now notHomeMaxTries → progress = 100.
func TestDomainHook_AggregateFullChain(t *testing.T) {
	adminToken, err := generateToken("admin@alpha.test")
	if err != nil {
		t.Fatal(err)
	}

	scenarios := []tests.ApiScenario{
		{
			Name:   "not_home_tries hitting max_tries causes batch job to recompute progress to 100",
			Method: http.MethodPatch,
			URL:    "/api/collections/addresses/records/testalpha01a003",
			Body:   strings.NewReader(`{"status":"not_home","not_home_tries":3,"updated_by":"Admin","notes":""}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			ExpectedStatus:  200,
			ExpectedContent: []string{`"testalpha01a003"`},
			TestAppFactory: func(t testing.TB) *tests.TestApp {
				app := setupTestApp(t)
				// Pre-set testalpha01a004 to max_tries so it's already in the
				// numerator before the test PATCH runs.
				addr, err := app.FindRecordById("addresses", "testalpha01a004")
				if err != nil {
					t.Fatalf("failed to find testalpha01a004: %v", err)
				}
				addr.Set("not_home_tries", 3)
				if err := app.SaveNoValidate(addr); err != nil {
					t.Fatalf("failed to pre-set testalpha01a004 tries: %v", err)
				}
				return app
			},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				// 1. Confirm the log entry was written (the fix under test).
				logs, err := app.FindRecordsByFilter(
					"addresses_log",
					"address = 'testalpha01a003'",
					"", 0, 0,
				)
				if err != nil {
					t.Fatalf("failed to query addresses_log: %v", err)
				}
				if len(logs) == 0 {
					t.Fatal("expected addresses_log entry for not_home_tries increment, found none")
				}

				// 2. Run the batch aggregate job with a 60-minute lookback so it
				//    catches the log entry that was just written.
				if err := jobs.RunAggregates(app, 60); err != nil {
					t.Fatalf("aggregate job failed: %v", err)
				}

				// 3. Both countable addresses are now at max_tries → progress = 100.
				mapRecord, err := app.FindRecordById("maps", "testmapalpha01a")
				if err != nil {
					t.Fatalf("failed to find map record: %v", err)
				}
				if got := mapRecord.GetInt("progress"); got != 100 {
					t.Errorf("expected map progress 100, got %d", got)
				}

				// 4. Aggregates JSON: all buckets zero (both addresses are notHomeMaxTries,
				//    which is not stored — only notHomeLessTries is stored as "notHome").
				var aggs map[string]interface{}
				if err := json.Unmarshal([]byte(mapRecord.GetString("aggregates")), &aggs); err != nil {
					t.Fatalf("failed to parse aggregates: %v", err)
				}
				for field, want := range map[string]int{"done": 0, "notHome": 0, "notDone": 0, "dnc": 0, "invalid": 0} {
					if got := int(aggs[field].(float64)); got != want {
						t.Errorf("aggregates.%s = %d, want %d", field, got, want)
					}
				}

				// 5. Territory progress also updated by the batch job.
				territory, err := app.FindRecordById("territories", "testterralpha01")
				if err != nil {
					t.Fatalf("failed to find territory: %v", err)
				}
				if got := territory.GetInt("progress"); got != 100 {
					t.Errorf("expected territory progress 100, got %d", got)
				}
			},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

// TestDomainHook_AggregateScenarios covers the remaining aggregate calculation
// scenarios that TestDomainHook_AggregateFullChain does not exercise.
//
// Countable addresses in testmapalpha01a (both have address_options with is_countable=true):
//   testalpha01a003: not_home, tries=0, max_tries=3
//   testalpha01a004: not_home, tries=0, max_tries=3
//
// testmapalpha01b has no address_options → zero countable addresses.
func TestDomainHook_AggregateScenarios(t *testing.T) {
	adminToken, err := generateToken("admin@alpha.test")
	if err != nil {
		t.Fatal(err)
	}

	parseAggs := func(t testing.TB, record interface{ GetString(string) string }) map[string]interface{} {
		t.Helper()
		var aggs map[string]interface{}
		if err := json.Unmarshal([]byte(record.GetString("aggregates")), &aggs); err != nil {
			t.Fatalf("failed to parse aggregates: %v", err)
		}
		return aggs
	}
	aggInt := func(aggs map[string]interface{}, key string) int {
		return int(aggs[key].(float64))
	}

	scenarios := []tests.ApiScenario{
		{
			// done address contributes to numerator.
			// testalpha01a003 changes not_home → done (status change → log entry).
			// testalpha01a004 stays not_home tries=0 < max_tries=3 (denominator only).
			// total=2, numerator=1 → progress=50; aggregates.done=1, notHome=1.
			Name:   "done status contributes to numerator — progress 50",
			Method: http.MethodPatch,
			URL:    "/api/collections/addresses/records/testalpha01a003",
			Body:   strings.NewReader(`{"status":"done","updated_by":"Admin","notes":""}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"testalpha01a003"`},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				if err := jobs.RunAggregates(app, 60); err != nil {
					t.Fatalf("aggregate job failed: %v", err)
				}
				mapRecord, err := app.FindRecordById("maps", "testmapalpha01a")
				if err != nil {
					t.Fatalf("failed to find map record: %v", err)
				}
				if got := mapRecord.GetInt("progress"); got != 50 {
					t.Errorf("expected progress 50, got %d", got)
				}
				aggs := parseAggs(t, mapRecord)
				if aggInt(aggs, "done") != 1 {
					t.Errorf("aggregates.done: want 1, got %d", aggInt(aggs, "done"))
				}
				if aggInt(aggs, "notHome") != 1 {
					t.Errorf("aggregates.notHome: want 1, got %d", aggInt(aggs, "notHome"))
				}
			},
		},
		{
			// not_home with tries < max_tries stays in the denominator only.
			// Factory pre-sets testalpha01a004 to tries=3 (numerator).
			// PATCH increments testalpha01a003 to tries=1 (still < max_tries=3, denom only).
			// total=2, numerator=1 → progress=50; aggregates.notHome=1.
			Name:   "not_home below max_tries stays in denominator only — progress 50",
			Method: http.MethodPatch,
			URL:    "/api/collections/addresses/records/testalpha01a003",
			Body:   strings.NewReader(`{"status":"not_home","not_home_tries":1,"updated_by":"Admin","notes":""}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory: func(t testing.TB) *tests.TestApp {
				app := setupTestApp(t)
				addr, err := app.FindRecordById("addresses", "testalpha01a004")
				if err != nil {
					t.Fatalf("failed to find testalpha01a004: %v", err)
				}
				addr.Set("not_home_tries", 3)
				if err := app.SaveNoValidate(addr); err != nil {
					t.Fatalf("failed to pre-set testalpha01a004: %v", err)
				}
				return app
			},
			ExpectedStatus:  200,
			ExpectedContent: []string{`"testalpha01a003"`},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				if err := jobs.RunAggregates(app, 60); err != nil {
					t.Fatalf("aggregate job failed: %v", err)
				}
				mapRecord, err := app.FindRecordById("maps", "testmapalpha01a")
				if err != nil {
					t.Fatalf("failed to find map record: %v", err)
				}
				if got := mapRecord.GetInt("progress"); got != 50 {
					t.Errorf("expected progress 50, got %d", got)
				}
				aggs := parseAggs(t, mapRecord)
				if aggInt(aggs, "notHome") != 1 {
					t.Errorf("aggregates.notHome: want 1, got %d", aggInt(aggs, "notHome"))
				}
			},
		},
		{
			// not_done countable address reduces progress below 100%.
			// Factory resets testalpha01a003 back to not_done (SaveNoValidate, no hook).
			// PATCH increments testalpha01a004 to tries=3 (max_tries) → log entry.
			// total=2 (notDone=1 + notHomeMaxTries=1), numerator=1 → progress=50; aggregates.notDone=1.
			Name:   "not_done countable address reduces progress — progress 50",
			Method: http.MethodPatch,
			URL:    "/api/collections/addresses/records/testalpha01a004",
			Body:   strings.NewReader(`{"status":"not_home","not_home_tries":3,"updated_by":"Admin","notes":""}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory: func(t testing.TB) *tests.TestApp {
				app := setupTestApp(t)
				addr, err := app.FindRecordById("addresses", "testalpha01a003")
				if err != nil {
					t.Fatalf("failed to find testalpha01a003: %v", err)
				}
				addr.Set("status", "not_done")
				if err := app.SaveNoValidate(addr); err != nil {
					t.Fatalf("failed to reset testalpha01a003 to not_done: %v", err)
				}
				return app
			},
			ExpectedStatus:  200,
			ExpectedContent: []string{`"testalpha01a004"`},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				if err := jobs.RunAggregates(app, 60); err != nil {
					t.Fatalf("aggregate job failed: %v", err)
				}
				mapRecord, err := app.FindRecordById("maps", "testmapalpha01a")
				if err != nil {
					t.Fatalf("failed to find map record: %v", err)
				}
				if got := mapRecord.GetInt("progress"); got != 50 {
					t.Errorf("expected progress 50, got %d", got)
				}
				aggs := parseAggs(t, mapRecord)
				if aggInt(aggs, "notDone") != 1 {
					t.Errorf("aggregates.notDone: want 1, got %d", aggInt(aggs, "notDone"))
				}
			},
		},
		{
			// Map with zero countable addresses: no division by zero, progress stays 0.
			// testalpha01b001-005 have no address_options → none are countable.
			// Changing one to done triggers an addresses_log entry and runs the batch job,
			// but the aggregate query returns no rows → total=0, progress=0.
			Name:   "map with no countable addresses keeps progress at 0",
			Method: http.MethodPatch,
			URL:    "/api/collections/addresses/records/testalpha01b001",
			Body:   strings.NewReader(`{"status":"done","updated_by":"Admin","notes":""}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"testalpha01b001"`},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				if err := jobs.RunAggregates(app, 60); err != nil {
					t.Fatalf("aggregate job failed: %v", err)
				}
				mapRecord, err := app.FindRecordById("maps", "testmapalpha01b")
				if err != nil {
					t.Fatalf("failed to find map record: %v", err)
				}
				if got := mapRecord.GetInt("progress"); got != 0 {
					t.Errorf("expected progress 0 for map with no countable addresses, got %d", got)
				}
			},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestDomainHook_HandleRoleDelete_NotLastRole(t *testing.T) {
	t.Skip("SaveNoValidate visibility inside hook transaction not yet resolved")
	adminToken, err := generateToken("admin@alpha.test")
	if err != nil {
		t.Fatal(err)
	}

	scenarios := []tests.ApiScenario{
		{
			// Give testuseralpha03 a second role first, then delete testrolexcng01c.
			// unprovisioned_since must NOT be set because one role remains.
			Name:   "deleting non-last role does not stamp unprovisioned_since",
			Method: http.MethodDelete,
			URL:    "/api/collections/roles/records/testrolexcng01c",
			Headers: map[string]string{
				"Authorization": adminToken,
			},
			TestAppFactory: func(t testing.TB) *tests.TestApp {
				app := setupTestApp(t)
				roleCollection, err := app.FindCollectionByNameOrId("roles")
				if err != nil {
					t.Fatalf("failed to find roles collection: %v", err)
				}
				extraRole := core.NewRecord(roleCollection)
				extraRole.Set("user", "testuseralpha03")
				extraRole.Set("congregation", "testcongalpha01")
				extraRole.Set("role", "conductor")
				if err := app.SaveNoValidate(extraRole); err != nil {
					t.Fatalf("failed to save extra role: %v", err)
				}
				return app
			},
			ExpectedStatus: 204,
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				user, err := app.FindRecordById("users", "testuseralpha03")
				if err != nil {
					t.Fatalf("failed to fetch user: %v", err)
				}
				if user.Get("unprovisioned_since") != nil {
					t.Error("expected unprovisioned_since to be nil when user still has roles")
				}
			},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}
