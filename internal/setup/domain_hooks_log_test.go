package setup

import (
	"net/http"
	"strings"
	"testing"

	"ministry-mapper/internal/jobs"

	"github.com/pocketbase/pocketbase/tests"
)

func TestDomainHook_LogAssignmentCreatedDeleted(t *testing.T) {
	adminToken, err := generateToken("admin@alpha.test")
	if err != nil {
		t.Fatal(err)
	}

	scenarios := []tests.ApiScenario{
		{
			Name:   "creating assignment writes assigned log entry with changed_by",
			Method: http.MethodPost,
			URL:    "/api/collections/assignments/records",
			Body: strings.NewReader(`{
				"map":"testmapalpha01a",
				"congregation":"testcongalpha01",
				"publisher":"Test Publisher",
				"expiry_date":"2099-01-01 00:00:00.000Z",
				"type":"normal"
			}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory: setupTestApp,
			ExpectedStatus: 200,
			ExpectedContent: []string{`"collectionName":"assignments"`},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				logs, err := app.FindRecordsByFilter("assignments_log", "action = 'assigned'", "", 0, 0)
				if err != nil {
					t.Fatalf("failed to query assignments_log: %v", err)
				}
				if len(logs) == 0 {
					t.Fatal("expected assignments_log entry for created assignment, found none")
				}
				if got := logs[0].GetString("changed_by"); got != "testuseralpha01" {
					t.Errorf("expected changed_by 'testuseralpha01', got %q", got)
				}
				if got := logs[0].GetString("congregation"); got != "testcongalpha01" {
					t.Errorf("expected congregation 'testcongalpha01', got %q", got)
				}
				if got := logs[0].GetString("map"); got != "testmapalpha01a" {
					t.Errorf("expected map 'testmapalpha01a', got %q", got)
				}
			},
		},
		{
			Name:   "deleting assignment writes unassigned log entry with changed_by",
			Method: http.MethodDelete,
			URL:    "/api/collections/assignments/records/testassignalpha01",
			Headers: map[string]string{
				"Authorization": adminToken,
			},
			TestAppFactory: setupTestApp,
			ExpectedStatus: 204,
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				logs, err := app.FindRecordsByFilter("assignments_log", "action = 'unassigned'", "", 0, 0)
				if err != nil {
					t.Fatalf("failed to query assignments_log: %v", err)
				}
				if len(logs) == 0 {
					t.Fatal("expected assignments_log entry for deleted assignment, found none")
				}
				if got := logs[0].GetString("changed_by"); got != "testuseralpha01" {
					t.Errorf("expected changed_by 'testuseralpha01', got %q", got)
				}
				if got := logs[0].GetString("map"); got != "testmapalpha01a" {
					t.Errorf("expected map 'testmapalpha01a', got %q", got)
				}
			},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

// TestDomainHook_LogAssignmentExpired verifies that the cleanup cron job writes
// an expired log entry with an empty changed_by (no authenticated user in cron context).
func TestDomainHook_LogAssignmentExpired(t *testing.T) {
	app := setupTestApp(t)
	defer app.Cleanup()

	if err := jobs.RunAssignmentsCleanup(app); err != nil {
		t.Fatalf("cleanup job failed: %v", err)
	}

	logs, err := app.FindRecordsByFilter("assignments_log", "action = 'expired'", "", 0, 0)
	if err != nil {
		t.Fatalf("failed to query assignments_log: %v", err)
	}
	if len(logs) == 0 {
		t.Fatal("expected assignments_log entry for expired assignment, found none")
	}
	if got := logs[0].GetString("changed_by"); got != "" {
		t.Errorf("expected empty changed_by for cron expiry, got %q", got)
	}
	if got := logs[0].GetString("map"); got != "testmapalpha01a" {
		t.Errorf("expected map 'testmapalpha01a', got %q", got)
	}
}

func TestDomainHook_LogRole(t *testing.T) {
	adminToken, err := generateToken("admin@alpha.test")
	if err != nil {
		t.Fatal(err)
	}

	scenarios := []tests.ApiScenario{
		{
			Name:   "creating role writes granted log entry",
			Method: http.MethodPost,
			URL:    "/api/collections/roles/records",
			Body: strings.NewReader(`{
				"congregation":"testcongalpha01",
				"user":"testuseralpha03",
				"role":"conductor"
			}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory: setupTestApp,
			ExpectedStatus: 200,
			ExpectedContent: []string{`"collectionName":"roles"`},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				logs, err := app.FindRecordsByFilter("roles_log", "action = 'granted'", "", 0, 0)
				if err != nil {
					t.Fatalf("failed to query roles_log: %v", err)
				}
				if len(logs) == 0 {
					t.Fatal("expected roles_log entry for granted role, found none")
				}
				if got := logs[0].GetString("new_role"); got != "conductor" {
					t.Errorf("expected new_role 'conductor', got %q", got)
				}
				if got := logs[0].GetString("old_role"); got != "" {
					t.Errorf("expected empty old_role for grant, got %q", got)
				}
				if got := logs[0].GetString("changed_by"); got != "testuseralpha01" {
					t.Errorf("expected changed_by 'testuseralpha01', got %q", got)
				}
			},
		},
		{
			// testrolexcng01c is read_only; patching to conductor must write a changed entry.
			Name:   "patching role to a different level writes changed log entry",
			Method: http.MethodPatch,
			URL:    "/api/collections/roles/records/testrolexcng01c",
			Body:   strings.NewReader(`{"role":"conductor"}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory: setupTestApp,
			ExpectedStatus: 200,
			ExpectedContent: []string{`"collectionName":"roles"`},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				logs, err := app.FindRecordsByFilter("roles_log", "action = 'changed'", "", 0, 0)
				if err != nil {
					t.Fatalf("failed to query roles_log: %v", err)
				}
				if len(logs) == 0 {
					t.Fatal("expected roles_log entry for changed role, found none")
				}
				if got := logs[0].GetString("old_role"); got != "read_only" {
					t.Errorf("expected old_role 'read_only', got %q", got)
				}
				if got := logs[0].GetString("new_role"); got != "conductor" {
					t.Errorf("expected new_role 'conductor', got %q", got)
				}
			},
		},
		{
			// Patching a role to its current value must produce no log entry.
			Name:   "patching role to same level writes no log entry",
			Method: http.MethodPatch,
			URL:    "/api/collections/roles/records/testrolexcng01c",
			Body:   strings.NewReader(`{"role":"read_only"}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory: setupTestApp,
			ExpectedStatus: 200,
			ExpectedContent: []string{`"collectionName":"roles"`},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				logs, err := app.FindRecordsByFilter("roles_log", "id != ''", "", 0, 0)
				if err != nil {
					t.Fatalf("failed to query roles_log: %v", err)
				}
				if len(logs) != 0 {
					t.Errorf("expected no roles_log entries for same-role PATCH, found %d", len(logs))
				}
			},
		},
		{
			// testrolexcng01c is read_only; deleting it must write a revoked entry.
			Name:   "deleting role writes revoked log entry",
			Method: http.MethodDelete,
			URL:    "/api/collections/roles/records/testrolexcng01c",
			Headers: map[string]string{
				"Authorization": adminToken,
			},
			TestAppFactory: setupTestApp,
			ExpectedStatus: 204,
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				logs, err := app.FindRecordsByFilter("roles_log", "action = 'revoked'", "", 0, 0)
				if err != nil {
					t.Fatalf("failed to query roles_log: %v", err)
				}
				if len(logs) == 0 {
					t.Fatal("expected roles_log entry for revoked role, found none")
				}
				if got := logs[0].GetString("old_role"); got != "read_only" {
					t.Errorf("expected old_role 'read_only', got %q", got)
				}
				if got := logs[0].GetString("new_role"); got != "" {
					t.Errorf("expected empty new_role for revoke, got %q", got)
				}
				if got := logs[0].GetString("changed_by"); got != "testuseralpha01" {
					t.Errorf("expected changed_by 'testuseralpha01', got %q", got)
				}
			},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}
