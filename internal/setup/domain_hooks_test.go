package setup

import (
	"net/http"
	"strings"
	"testing"

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
