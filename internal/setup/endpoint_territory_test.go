package setup

import (
	"net/http"
	"strings"
	"testing"

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
			Body:   strings.NewReader(`{"territory":"testterralpha01","coordinates":{"lat":1.3521,"lng":103.8198}}`),
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
			Body:   strings.NewReader(`{"territory":"testterralpha01","coordinates":{"lat":1.3521,"lng":103.8198}}`),
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
			Body:   strings.NewReader(`{"territory":"testterralpha01","coordinates":{"lat":1.3521,"lng":103.8198}}`),
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
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}
