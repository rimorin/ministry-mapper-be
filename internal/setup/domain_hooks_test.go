package setup

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"ministry-mapper/internal/handlers"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

func waitForMapAggregates(t testing.TB, app *tests.TestApp, mapID string, wantProgress int, wantAggs map[string]int) *core.Record {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mapRecord, err := app.FindRecordById("maps", mapID)
		if err == nil && mapRecord.GetInt("progress") == wantProgress {
			if wantAggs == nil {
				waitForAsyncAggregateSettle()
				return mapRecord
			}

			var aggs map[string]interface{}
			if err := json.Unmarshal([]byte(mapRecord.GetString("aggregates")), &aggs); err == nil {
				matched := true
				for field, want := range wantAggs {
					value, ok := aggs[field].(float64)
					if !ok || int(value) != want {
						matched = false
						break
					}
				}
				if matched {
					waitForAsyncAggregateSettle()
					return mapRecord
				}
			}
		}

		time.Sleep(10 * time.Millisecond)
	}

	mapRecord, err := app.FindRecordById("maps", mapID)
	if err != nil {
		t.Fatalf("failed to find map record %s after waiting: %v", mapID, err)
	}
	if wantAggs != nil {
		t.Fatalf("timed out waiting for map %s progress %d and aggregates %v; got progress %d aggregates %s", mapID, wantProgress, wantAggs, mapRecord.GetInt("progress"), mapRecord.GetString("aggregates"))
	}
	t.Fatalf("timed out waiting for map %s progress %d; got %d", mapID, wantProgress, mapRecord.GetInt("progress"))
	return nil
}

func waitForTerritoryProgress(t testing.TB, app *tests.TestApp, territoryID string, wantProgress int) *core.Record {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		territory, err := app.FindRecordById("territories", territoryID)
		if err == nil && territory.GetInt("progress") == wantProgress {
			waitForAsyncAggregateSettle()
			return territory
		}

		time.Sleep(10 * time.Millisecond)
	}

	territory, err := app.FindRecordById("territories", territoryID)
	if err != nil {
		t.Fatalf("failed to find territory %s after waiting: %v", territoryID, err)
	}
	t.Fatalf("timed out waiting for territory %s progress %d; got %d", territoryID, wantProgress, territory.GetInt("progress"))
	return nil
}

func waitForAsyncAggregateSettle() {
	time.Sleep(50 * time.Millisecond)
}

func TestDomainHook_LogAddressStatusChange(t *testing.T) {
	adminToken, err := generateToken("admin@alpha.test")
	if err != nil {
		t.Fatal(err)
	}

	scenarios := []tests.ApiScenario{
		{
			// testalpha01a001 is not_done; updating to not_home creates a log entry
			Name:   "status change creates addresses_log entry",
			Method: http.MethodPost,
			URL:    "/address/update",
			Body:   strings.NewReader(`{"address_id":"testalpha01a001","map_id":"testmapalpha01a","status":"not_home","updated_by":"Admin","notes":""}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory: setupTestApp,
			ExpectedStatus: 204,
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
				waitForAsyncAggregateSettle()
			},
		},
		{
			// Updating with same status must not create a log entry
			Name:   "no status change does not create addresses_log entry",
			Method: http.MethodPost,
			URL:    "/address/update",
			Body:   strings.NewReader(`{"address_id":"testalpha01a001","map_id":"testmapalpha01a","status":"not_done","updated_by":"Admin","notes":""}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory: setupTestApp,
			ExpectedStatus: 204,
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
			Method: http.MethodPost,
			URL:    "/address/update",
			Body:   strings.NewReader(`{"address_id":"testalpha01a003","map_id":"testmapalpha01a","status":"not_home","not_home_tries":1,"updated_by":"Admin","notes":""}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory: setupTestApp,
			ExpectedStatus: 204,
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
				waitForAsyncAggregateSettle()
			},
		},
		{
			// Updating a not_home address with the same tries value must not create a log entry.
			Name:   "same not_home_tries does not create addresses_log entry",
			Method: http.MethodPost,
			URL:    "/address/update",
			Body:   strings.NewReader(`{"address_id":"testalpha01a003","map_id":"testmapalpha01a","status":"not_home","not_home_tries":0,"updated_by":"Admin","notes":""}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory: setupTestApp,
			ExpectedStatus: 204,
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
		{
			// Decrementing tries (3→1) must also create a log entry: the address
			// shifts from the notHomeMaxTries bucket (numerator) to notHomeLessTries
			// (denominator only), changing the aggregate. Factory pre-sets tries=3
			// so the update to tries=1 is a genuine decrement.
			Name:   "not_home_tries decrement creates addresses_log entry",
			Method: http.MethodPost,
			URL:    "/address/update",
			Body:   strings.NewReader(`{"address_id":"testalpha01a003","map_id":"testmapalpha01a","status":"not_home","not_home_tries":1,"updated_by":"Admin","notes":""}`),
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
				addr.Set("not_home_tries", 3)
				if err := app.SaveNoValidate(addr); err != nil {
					t.Fatalf("failed to pre-set testalpha01a003 tries to 3: %v", err)
				}
				return app
			},
			ExpectedStatus: 204,
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
					t.Error("expected addresses_log entry when not_home_tries decrements, found none")
				}
				waitForAsyncAggregateSettle()
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
// increment through to the map progress field being recomputed by the async
// address update hook.
//
// Seed state for testmapalpha01a (only countable addresses):
//
//	testalpha01a003: not_home, tries=0, max_tries=3
//	testalpha01a004: not_home, tries=0, max_tries=3
//
// The custom factory pre-sets testalpha01a004 to tries=3 (already at max) and
// marks testalpha01a003 as source=app so the hook runs. The test PATCHes
// testalpha01a003 to tries=3; both countable addresses are then notHomeMaxTries
// and the hook recomputes progress to 100 immediately via FireAndForget.
func TestDomainHook_AggregateFullChain(t *testing.T) {
	adminToken, err := generateToken("admin@alpha.test")
	if err != nil {
		t.Fatal(err)
	}

	scenarios := []tests.ApiScenario{
		{
			Name:   "not_home_tries hitting max_tries recalculates progress to 100 immediately",
			Method: http.MethodPost,
			URL:    "/address/update",
			Body:   strings.NewReader(`{"address_id":"testalpha01a003","map_id":"testmapalpha01a","status":"not_home","not_home_tries":3,"updated_by":"Admin","notes":""}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			ExpectedStatus: 204,
			TestAppFactory: func(t testing.TB) *tests.TestApp {
				app := setupTestApp(t)

				addr, err := app.FindRecordById("addresses", "testalpha01a004")
				if err != nil {
					t.Fatalf("failed to find testalpha01a004: %v", err)
				}
				addr.Set("not_home_tries", 3)
				if err := app.SaveNoValidate(addr); err != nil {
					t.Fatalf("failed to pre-set testalpha01a004 tries: %v", err)
				}

				addr, err = app.FindRecordById("addresses", "testalpha01a003")
				if err != nil {
					t.Fatalf("failed to find testalpha01a003: %v", err)
				}
				addr.Set("source", "app")
				if err := app.SaveNoValidate(addr); err != nil {
					t.Fatalf("failed to mark testalpha01a003 as app sourced: %v", err)
				}

				return app
			},
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
					t.Fatal("expected addresses_log entry for not_home_tries increment, found none")
				}

				mapRecord := waitForMapAggregates(t, app, "testmapalpha01a", 100, map[string]int{"done": 0, "notHome": 0, "notDone": 0, "dnc": 0, "invalid": 0})

				var aggs map[string]interface{}
				if err := json.Unmarshal([]byte(mapRecord.GetString("aggregates")), &aggs); err != nil {
					t.Fatalf("failed to parse aggregates: %v", err)
				}
				for field, want := range map[string]int{"done": 0, "notHome": 0, "notDone": 0, "dnc": 0, "invalid": 0} {
					if got := int(aggs[field].(float64)); got != want {
						t.Errorf("aggregates.%s = %d, want %d", field, got, want)
					}
				}

				territory := waitForTerritoryProgress(t, app, "testterralpha01", 100)
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
//
//	testalpha01a003: not_home, tries=0, max_tries=3
//	testalpha01a004: not_home, tries=0, max_tries=3
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
	withAppSource := func(addrID string, mutate func(testing.TB, *tests.TestApp)) func(testing.TB) *tests.TestApp {
		return func(t testing.TB) *tests.TestApp {
			app := setupTestApp(t)
			if mutate != nil {
				mutate(t, app)
			}

			addr, err := app.FindRecordById("addresses", addrID)
			if err != nil {
				t.Fatalf("failed to find %s: %v", addrID, err)
			}
			addr.Set("source", "app")
			if err := app.SaveNoValidate(addr); err != nil {
				t.Fatalf("failed to mark %s as app sourced: %v", addrID, err)
			}

			return app
		}
	}

	scenarios := []tests.ApiScenario{
		{
			Name:   "done status contributes to numerator — progress 50",
			Method: http.MethodPost,
			URL:    "/address/update",
			Body:   strings.NewReader(`{"address_id":"testalpha01a003","map_id":"testmapalpha01a","status":"done","updated_by":"Admin","notes":""}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory: withAppSource("testalpha01a003", nil),
			ExpectedStatus: 204,
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				mapRecord := waitForMapAggregates(t, app, "testmapalpha01a", 50, map[string]int{"done": 1, "notHome": 1})
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
			Name:   "not_home below max_tries stays in denominator only — progress 50",
			Method: http.MethodPost,
			URL:    "/address/update",
			Body:   strings.NewReader(`{"address_id":"testalpha01a003","map_id":"testmapalpha01a","status":"not_home","not_home_tries":1,"updated_by":"Admin","notes":""}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory: withAppSource("testalpha01a003", func(t testing.TB, app *tests.TestApp) {
				addr, err := app.FindRecordById("addresses", "testalpha01a004")
				if err != nil {
					t.Fatalf("failed to find testalpha01a004: %v", err)
				}
				addr.Set("not_home_tries", 3)
				if err := app.SaveNoValidate(addr); err != nil {
					t.Fatalf("failed to pre-set testalpha01a004: %v", err)
				}
			}),
			ExpectedStatus: 204,
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				mapRecord := waitForMapAggregates(t, app, "testmapalpha01a", 50, map[string]int{"notHome": 1})
				aggs := parseAggs(t, mapRecord)
				if aggInt(aggs, "notHome") != 1 {
					t.Errorf("aggregates.notHome: want 1, got %d", aggInt(aggs, "notHome"))
				}
			},
		},
		{
			Name:   "not_done countable address reduces progress — progress 50",
			Method: http.MethodPost,
			URL:    "/address/update",
			Body:   strings.NewReader(`{"address_id":"testalpha01a004","map_id":"testmapalpha01a","status":"not_home","not_home_tries":3,"updated_by":"Admin","notes":""}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory: withAppSource("testalpha01a004", func(t testing.TB, app *tests.TestApp) {
				addr, err := app.FindRecordById("addresses", "testalpha01a003")
				if err != nil {
					t.Fatalf("failed to find testalpha01a003: %v", err)
				}
				addr.Set("status", "not_done")
				if err := app.SaveNoValidate(addr); err != nil {
					t.Fatalf("failed to reset testalpha01a003 to not_done: %v", err)
				}
			}),
			ExpectedStatus: 204,
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				mapRecord := waitForMapAggregates(t, app, "testmapalpha01a", 50, map[string]int{"notDone": 1})
				aggs := parseAggs(t, mapRecord)
				if aggInt(aggs, "notDone") != 1 {
					t.Errorf("aggregates.notDone: want 1, got %d", aggInt(aggs, "notDone"))
				}
			},
		},
		{
			Name:   "map with no countable addresses keeps progress at 0",
			Method: http.MethodPost,
			URL:    "/address/update",
			Body:   strings.NewReader(`{"address_id":"testalpha01b001","map_id":"testmapalpha01b","status":"done","updated_by":"Admin","notes":""}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory: withAppSource("testalpha01b001", nil),
			ExpectedStatus: 204,
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				mapRecord := waitForMapAggregates(t, app, "testmapalpha01b", 0, nil)
				if got := mapRecord.GetInt("progress"); got != 0 {
					t.Errorf("expected progress 0 for map with no countable addresses, got %d", got)
				}
			},
		},
		{
			Name:   "do_not_call excluded from total — done + dnc gives progress 100",
			Method: http.MethodPost,
			URL:    "/address/update",
			Body:   strings.NewReader(`{"address_id":"testalpha01a003","map_id":"testmapalpha01a","status":"do_not_call","updated_by":"Admin","notes":""}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory: withAppSource("testalpha01a003", func(t testing.TB, app *tests.TestApp) {
				addr, err := app.FindRecordById("addresses", "testalpha01a004")
				if err != nil {
					t.Fatalf("failed to find testalpha01a004: %v", err)
				}
				addr.Set("status", "done")
				if err := app.SaveNoValidate(addr); err != nil {
					t.Fatalf("failed to pre-set testalpha01a004 to done: %v", err)
				}
			}),
			ExpectedStatus: 204,
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				mapRecord := waitForMapAggregates(t, app, "testmapalpha01a", 100, map[string]int{"done": 1, "dnc": 1})
				aggs := parseAggs(t, mapRecord)
				if aggInt(aggs, "done") != 1 {
					t.Errorf("aggregates.done: want 1, got %d", aggInt(aggs, "done"))
				}
				if aggInt(aggs, "dnc") != 1 {
					t.Errorf("aggregates.dnc: want 1, got %d", aggInt(aggs, "dnc"))
				}
			},
		},
		{
			Name:   "invalid excluded from total — invalid + not_done gives progress 0",
			Method: http.MethodPost,
			URL:    "/address/update",
			Body:   strings.NewReader(`{"address_id":"testalpha01a003","map_id":"testmapalpha01a","status":"invalid","updated_by":"Admin","notes":""}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory: withAppSource("testalpha01a003", func(t testing.TB, app *tests.TestApp) {
				addr, err := app.FindRecordById("addresses", "testalpha01a004")
				if err != nil {
					t.Fatalf("failed to find testalpha01a004: %v", err)
				}
				addr.Set("status", "not_done")
				if err := app.SaveNoValidate(addr); err != nil {
					t.Fatalf("failed to pre-set testalpha01a004 to not_done: %v", err)
				}
			}),
			ExpectedStatus: 204,
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				mapRecord := waitForMapAggregates(t, app, "testmapalpha01a", 0, map[string]int{"invalid": 1, "notDone": 1})
				aggs := parseAggs(t, mapRecord)
				if aggInt(aggs, "invalid") != 1 {
					t.Errorf("aggregates.invalid: want 1, got %d", aggInt(aggs, "invalid"))
				}
				if aggInt(aggs, "notDone") != 1 {
					t.Errorf("aggregates.notDone: want 1, got %d", aggInt(aggs, "notDone"))
				}
			},
		},
		{
			Name:   "all countable addresses dnc — total is 0, progress stays 0",
			Method: http.MethodPost,
			URL:    "/address/update",
			Body:   strings.NewReader(`{"address_id":"testalpha01a003","map_id":"testmapalpha01a","status":"do_not_call","updated_by":"Admin","notes":""}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory: withAppSource("testalpha01a003", func(t testing.TB, app *tests.TestApp) {
				addr, err := app.FindRecordById("addresses", "testalpha01a004")
				if err != nil {
					t.Fatalf("failed to find testalpha01a004: %v", err)
				}
				addr.Set("status", "do_not_call")
				if err := app.SaveNoValidate(addr); err != nil {
					t.Fatalf("failed to pre-set testalpha01a004 to do_not_call: %v", err)
				}
			}),
			ExpectedStatus: 204,
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				mapRecord := waitForMapAggregates(t, app, "testmapalpha01a", 0, map[string]int{"dnc": 2})
				aggs := parseAggs(t, mapRecord)
				if aggInt(aggs, "dnc") != 2 {
					t.Errorf("aggregates.dnc: want 2, got %d", aggInt(aggs, "dnc"))
				}
			},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

// TestDomainHook_AggregateStatusReversals verifies that reversing address
// statuses back to not_done correctly re-includes those addresses in the map
// denominator when applicable.
func TestDomainHook_AggregateStatusReversals(t *testing.T) {
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
	withAppSource := func(addrID string, fromStatus string) func(testing.TB) *tests.TestApp {
		return func(t testing.TB) *tests.TestApp {
			app := setupTestApp(t)

			addr, err := app.FindRecordById("addresses", addrID)
			if err != nil {
				t.Fatalf("failed to find %s: %v", addrID, err)
			}
			addr.Set("status", fromStatus)
			addr.Set("source", "app")
			if err := app.SaveNoValidate(addr); err != nil {
				t.Fatalf("failed to pre-set %s: %v", addrID, err)
			}

			otherAddr, err := app.FindRecordById("addresses", "testalpha01a004")
			if err != nil {
				t.Fatalf("failed to find testalpha01a004: %v", err)
			}
			otherAddr.Set("status", "not_done")
			if err := app.SaveNoValidate(otherAddr); err != nil {
				t.Fatalf("failed to pre-set testalpha01a004: %v", err)
			}

			return app
		}
	}
	assertAggBreakdown := func(t testing.TB, mapRecord *core.Record, want map[string]int) {
		t.Helper()
		aggs := parseAggs(t, mapRecord)
		for key, expected := range want {
			if got := aggInt(aggs, key); got != expected {
				t.Errorf("aggregates.%s: want %d, got %d", key, expected, got)
			}
		}
	}

	scenarios := []tests.ApiScenario{
		{
			Name:   "done to not_done adds address back to denominator",
			Method: http.MethodPost,
			URL:    "/address/update",
			Body:   strings.NewReader(`{"address_id":"testalpha01a003","map_id":"testmapalpha01a","status":"not_done","updated_by":"Admin","notes":""}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory: withAppSource("testalpha01a003", "done"),
			ExpectedStatus: 204,
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				mapRecord := waitForMapAggregates(t, app, "testmapalpha01a", 0, map[string]int{"notDone": 2, "done": 0, "notHome": 0, "dnc": 0, "invalid": 0})
				assertAggBreakdown(t, mapRecord, map[string]int{"notDone": 2, "done": 0, "notHome": 0, "dnc": 0, "invalid": 0})
			},
		},
		{
			Name:   "do_not_call to not_done adds excluded address back to totals",
			Method: http.MethodPost,
			URL:    "/address/update",
			Body:   strings.NewReader(`{"address_id":"testalpha01a003","map_id":"testmapalpha01a","status":"not_done","updated_by":"Admin","notes":""}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory: withAppSource("testalpha01a003", "do_not_call"),
			ExpectedStatus: 204,
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				mapRecord := waitForMapAggregates(t, app, "testmapalpha01a", 0, map[string]int{"notDone": 2, "done": 0, "notHome": 0, "dnc": 0, "invalid": 0})
				assertAggBreakdown(t, mapRecord, map[string]int{"notDone": 2, "done": 0, "notHome": 0, "dnc": 0, "invalid": 0})
			},
		},
		{
			Name:   "invalid to not_done adds excluded address back to totals",
			Method: http.MethodPost,
			URL:    "/address/update",
			Body:   strings.NewReader(`{"address_id":"testalpha01a003","map_id":"testmapalpha01a","status":"not_done","updated_by":"Admin","notes":""}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory: withAppSource("testalpha01a003", "invalid"),
			ExpectedStatus: 204,
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				mapRecord := waitForMapAggregates(t, app, "testmapalpha01a", 0, map[string]int{"notDone": 2, "done": 0, "notHome": 0, "dnc": 0, "invalid": 0})
				assertAggBreakdown(t, mapRecord, map[string]int{"notDone": 2, "done": 0, "notHome": 0, "dnc": 0, "invalid": 0})
			},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

// TestDomainHook_AggregateRounding verifies that progress uses math.Round rather
// than int truncation. 2/3 * 100 = 66.666... truncates to 66 but rounds to 67.
//
// Setup: add address_option for testalpha01a001 (making it the third countable
// address in testmapalpha01a alongside 003 and 004). Pre-set testalpha01a004 to
// done, mark testalpha01a003 as source=app, then update testalpha01a003 to done
// so the hook eventually recalculates 2/3 → 67.
func TestDomainHook_AggregateRounding(t *testing.T) {
	adminToken, err := generateToken("admin@alpha.test")
	if err != nil {
		t.Fatal(err)
	}

	scenarios := []tests.ApiScenario{
		{
			Name:   "2/3 progress rounds up to 67 not truncates to 66",
			Method: http.MethodPost,
			URL:    "/address/update",
			Body:   strings.NewReader(`{"address_id":"testalpha01a003","map_id":"testmapalpha01a","status":"done","updated_by":"Admin","notes":""}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			ExpectedStatus: 204,
			TestAppFactory: func(t testing.TB) *tests.TestApp {
				app := setupTestApp(t)

				aoCol, err := app.FindCollectionByNameOrId("address_options")
				if err != nil {
					t.Fatalf("failed to find address_options collection: %v", err)
				}
				ao := core.NewRecord(aoCol)
				ao.Set("address", "testalpha01a001")
				ao.Set("map", "testmapalpha01a")
				ao.Set("congregation", "testcongalpha01")
				ao.Set("option", "testoptialpha01")
				if err := app.SaveNoValidate(ao); err != nil {
					t.Fatalf("failed to add address_option for testalpha01a001: %v", err)
				}

				addr, err := app.FindRecordById("addresses", "testalpha01a004")
				if err != nil {
					t.Fatalf("failed to find testalpha01a004: %v", err)
				}
				addr.Set("status", "done")
				if err := app.SaveNoValidate(addr); err != nil {
					t.Fatalf("failed to pre-set testalpha01a004 to done: %v", err)
				}

				addr, err = app.FindRecordById("addresses", "testalpha01a003")
				if err != nil {
					t.Fatalf("failed to find testalpha01a003: %v", err)
				}
				addr.Set("source", "app")
				if err := app.SaveNoValidate(addr); err != nil {
					t.Fatalf("failed to mark testalpha01a003 as app sourced: %v", err)
				}

				return app
			},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				mapRecord := waitForMapAggregates(t, app, "testmapalpha01a", 67, map[string]int{"done": 2, "notDone": 1})
				if got := mapRecord.GetInt("progress"); got != 67 {
					t.Errorf("expected progress 67 (2/3 rounded), got %d", got)
				}
			},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestDomainHook_AggregateNotTriggeredForIrrelevantFields(t *testing.T) {
	adminToken, err := generateToken("admin@alpha.test")
	if err != nil {
		t.Fatal(err)
	}

	seedSentinelMap := func(t testing.TB, app *tests.TestApp, progress int, aggs map[string]interface{}) {
		t.Helper()
		mapRecord, err := app.FindRecordById("maps", "testmapalpha01a")
		if err != nil {
			t.Fatalf("failed to find testmapalpha01a: %v", err)
		}
		mapRecord.Set("progress", progress)
		mapRecord.Set("aggregates", aggs)
		if err := app.SaveNoValidate(mapRecord); err != nil {
			t.Fatalf("failed to seed sentinel map aggregates: %v", err)
		}
	}
	assertSentinelMap := func(t testing.TB, app *tests.TestApp, wantProgress int, wantAggs map[string]int) {
		t.Helper()
		waitForAsyncAggregateSettle()

		mapRecord, err := app.FindRecordById("maps", "testmapalpha01a")
		if err != nil {
			t.Fatalf("failed to reload map record: %v", err)
		}
		if got := mapRecord.GetInt("progress"); got != wantProgress {
			t.Fatalf("expected progress to remain %d when aggregate hook is skipped, got %d", wantProgress, got)
		}

		var aggs map[string]interface{}
		if err := json.Unmarshal([]byte(mapRecord.GetString("aggregates")), &aggs); err != nil {
			t.Fatalf("failed to parse aggregates: %v", err)
		}
		for field, want := range wantAggs {
			if got := int(aggs[field].(float64)); got != want {
				t.Fatalf("expected aggregates.%s to remain %d when aggregate hook is skipped, got %d", field, want, got)
			}
		}
	}
	withAppSource := func(addrID string, mutate func(testing.TB, *tests.TestApp)) func(testing.TB) *tests.TestApp {
		return func(t testing.TB) *tests.TestApp {
			app := setupTestApp(t)
			if mutate != nil {
				mutate(t, app)
			}

			addr, err := app.FindRecordById("addresses", addrID)
			if err != nil {
				t.Fatalf("failed to find %s: %v", addrID, err)
			}
			addr.Set("source", "app")
			if err := app.SaveNoValidate(addr); err != nil {
				t.Fatalf("failed to mark %s as app sourced: %v", addrID, err)
			}

			seedSentinelMap(t, app, 77, map[string]interface{}{
				"notDone": 9,
				"done":    8,
				"notHome": 7,
				"invalid": 6,
				"dnc":     5,
			})

			return app
		}
	}

	scenarios := []tests.ApiScenario{
		{
			Name:   "notes-only update does not trigger aggregate recalculation",
			Method: http.MethodPost,
			URL:    "/address/update",
			Body:   strings.NewReader(`{"address_id":"testalpha01a003","map_id":"testmapalpha01a","status":"not_home","not_home_tries":0,"updated_by":"Admin","notes":"follow up note"}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			ExpectedStatus: 204,
			TestAppFactory: withAppSource("testalpha01a003", func(t testing.TB, app *tests.TestApp) {
				addr, err := app.FindRecordById("addresses", "testalpha01a003")
				if err != nil {
					t.Fatalf("failed to find testalpha01a003: %v", err)
				}
				addr.Set("updated_by", "Admin")
				if err := app.SaveNoValidate(addr); err != nil {
					t.Fatalf("failed to pre-set testalpha01a003 updated_by: %v", err)
				}
			}),
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				assertSentinelMap(t, app, 77, map[string]int{"notDone": 9, "done": 8, "notHome": 7, "invalid": 6, "dnc": 5})
			},
		},
		{
			Name:   "updated_by-only update does not trigger aggregate recalculation",
			Method: http.MethodPost,
			URL:    "/address/update",
			Body:   strings.NewReader(`{"address_id":"testalpha01a003","map_id":"testmapalpha01a","status":"not_home","not_home_tries":0,"updated_by":"Field Overseer","notes":""}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			ExpectedStatus: 204,
			TestAppFactory: withAppSource("testalpha01a003", nil),
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				assertSentinelMap(t, app, 77, map[string]int{"notDone": 9, "done": 8, "notHome": 7, "invalid": 6, "dnc": 5})
			},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

// TestDomainHook_AggregatesTriggeredForCreationSourceAddresses verifies that
// addresses with creation-time source values (admin, map_init, floor_copy) still
// trigger immediate aggregate recalculation when updated via POST /address/update.
// These source values describe how the address was created, not the update context,
// so field-worker updates to them must recalculate aggregates just like app-sourced
// addresses. Only source=bulk_reset (an update-time marker set by reset handlers)
// suppresses the hook.
func TestDomainHook_AggregatesTriggeredForCreationSourceAddresses(t *testing.T) {
	adminToken, err := generateToken("admin@alpha.test")
	if err != nil {
		t.Fatal(err)
	}

	for _, src := range []string{"admin", "map_init", "floor_copy"} {
		src := src
		scenario := tests.ApiScenario{
			Name:   src + " sourced address update triggers aggregate hook immediately",
			Method: http.MethodPost,
			URL:    "/address/update",
			Body:   strings.NewReader(`{"address_id":"testalpha01a003","map_id":"testmapalpha01a","status":"done","updated_by":"Admin","notes":""}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			ExpectedStatus: 204,
			TestAppFactory: func(t testing.TB) *tests.TestApp {
				app := setupTestApp(t)

				addr, err := app.FindRecordById("addresses", "testalpha01a003")
				if err != nil {
					t.Fatalf("failed to find testalpha01a003: %v", err)
				}
				addr.Set("source", src)
				if err := app.SaveNoValidate(addr); err != nil {
					t.Fatalf("failed to set source=%s on testalpha01a003: %v", src, err)
				}

				return app
			},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				// Hook must fire even though source is a creation-time value, not "app".
				// Progress should update to 50 (1 done out of 2 countable addresses).
				waitForMapAggregates(t, app, "testmapalpha01a", 50, map[string]int{"done": 1, "notHome": 1})
			},
		}
		scenario.Test(t)
	}
}

func TestDomainHook_AggregateTriggeredForTriesChangeOnDoneStatus(t *testing.T) {
	adminToken, err := generateToken("admin@alpha.test")
	if err != nil {
		t.Fatal(err)
	}

	scenarios := []tests.ApiScenario{
		{
			Name:   "tries change on done status still triggers aggregate recalculation",
			Method: http.MethodPost,
			URL:    "/address/update",
			Body:   strings.NewReader(`{"address_id":"testalpha01a003","map_id":"testmapalpha01a","status":"done","not_home_tries":2,"updated_by":"Admin","notes":""}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			ExpectedStatus: 204,
			TestAppFactory: func(t testing.TB) *tests.TestApp {
				app := setupTestApp(t)

				addr, err := app.FindRecordById("addresses", "testalpha01a003")
				if err != nil {
					t.Fatalf("failed to find testalpha01a003: %v", err)
				}
				addr.Set("source", "admin")
				addr.Set("status", "done")
				if err := app.SaveNoValidate(addr); err != nil {
					t.Fatalf("failed to pre-set testalpha01a003 to done with suppressed source: %v", err)
				}

				addr, err = app.FindRecordById("addresses", "testalpha01a003")
				if err != nil {
					t.Fatalf("failed to reload testalpha01a003: %v", err)
				}
				addr.Set("source", "app")
				if err := app.SaveNoValidate(addr); err != nil {
					t.Fatalf("failed to mark testalpha01a003 as app sourced: %v", err)
				}

				mapRecord, err := app.FindRecordById("maps", "testmapalpha01a")
				if err != nil {
					t.Fatalf("failed to find testmapalpha01a: %v", err)
				}
				mapRecord.Set("progress", 77)
				mapRecord.Set("aggregates", map[string]interface{}{
					"notDone": 9,
					"done":    8,
					"notHome": 7,
					"invalid": 6,
					"dnc":     5,
				})
				if err := app.SaveNoValidate(mapRecord); err != nil {
					t.Fatalf("failed to seed sentinel aggregates: %v", err)
				}

				return app
			},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				mapRecord := waitForMapAggregates(t, app, "testmapalpha01a", 50, map[string]int{"done": 1, "notHome": 1})
				if got := mapRecord.GetInt("progress"); got != 50 {
					t.Errorf("expected progress 50 after false-positive aggregate recalculation, got %d", got)
				}
			},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestDomainHook_AggregateSkippedForMissingMapID(t *testing.T) {
	app := setupTestApp(t)
	defer app.Cleanup()

	addr, err := app.FindRecordById("addresses", "testalpha01a003")
	if err != nil {
		t.Fatalf("failed to find testalpha01a003: %v", err)
	}
	addr.Set("map", "")
	if err := app.SaveNoValidate(addr); err != nil {
		t.Fatalf("failed to clear map on testalpha01a003: %v", err)
	}

	mapRecord, err := app.FindRecordById("maps", "testmapalpha01a")
	if err != nil {
		t.Fatalf("failed to find testmapalpha01a: %v", err)
	}
	mapRecord.Set("progress", 77)
	if err := app.SaveNoValidate(mapRecord); err != nil {
		t.Fatalf("failed to seed sentinel progress on testmapalpha01a: %v", err)
	}

	addr, err = app.FindRecordById("addresses", "testalpha01a003")
	if err != nil {
		t.Fatalf("failed to reload testalpha01a003: %v", err)
	}

	nextStatus := "done"
	if addr.GetString("status") == nextStatus {
		nextStatus = "not_done"
	}
	addr.Set("source", "app")
	addr.Set("status", nextStatus)
	if err := app.SaveNoValidate(addr); err != nil {
		t.Fatalf("failed to trigger aggregate hook with empty map on testalpha01a003: %v", err)
	}

	waitForAsyncAggregateSettle()

	mapRecord, err = app.FindRecordById("maps", "testmapalpha01a")
	if err != nil {
		t.Fatalf("failed to reload testmapalpha01a: %v", err)
	}
	if got := mapRecord.GetInt("progress"); got != 77 {
		t.Fatalf("expected progress to remain 77 when aggregate hook is skipped for empty map, got %d", got)
	}
}

func TestDomainHook_AggregatesSuppressedForBulkResetSource(t *testing.T) {
	adminToken, err := generateToken("admin@alpha.test")
	if err != nil {
		t.Fatal(err)
	}

	scenarios := []tests.ApiScenario{
		{
			Name:   "bulk_reset sourced address update skips aggregate hook until explicit processing",
			Method: http.MethodPost,
			URL:    "/address/update",
			Body:   strings.NewReader(`{"address_id":"testalpha01a003","map_id":"testmapalpha01a","status":"done","updated_by":"Admin","notes":""}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			ExpectedStatus: 204,
			TestAppFactory: func(t testing.TB) *tests.TestApp {
				app := setupTestApp(t)

				addr, err := app.FindRecordById("addresses", "testalpha01a003")
				if err != nil {
					t.Fatalf("failed to find testalpha01a003: %v", err)
				}
				addr.Set("source", "bulk_reset")
				if err := app.SaveNoValidate(addr); err != nil {
					t.Fatalf("failed to mark testalpha01a003 as bulk_reset sourced: %v", err)
				}

				mapRecord, err := app.FindRecordById("maps", "testmapalpha01a")
				if err != nil {
					t.Fatalf("failed to find testmapalpha01a: %v", err)
				}
				mapRecord.Set("progress", 77)
				mapRecord.Set("aggregates", map[string]interface{}{
					"notDone": 9,
					"done":    8,
					"notHome": 7,
					"invalid": 6,
					"dnc":     5,
				})
				if err := app.SaveNoValidate(mapRecord); err != nil {
					t.Fatalf("failed to seed sentinel aggregates: %v", err)
				}

				return app
			},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				mapRecord, err := app.FindRecordById("maps", "testmapalpha01a")
				if err != nil {
					t.Fatalf("failed to find map record: %v", err)
				}
				if got := mapRecord.GetInt("progress"); got != 77 {
					t.Fatalf("expected progress to remain 77 when hook is suppressed, got %d", got)
				}

				var aggs map[string]interface{}
				if err := json.Unmarshal([]byte(mapRecord.GetString("aggregates")), &aggs); err != nil {
					t.Fatalf("failed to parse aggregates: %v", err)
				}
				for field, want := range map[string]int{"notDone": 9, "done": 8, "notHome": 7, "invalid": 6, "dnc": 5} {
					if got := int(aggs[field].(float64)); got != want {
						t.Fatalf("expected sentinel aggregates.%s=%d before explicit processing, got %d", field, want, got)
					}
				}

				if err := handlers.ProcessMapAggregates("testmapalpha01a", app); err != nil {
					t.Fatalf("explicit aggregate processing failed: %v", err)
				}

				mapRecord, err = app.FindRecordById("maps", "testmapalpha01a")
				if err != nil {
					t.Fatalf("failed to reload map record: %v", err)
				}
				if got := mapRecord.GetInt("progress"); got != 50 {
					t.Errorf("expected progress 50 after explicit processing, got %d", got)
				}
				if err := json.Unmarshal([]byte(mapRecord.GetString("aggregates")), &aggs); err != nil {
					t.Fatalf("failed to parse recalculated aggregates: %v", err)
				}
				if got := int(aggs["done"].(float64)); got != 1 {
					t.Errorf("expected aggregates.done 1 after explicit processing, got %d", got)
				}
				if got := int(aggs["notHome"].(float64)); got != 1 {
					t.Errorf("expected aggregates.notHome 1 after explicit processing, got %d", got)
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
