//go:build testdata

package setup

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

func TestHandleUpdateAddress(t *testing.T) {
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
		// ── Input validation ─────────────────────────────────────────────────────

		{
			Name:   "invalid JSON body returns 400",
			Method: http.MethodPost,
			URL:    "/address/update",
			Body:   strings.NewReader(`not-json`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  400,
			ExpectedContent: []string{`"status":400`},
		},
		{
			Name:   "missing address_id returns 400",
			Method: http.MethodPost,
			URL:    "/address/update",
			Body:   strings.NewReader(`{"map_id":"testmapalpha01a","status":"not_done"}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  400,
			ExpectedContent: []string{`"status":400`},
		},
		{
			Name:   "missing map_id returns 400",
			Method: http.MethodPost,
			URL:    "/address/update",
			Body:   strings.NewReader(`{"address_id":"testalpha01a001","status":"not_done"}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  400,
			ExpectedContent: []string{`"status":400`},
		},

		// ── Authentication / authorisation ────────────────────────────────────

		{
			Name:   "no auth and no link-id returns 403",
			Method: http.MethodPost,
			URL:    "/address/update",
			Body: strings.NewReader(`{
				"address_id": "testalpha01a001",
				"map_id":     "testmapalpha01a",
				"status":     "not_done"
			}`),
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Unauthorized."`},
		},
		{
			Name:   "link-id for a different map returns 403",
			Method: http.MethodPost,
			URL:    "/address/update",
			Body: strings.NewReader(`{
				"address_id": "testalpha01a001",
				"map_id":     "testmapalpha01a",
				"status":     "not_done"
			}`),
			Headers: map[string]string{
				"Content-Type": "application/json",
				// testassignbeta001 belongs to testmapbeta001a, not testmapalpha01a
				"link-id": "testassignbeta001",
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Unauthorized."`},
		},
		{
			Name:   "user with no role in the map congregation returns 403",
			Method: http.MethodPost,
			URL:    "/address/update",
			Body: strings.NewReader(`{
				"address_id": "testalpha01a001",
				"map_id":     "testmapalpha01a",
				"status":     "not_done"
			}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": betaAdminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Unauthorized."`},
		},

		// ── Record ownership checks ───────────────────────────────────────────

		{
			Name:   "non-existent address returns 404",
			Method: http.MethodPost,
			URL:    "/address/update",
			Body: strings.NewReader(`{
				"address_id": "doesnotexist000",
				"map_id":     "testmapalpha01a",
				"status":     "not_done"
			}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  404,
			ExpectedContent: []string{`"status":404`},
		},
		{
			Name:   "address belonging to a different map returns 403",
			Method: http.MethodPost,
			URL:    "/address/update",
			Body: strings.NewReader(`{
				"address_id": "testalpha01b001",
				"map_id":     "testmapalpha01a",
				"status":     "not_done"
			}`),
			// testalpha01b001 is in testmapalpha01b, not testmapalpha01a
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Address does not belong to the specified map."`},
		},
		{
			Name:   "deleting an address_option that belongs to a different address returns 403",
			Method: http.MethodPost,
			URL:    "/address/update",
			Body: strings.NewReader(`{
				"address_id":     "testalpha01a003",
				"map_id":         "testmapalpha01a",
				"status":         "not_home",
				"delete_ao_ids":  ["testaoalph01002"]
			}`),
			// testaoalph01002 belongs to testalpha01a004, not testalpha01a003
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"The address_option does not belong to this address."`},
		},

		// ── Happy paths ───────────────────────────────────────────────────────

		{
			Name:   "admin updates address fields successfully",
			Method: http.MethodPost,
			URL:    "/address/update",
			Body: strings.NewReader(`{
				"address_id":    "testalpha01a001",
				"map_id":        "testmapalpha01a",
				"notes":         "Knock loudly",
				"status":        "not_home",
				"not_home_tries": 2,
				"updated_by":    "test-user"
			}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory: setupTestApp,
			ExpectedStatus: http.StatusNoContent,
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				addr, err := app.FindRecordById("addresses", "testalpha01a001")
				if err != nil {
					t.Fatalf("could not fetch address: %v", err)
				}
				if addr.GetString("notes") != "Knock loudly" {
					t.Errorf("expected notes='Knock loudly', got %q", addr.GetString("notes"))
				}
				if addr.GetString("status") != "not_home" {
					t.Errorf("expected status='not_home', got %q", addr.GetString("status"))
				}
				if addr.GetInt("not_home_tries") != 2 {
					t.Errorf("expected not_home_tries=2, got %d", addr.GetInt("not_home_tries"))
				}
				if addr.GetString("updated_by") != "test-user" {
					t.Errorf("expected updated_by='test-user', got %q", addr.GetString("updated_by"))
				}
			},
		},
		{
			Name:   "conductor updates address fields successfully",
			Method: http.MethodPost,
			URL:    "/address/update",
			Body: strings.NewReader(`{
				"address_id": "testalpha01a002",
				"map_id":     "testmapalpha01a",
				"status":     "done",
				"updated_by": "conductor-user"
			}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			TestAppFactory: setupTestApp,
			ExpectedStatus: http.StatusNoContent,
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				addr, err := app.FindRecordById("addresses", "testalpha01a002")
				if err != nil {
					t.Fatalf("could not fetch address: %v", err)
				}
				if addr.GetString("status") != "done" {
					t.Errorf("expected status='done', got %q", addr.GetString("status"))
				}
			},
		},
		{
			Name:   "publisher link-id updates address successfully",
			Method: http.MethodPost,
			URL:    "/address/update",
			Body: strings.NewReader(`{
				"address_id": "testalpha01a001",
				"map_id":     "testmapalpha01a",
				"status":     "not_home",
				"updated_by": "link-publisher"
			}`),
			Headers: map[string]string{
				"Content-Type": "application/json",
				// testassignalpha01 is linked to testmapalpha01a
				"link-id": "testassignalpha01",
			},
			TestAppFactory: setupTestApp,
			ExpectedStatus: http.StatusNoContent,
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				addr, err := app.FindRecordById("addresses", "testalpha01a001")
				if err != nil {
					t.Fatalf("could not fetch address: %v", err)
				}
				if addr.GetString("status") != "not_home" {
					t.Errorf("expected status='not_home', got %q", addr.GetString("status"))
				}
			},
		},
		{
			Name:   "delete existing address_option and add a new one atomically",
			Method: http.MethodPost,
			URL:    "/address/update",
			Body: strings.NewReader(`{
				"address_id":    "testalpha01a003",
				"map_id":        "testmapalpha01a",
				"status":        "not_home",
				"delete_ao_ids": ["testaoalph01001"],
				"add_option_ids": ["testoptialpha02"]
			}`),
			// testaoalph01001: NH option on testalpha01a003; replacing with DNC
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory: setupTestApp,
			ExpectedStatus: http.StatusNoContent,
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				// Old AO (NH) must be gone
				if _, err := app.FindRecordById("address_options", "testaoalph01001"); err == nil {
					t.Error("expected testaoalph01001 to be deleted")
				}
				// New AO (DNC) must exist
				recs, err := app.FindRecordsByFilter(
					"address_options",
					`address = "testalpha01a003" && option = "testoptialpha02"`,
					"", 0, 0,
				)
				if err != nil {
					t.Fatalf("query failed: %v", err)
				}
				if len(recs) != 1 {
					t.Errorf("expected 1 new address_option for DNC, got %d", len(recs))
				}
			},
		},
		{
			Name:   "deleting a non-existent address_option id is treated as success (idempotent)",
			Method: http.MethodPost,
			URL:    "/address/update",
			Body: strings.NewReader(`{
				"address_id":    "testalpha01a001",
				"map_id":        "testmapalpha01a",
				"status":        "not_done",
				"delete_ao_ids": ["ao-already-gone-000"]
			}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  http.StatusNoContent,
		},
		{
			Name:   "adding an option that already exists is silently skipped (UNIQUE constraint)",
			Method: http.MethodPost,
			URL:    "/address/update",
			Body: strings.NewReader(`{
				"address_id":     "testalpha01a003",
				"map_id":         "testmapalpha01a",
				"status":         "not_home",
				"add_option_ids": ["testoptialpha01"]
			}`),
			// testalpha01a003 already has testoptialpha01 (NH) via testaoalph01001
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory: setupTestApp,
			ExpectedStatus: http.StatusNoContent,
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				// Original record must still be there (not duplicated)
				recs, err := app.FindRecordsByFilter(
					"address_options",
					`address = "testalpha01a003" && option = "testoptialpha01"`,
					"", 0, 0,
				)
				if err != nil {
					t.Fatalf("query failed: %v", err)
				}
				if len(recs) != 1 {
					t.Errorf("expected exactly 1 NH address_option, got %d", len(recs))
				}
			},
		},
		{
			Name:   "coordinates are stored when provided as a JSON object",
			Method: http.MethodPost,
			URL:    "/address/update",
			Body: strings.NewReader(`{
				"address_id":  "testalpha01a001",
				"map_id":      "testmapalpha01a",
				"status":      "not_done",
				"coordinates": {"lat": 1.3521, "lng": 103.8198}
			}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory: setupTestApp,
			ExpectedStatus: http.StatusNoContent,
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				addr, err := app.FindRecordById("addresses", "testalpha01a001")
				if err != nil {
					t.Fatalf("could not fetch address: %v", err)
				}
				coords := addr.Get("coordinates")
				if coords == nil {
					t.Error("expected coordinates to be set, got nil")
				}
			},
		},
		{
			Name:   "coordinates are cleared when provided as null",
			Method: http.MethodPost,
			URL:    "/address/update",
			Body: strings.NewReader(`{
				"address_id":  "testalpha01a001",
				"map_id":      "testmapalpha01a",
				"status":      "not_done",
				"coordinates": null
			}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory: setupTestApp,
			ExpectedStatus: http.StatusNoContent,
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				addr, err := app.FindRecordById("addresses", "testalpha01a001")
				if err != nil {
					t.Fatalf("could not fetch address: %v", err)
				}
				coordStr := addr.GetString("coordinates")
				if coordStr != "" && coordStr != "null" {
					t.Errorf("expected coordinates to be null/empty after clearing, got %q", coordStr)
				}
			},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

// TestHandleUpdateAddress_StatusTransitions exhaustively covers every status value,
// every meaningful transition, the audit-log hook, the notes-stamp hook, and the
// associated scalar fields (not_home_tries, dnc_time, updated_by) — all going through
// the /address/update endpoint so the full hook chain is exercised end-to-end.
//
// Seed reference (testmapalpha01a):
//   testalpha01a001  not_done   tries=0  notes=""  no AO
//   testalpha01a002  not_done   tries=0  notes=""  no AO
//   testalpha01a003  not_home   tries=0  notes=""  AO: NH (testaoalph01001)
//   testalpha01a004  not_home   tries=0  notes=""  AO: NH (testaoalph01002)
//   testalpha01a005  done       tries=0  notes=""  no AO
func TestHandleUpdateAddress_StatusTransitions(t *testing.T) {
	adminToken, err := generateToken("admin@alpha.test")
	if err != nil {
		t.Fatal(err)
	}

	// ── helpers ──────────────────────────────────────────────────────────────────

	// preSet mutates an address record before the HTTP request fires.
	preSet := func(addrId string, fields map[string]any) func(testing.TB) *tests.TestApp {
		return func(t testing.TB) *tests.TestApp {
			app := setupTestApp(t)
			addr, err := app.FindRecordById("addresses", addrId)
			if err != nil {
				t.Fatalf("preSet: cannot find %s: %v", addrId, err)
			}
			for k, v := range fields {
				addr.Set(k, v)
			}
			if err := app.SaveNoValidate(addr); err != nil {
				t.Fatalf("preSet: cannot save %s: %v", addrId, err)
			}
			return app
		}
	}

	// postAddr fetches an address record after the request.
	postAddr := func(t testing.TB, app *tests.TestApp, id string) *core.Record {
		t.Helper()
		rec, err := app.FindRecordById("addresses", id)
		if err != nil {
			t.Fatalf("postAddr: cannot find %s: %v", id, err)
		}
		return rec
	}

	// logEntries returns all addresses_log rows for the given address, oldest first.
	logEntries := func(t testing.TB, app *tests.TestApp, addrId string) []*core.Record {
		t.Helper()
		recs, err := app.FindRecordsByFilter(
			"addresses_log", "address = {:id}", "created", 0, 0,
			map[string]any{"id": addrId},
		)
		if err != nil {
			t.Fatalf("logEntries: query failed: %v", err)
		}
		return recs
	}

	// ── Status persistence: each valid value ─────────────────────────────────────

	statusCases := []struct {
		name   string
		seedID string // address to update
		body   string
		want   string // expected status after update
	}{
		{
			name:   "status not_done is stored",
			seedID: "testalpha01a003", // starts as not_home
			body:   `{"address_id":"testalpha01a003","map_id":"testmapalpha01a","status":"not_done","not_home_tries":0,"updated_by":"u"}`,
			want:   "not_done",
		},
		{
			name:   "status not_home is stored",
			seedID: "testalpha01a001",
			body:   `{"address_id":"testalpha01a001","map_id":"testmapalpha01a","status":"not_home","not_home_tries":1,"updated_by":"u"}`,
			want:   "not_home",
		},
		{
			name:   "status done is stored",
			seedID: "testalpha01a001",
			body:   `{"address_id":"testalpha01a001","map_id":"testmapalpha01a","status":"done","updated_by":"u"}`,
			want:   "done",
		},
		{
			name:   "status do_not_call is stored",
			seedID: "testalpha01a001",
			body:   `{"address_id":"testalpha01a001","map_id":"testmapalpha01a","status":"do_not_call","dnc_time":"2026-01-01T00:00:00.000Z","updated_by":"u"}`,
			want:   "do_not_call",
		},
		{
			name:   "status invalid is stored",
			seedID: "testalpha01a001",
			body:   `{"address_id":"testalpha01a001","map_id":"testmapalpha01a","status":"invalid","updated_by":"u"}`,
			want:   "invalid",
		},
	}

	for _, tc := range statusCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			s := tests.ApiScenario{
				Name:   tc.name,
				Method: http.MethodPost,
				URL:    "/address/update",
				Body:   strings.NewReader(tc.body),
				Headers: map[string]string{
					"Content-Type":  "application/json",
					"Authorization": adminToken,
				},
				TestAppFactory: setupTestApp,
				ExpectedStatus: http.StatusNoContent,
				AfterTestFunc: func(tb testing.TB, app *tests.TestApp, res *http.Response) {
					addr := postAddr(tb, app, tc.seedID)
					if got := addr.GetString("status"); got != tc.want {
						tb.Errorf("status: want %q, got %q", tc.want, got)
					}
				},
			}
			s.Test(t)
		})
	}

	// ── Status transitions with audit-log verification ────────────────────────────
	//
	// The OnRecordAfterUpdateSuccess hook writes an addresses_log entry whenever
	// status changes OR not_home_tries changes while status is not_home.
	// All writes go through /address/update, not the raw PocketBase API.

	type transitionCase struct {
		name      string
		factory   func(testing.TB) *tests.TestApp // nil = setupTestApp
		addrID    string
		body      string
		wantLog   bool   // expect a log entry?
		wantOld   string // expected old_status in log (only checked when wantLog=true)
		wantNew   string // expected new_status in log
		wantLogBy string // expected changed_by in log
	}

	transitions := []transitionCase{
		// ── Transitions that MUST produce a log entry ────────────────────────────
		{
			name:    "not_done → not_home produces log",
			addrID:  "testalpha01a001",
			body:    `{"address_id":"testalpha01a001","map_id":"testmapalpha01a","status":"not_home","not_home_tries":1,"updated_by":"pub1"}`,
			wantLog: true, wantOld: "not_done", wantNew: "not_home", wantLogBy: "pub1",
		},
		{
			name:    "not_done → done produces log",
			addrID:  "testalpha01a001",
			body:    `{"address_id":"testalpha01a001","map_id":"testmapalpha01a","status":"done","updated_by":"pub2"}`,
			wantLog: true, wantOld: "not_done", wantNew: "done", wantLogBy: "pub2",
		},
		{
			name:    "not_done → do_not_call produces log",
			addrID:  "testalpha01a001",
			body:    `{"address_id":"testalpha01a001","map_id":"testmapalpha01a","status":"do_not_call","dnc_time":"2026-05-01T00:00:00.000Z","updated_by":"pub3"}`,
			wantLog: true, wantOld: "not_done", wantNew: "do_not_call", wantLogBy: "pub3",
		},
		{
			name:    "not_done → invalid produces log",
			addrID:  "testalpha01a001",
			body:    `{"address_id":"testalpha01a001","map_id":"testmapalpha01a","status":"invalid","updated_by":"pub4"}`,
			wantLog: true, wantOld: "not_done", wantNew: "invalid", wantLogBy: "pub4",
		},
		{
			// testalpha01a005 is seeded as done; reset to not_done.
			name:    "done → not_done produces log",
			addrID:  "testalpha01a005",
			body:    `{"address_id":"testalpha01a005","map_id":"testmapalpha01a","status":"not_done","not_home_tries":0,"updated_by":"admin1"}`,
			wantLog: true, wantOld: "done", wantNew: "not_done", wantLogBy: "admin1",
		},
		{
			// testalpha01a003 is seeded as not_home; transition to done.
			name:    "not_home → done produces log",
			addrID:  "testalpha01a003",
			body:    `{"address_id":"testalpha01a003","map_id":"testmapalpha01a","status":"done","not_home_tries":0,"updated_by":"admin2"}`,
			wantLog: true, wantOld: "not_home", wantNew: "done", wantLogBy: "admin2",
		},
		{
			// testalpha01a003 is seeded as not_home; reset to not_done.
			name:    "not_home → not_done produces log",
			addrID:  "testalpha01a003",
			body:    `{"address_id":"testalpha01a003","map_id":"testmapalpha01a","status":"not_done","not_home_tries":0,"updated_by":"admin3"}`,
			wantLog: true, wantOld: "not_home", wantNew: "not_done", wantLogBy: "admin3",
		},
		{
			// Pre-set address to do_not_call; reset to not_done.
			name:    "do_not_call → not_done produces log",
			factory: preSet("testalpha01a001", map[string]any{"status": "do_not_call"}),
			addrID:  "testalpha01a001",
			body:    `{"address_id":"testalpha01a001","map_id":"testmapalpha01a","status":"not_done","updated_by":"admin4"}`,
			wantLog: true, wantOld: "do_not_call", wantNew: "not_done", wantLogBy: "admin4",
		},
		{
			// Pre-set address to invalid; reset to not_done.
			name:    "invalid → not_done produces log",
			factory: preSet("testalpha01a001", map[string]any{"status": "invalid"}),
			addrID:  "testalpha01a001",
			body:    `{"address_id":"testalpha01a001","map_id":"testmapalpha01a","status":"not_done","updated_by":"admin5"}`,
			wantLog: true, wantOld: "invalid", wantNew: "not_done", wantLogBy: "admin5",
		},
		{
			// not_home + tries increment without status change → log entry required
			// because the aggregate bucket shifts.
			name:    "not_home + tries increment (0→2) produces log even though status unchanged",
			addrID:  "testalpha01a003",
			body:    `{"address_id":"testalpha01a003","map_id":"testmapalpha01a","status":"not_home","not_home_tries":2,"updated_by":"pub5"}`,
			wantLog: true, wantOld: "not_home", wantNew: "not_home", wantLogBy: "pub5",
		},

		// ── Transitions that must NOT produce a log entry ────────────────────────
		{
			// Same status, same tries → no log.
			name:    "not_done → not_done (no change) produces no log",
			addrID:  "testalpha01a001",
			body:    `{"address_id":"testalpha01a001","map_id":"testmapalpha01a","status":"not_done","not_home_tries":0,"updated_by":"pub6"}`,
			wantLog: false,
		},
		{
			// not_home same status, same tries value → no log.
			name:    "not_home + same tries (0→0) produces no log",
			addrID:  "testalpha01a003",
			body:    `{"address_id":"testalpha01a003","map_id":"testmapalpha01a","status":"not_home","not_home_tries":0,"updated_by":"pub7"}`,
			wantLog: false,
		},
		{
			// done → done (no status change) → no log.
			name:    "done → done (no change) produces no log",
			addrID:  "testalpha01a005",
			body:    `{"address_id":"testalpha01a005","map_id":"testmapalpha01a","status":"done","updated_by":"pub8"}`,
			wantLog: false,
		},
	}

	for _, tc := range transitions {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			factory := tc.factory
			if factory == nil {
				factory = setupTestApp
			}
			s := tests.ApiScenario{
				Name:   tc.name,
				Method: http.MethodPost,
				URL:    "/address/update",
				Body:   strings.NewReader(tc.body),
				Headers: map[string]string{
					"Content-Type":  "application/json",
					"Authorization": adminToken,
				},
				TestAppFactory: factory,
				ExpectedStatus: http.StatusNoContent,
				AfterTestFunc: func(tb testing.TB, app *tests.TestApp, res *http.Response) {
					logs := logEntries(tb, app, tc.addrID)
					if tc.wantLog {
						if len(logs) == 0 {
							tb.Fatalf("expected addresses_log entry, found none")
						}
						// preSet may produce an earlier log entry; take the last one.
						entry := logs[len(logs)-1]
						if got := entry.GetString("old_status"); got != tc.wantOld {
							tb.Errorf("old_status: want %q, got %q", tc.wantOld, got)
						}
						if got := entry.GetString("new_status"); got != tc.wantNew {
							tb.Errorf("new_status: want %q, got %q", tc.wantNew, got)
						}
						if got := entry.GetString("changed_by"); got != tc.wantLogBy {
							tb.Errorf("changed_by: want %q, got %q", tc.wantLogBy, got)
						}
						// Log must reference correct map and congregation.
						if got := entry.GetString("map"); got != "testmapalpha01a" {
							tb.Errorf("log.map: want testmapalpha01a, got %q", got)
						}
						if got := entry.GetString("congregation"); got != "testcongalpha01" {
							tb.Errorf("log.congregation: want testcongalpha01, got %q", got)
						}
					} else {
						if len(logs) != 0 {
							tb.Errorf("expected no addresses_log entry, found %d", len(logs))
						}
					}
				},
			}
			s.Test(t)
		})
	}

	// ── Associated scalar fields ──────────────────────────────────────────────────

	scalarScenarios := []tests.ApiScenario{
		{
			Name:   "not_home_tries value is persisted correctly",
			Method: http.MethodPost,
			URL:    "/address/update",
			Body: strings.NewReader(`{
				"address_id":     "testalpha01a001",
				"map_id":         "testmapalpha01a",
				"status":         "not_home",
				"not_home_tries": 3,
				"updated_by":     "u"
			}`),
			Headers:        map[string]string{"Content-Type": "application/json", "Authorization": adminToken},
			TestAppFactory: setupTestApp,
			ExpectedStatus: http.StatusNoContent,
			AfterTestFunc: func(tb testing.TB, app *tests.TestApp, res *http.Response) {
				if got := postAddr(tb, app, "testalpha01a001").GetInt("not_home_tries"); got != 3 {
					tb.Errorf("not_home_tries: want 3, got %d", got)
				}
			},
		},
		{
			Name:   "not_home_tries resets to 0 when explicitly sent as 0",
			Method: http.MethodPost,
			URL:    "/address/update",
			Body: strings.NewReader(`{
				"address_id":     "testalpha01a003",
				"map_id":         "testmapalpha01a",
				"status":         "not_home",
				"not_home_tries": 0,
				"updated_by":     "u"
			}`),
			Headers: map[string]string{"Content-Type": "application/json", "Authorization": adminToken},
			TestAppFactory: preSet("testalpha01a003", map[string]any{"not_home_tries": 2}),
			ExpectedStatus: http.StatusNoContent,
			AfterTestFunc: func(tb testing.TB, app *tests.TestApp, res *http.Response) {
				if got := postAddr(tb, app, "testalpha01a003").GetInt("not_home_tries"); got != 0 {
					tb.Errorf("not_home_tries: want 0 after reset, got %d", got)
				}
			},
		},
		{
			Name:   "dnc_time is stored when status is do_not_call",
			Method: http.MethodPost,
			URL:    "/address/update",
			Body: strings.NewReader(`{
				"address_id": "testalpha01a001",
				"map_id":     "testmapalpha01a",
				"status":     "do_not_call",
				"dnc_time":   "2026-05-01T12:00:00.000Z",
				"updated_by": "u"
			}`),
			Headers:        map[string]string{"Content-Type": "application/json", "Authorization": adminToken},
			TestAppFactory: setupTestApp,
			ExpectedStatus: http.StatusNoContent,
			AfterTestFunc: func(tb testing.TB, app *tests.TestApp, res *http.Response) {
				if got := postAddr(tb, app, "testalpha01a001").GetString("dnc_time"); got == "" {
					tb.Error("dnc_time: expected non-empty value, got empty")
				}
			},
		},
		{
			Name:   "updated_by is persisted",
			Method: http.MethodPost,
			URL:    "/address/update",
			Body: strings.NewReader(`{
				"address_id": "testalpha01a001",
				"map_id":     "testmapalpha01a",
				"status":     "not_done",
				"updated_by": "specific-publisher-id"
			}`),
			Headers:        map[string]string{"Content-Type": "application/json", "Authorization": adminToken},
			TestAppFactory: setupTestApp,
			ExpectedStatus: http.StatusNoContent,
			AfterTestFunc: func(tb testing.TB, app *tests.TestApp, res *http.Response) {
				if got := postAddr(tb, app, "testalpha01a001").GetString("updated_by"); got != "specific-publisher-id" {
					tb.Errorf("updated_by: want %q, got %q", "specific-publisher-id", got)
				}
			},
		},
	}

	for _, s := range scalarScenarios {
		s.Test(t)
	}

	// ── Notes hook: last_notes_updated / last_notes_updated_by ───────────────────

	notesScenarios := []tests.ApiScenario{
		{
			Name:   "notes change stamps last_notes_updated and last_notes_updated_by",
			Method: http.MethodPost,
			URL:    "/address/update",
			Body: strings.NewReader(`{
				"address_id": "testalpha01a001",
				"map_id":     "testmapalpha01a",
				"status":     "not_done",
				"notes":      "New delivery note",
				"updated_by": "note-changer"
			}`),
			Headers:        map[string]string{"Content-Type": "application/json", "Authorization": adminToken},
			TestAppFactory: setupTestApp,
			ExpectedStatus: http.StatusNoContent,
			AfterTestFunc: func(tb testing.TB, app *tests.TestApp, res *http.Response) {
				addr := postAddr(tb, app, "testalpha01a001")
				if addr.GetString("last_notes_updated") == "" {
					tb.Error("last_notes_updated: expected a timestamp after notes change, got empty")
				}
				if got := addr.GetString("last_notes_updated_by"); got != "note-changer" {
					tb.Errorf("last_notes_updated_by: want %q, got %q", "note-changer", got)
				}
			},
		},
		{
			// Send the same notes value that already exists in the seed ("").
			// The hook must NOT update last_notes_updated or last_notes_updated_by.
			Name:   "same notes value does not update last_notes_updated",
			Method: http.MethodPost,
			URL:    "/address/update",
			Body: strings.NewReader(`{
				"address_id": "testalpha01a001",
				"map_id":     "testmapalpha01a",
				"status":     "not_done",
				"notes":      "",
				"updated_by": "anyone"
			}`),
			Headers:        map[string]string{"Content-Type": "application/json", "Authorization": adminToken},
			TestAppFactory: setupTestApp,
			ExpectedStatus: http.StatusNoContent,
			AfterTestFunc: func(tb testing.TB, app *tests.TestApp, res *http.Response) {
				addr := postAddr(tb, app, "testalpha01a001")
				if ts := addr.GetString("last_notes_updated"); ts != "" {
					tb.Errorf("last_notes_updated: expected empty when notes unchanged, got %q", ts)
				}
				if by := addr.GetString("last_notes_updated_by"); by != "" {
					tb.Errorf("last_notes_updated_by: expected empty when notes unchanged, got %q", by)
				}
			},
		},
		{
			// Notes change with notes previously set: stamp must update to new updater.
			Name:   "notes change on pre-existing note updates last_notes_updated_by to new updater",
			Method: http.MethodPost,
			URL:    "/address/update",
			Body: strings.NewReader(`{
				"address_id": "testalpha01a001",
				"map_id":     "testmapalpha01a",
				"status":     "not_done",
				"notes":      "Updated again",
				"updated_by": "second-editor"
			}`),
			Headers: map[string]string{"Content-Type": "application/json", "Authorization": adminToken},
			TestAppFactory: preSet("testalpha01a001", map[string]any{
				"notes":                 "First note",
				"last_notes_updated_by": "first-editor",
			}),
			ExpectedStatus: http.StatusNoContent,
			AfterTestFunc: func(tb testing.TB, app *tests.TestApp, res *http.Response) {
				addr := postAddr(tb, app, "testalpha01a001")
				if got := addr.GetString("last_notes_updated_by"); got != "second-editor" {
					tb.Errorf("last_notes_updated_by: want %q, got %q", "second-editor", got)
				}
			},
		},
	}

	for _, s := range notesScenarios {
		s.Test(t)
	}
}
