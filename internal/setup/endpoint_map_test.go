package setup

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/tests"
)

func TestHandleResetMap(t *testing.T) {
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
			Name:   "guest (no auth) is rejected",
			Method: http.MethodPost,
			URL:    "/map/reset",
			Body:   strings.NewReader(`{"map":"testmapalpha01a"}`),
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  401,
			ExpectedContent: []string{`"status":401`},
		},
		{
			Name:   "conductor cannot reset map (403)",
			Method: http.MethodPost,
			URL:    "/map/reset",
			Body:   strings.NewReader(`{"map":"testmapalpha01a"}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Administrator access required."`},
		},
		{
			Name:   "admin from different congregation cannot reset map (403)",
			Method: http.MethodPost,
			URL:    "/map/reset",
			Body:   strings.NewReader(`{"map":"testmapalpha01a"}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": betaAdminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Administrator access required."`},
		},
		{
			Name:   "non-existent map returns 404",
			Method: http.MethodPost,
			URL:    "/map/reset",
			Body:   strings.NewReader(`{"map":"doesnotexist01"}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  404,
			ExpectedContent: []string{`"status":404`},
		},
		{
			Name:   "admin resets seeded single map successfully",
			Method: http.MethodPost,
			URL:    "/map/reset",
			Body:   strings.NewReader(`{"map":"testmapalpha01a"}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  http.StatusOK,
			ExpectedContent: []string{`"Map reset successfully"`},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				records, err := app.FindRecordsByFilter(
					"addresses",
					"map = 'testmapalpha01a' && (status = 'not_home' || status = 'done')",
					"", 0, 0,
				)
				if err != nil {
					t.Fatalf("query failed: %v", err)
				}
				if len(records) != 0 {
					t.Errorf("expected 0 residual not_home/done records, got %d", len(records))
				}
			},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestHandleNewMap(t *testing.T) {
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
			Name:   "conductor cannot create map (403)",
			Method: http.MethodPost,
			URL:    "/map/add",
			Body: strings.NewReader(`{
				"territory":    "testterralpha01",
				"congregation": "testcongalpha01",
				"type":         "single",
				"floors":       1,
				"name":         "Blk 888 Test",
				"coordinates":  "1.3714,103.8494",
				"sequence":     "01,02,03"
			}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Administrator access required."`},
		},
		{
			Name:   "admin from different congregation cannot create map (403)",
			Method: http.MethodPost,
			URL:    "/map/add",
			Body: strings.NewReader(`{
				"territory":    "testterralpha01",
				"congregation": "testcongalpha01",
				"type":         "single",
				"floors":       1,
				"name":         "Blk 888 Test",
				"coordinates":  "1.3714,103.8494",
				"sequence":     "01,02,03"
			}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": betaAdminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Administrator access required."`},
		},
		{
			Name:   "admin creates a valid single-floor map",
			Method: http.MethodPost,
			URL:    "/map/add",
			Body: strings.NewReader(`{
				"territory":    "testterralpha01",
				"congregation": "testcongalpha01",
				"type":         "single",
				"floors":       1,
				"name":         "Blk 999 Test",
				"coordinates":  "1.3714,103.8494",
				"sequence":     "01,02,03"
			}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  http.StatusOK,
			ExpectedContent: []string{`"type":"single"`},
		},
		{
			Name:   "single map with floors > 1 is rejected",
			Method: http.MethodPost,
			URL:    "/map/add",
			Body: strings.NewReader(`{
				"territory":    "testterralpha01",
				"congregation": "testcongalpha01",
				"type":         "single",
				"floors":       3,
				"name":         "Bad Map",
				"coordinates":  "1.3714,103.8494",
				"sequence":     "01,02,03"
			}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  http.StatusBadRequest,
			ExpectedContent: []string{`"status":400`},
		},
		{
			Name:   "invalid sequence format is rejected",
			Method: http.MethodPost,
			URL:    "/map/add",
			Body: strings.NewReader(`{
				"territory":    "testterralpha01",
				"congregation": "testcongalpha01",
				"type":         "single",
				"floors":       1,
				"name":         "Bad Sequence Map",
				"coordinates":  "1.3714,103.8494",
				"sequence":     "01, 02, 03"
			}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  http.StatusBadRequest,
			ExpectedContent: []string{`"status":400`},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestHandleGetMapAddresses_WithAuth(t *testing.T) {
	conductorToken, err := generateToken("conductor@alpha.test")
	if err != nil {
		t.Fatal(err)
	}

	scenarios := []tests.ApiScenario{
		{
			Name:   "conductor fetches addresses for seeded map",
			Method: http.MethodPost,
			URL:    "/map/addresses",
			Body:   strings.NewReader(`{"map_id":"testmapalpha01a"}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  http.StatusOK,
			ExpectedContent: []string{`"status":"not_done"`},
		},
		{
			Name:   "guest with no auth or link-id is rejected",
			Method: http.MethodPost,
			URL:    "/map/addresses",
			Body:   strings.NewReader(`{"map_id":"testmapalpha01a"}`),
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  http.StatusUnauthorized,
			ExpectedContent: []string{`"Unauthorized"`},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestHandleGetMapCodes(t *testing.T) {
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
			Name:   "no auth is rejected with 401",
			Method: http.MethodPost,
			URL:    "/map/codes",
			Body:   strings.NewReader(`{"map_id":"testmapalpha01a"}`),
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  401,
			ExpectedContent: []string{`"status":401`},
		},
		{
			Name:   "link-id without JWT is rejected with 401",
			Method: http.MethodPost,
			URL:    "/map/codes",
			Body:   strings.NewReader(`{"map_id":"testmapalpha01a"}`),
			Headers: map[string]string{
				"Content-Type": "application/json",
				"link-id":      "testassignalpha01",
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  401,
			ExpectedContent: []string{`"status":401`},
		},
		{
			Name:   "missing map_id returns 400",
			Method: http.MethodPost,
			URL:    "/map/codes",
			Body:   strings.NewReader(`{}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  400,
			ExpectedContent: []string{`"map_id is required"`},
		},
		{
			Name:   "conductor cannot get map codes (403)",
			Method: http.MethodPost,
			URL:    "/map/codes",
			Body:   strings.NewReader(`{"map_id":"testmapalpha01a"}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Administrator access required."`},
		},
		{
			Name:   "admin from different congregation is rejected with 403",
			Method: http.MethodPost,
			URL:    "/map/codes",
			Body:   strings.NewReader(`{"map_id":"testmapalpha01a"}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": betaAdminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Administrator access required."`},
		},
		{
			Name:   "valid request returns codes and type",
			Method: http.MethodPost,
			URL:    "/map/codes",
			Body:   strings.NewReader(`{"map_id":"testmapalpha01a"}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"codes"`, `"type"`},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestHandleMapAdd_Integration(t *testing.T) {
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
			Name:   "conductor cannot add address code (403)",
			Method: http.MethodPost,
			URL:    "/map/code/add",
			Body:   strings.NewReader(`{"map":"testmapalpha01b","codes":["NN"]}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Administrator access required."`},
		},
		{
			Name:   "admin from different congregation cannot add address code (403)",
			Method: http.MethodPost,
			URL:    "/map/code/add",
			Body:   strings.NewReader(`{"map":"testmapalpha01b","codes":["NN"]}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": betaAdminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Administrator access required."`},
		},
		{
			Name:   "no auth is rejected with 401",
			Method: http.MethodPost,
			URL:    "/map/code/add",
			Body:   strings.NewReader(`{"map":"testmapalpha01b","codes":["NN"]}`),
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  401,
			ExpectedContent: []string{`"status":401`},
		},
		{
			Name:   "empty codes array returns 400",
			Method: http.MethodPost,
			URL:    "/map/code/add",
			Body:   strings.NewReader(`{"map":"testmapalpha01b","codes":[]}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  400,
			ExpectedContent: []string{`"Codes array is required and cannot be empty."`},
		},
		{
			Name:   "invalid code format returns 400",
			Method: http.MethodPost,
			URL:    "/map/code/add",
			Body:   strings.NewReader(`{"map":"testmapalpha01b","codes":["A_B"]}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  400,
			ExpectedContent: []string{`alphanumeric characters and hyphens.`},
		},
		{
			Name:   "valid new code NN inserted into testmapalpha01b",
			Method: http.MethodPost,
			URL:    "/map/code/add",
			Body:   strings.NewReader(`{"map":"testmapalpha01b","codes":["NN"]}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"codes_inserted":1`},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				records, err := app.FindRecordsByFilter(
					"addresses",
					"map = 'testmapalpha01b' && code = 'NN'",
					"", 0, 0,
				)
				if err != nil {
					t.Fatalf("query failed: %v", err)
				}
				if len(records) == 0 {
					t.Error("expected new address with code NN in testmapalpha01b, found none")
				}
			},
		},
		{
			Name:   "all codes exist returns 200 with codes_inserted 0",
			Method: http.MethodPost,
			URL:    "/map/code/add",
			Body:   strings.NewReader(`{"map":"testmapalpha01a","codes":["10"]}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"codes_inserted":0`},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestHandleMapUpdateSequence(t *testing.T) {
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
			Name:   "conductor cannot update sequence (403)",
			Method: http.MethodPost,
			URL:    "/map/codes/update",
			Body:   strings.NewReader(`{"map":"testmapalpha01a","codes":[{"code":"10","sequence":1}]}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Administrator access required."`},
		},
		{
			Name:   "admin from different congregation cannot update sequence (403)",
			Method: http.MethodPost,
			URL:    "/map/codes/update",
			Body:   strings.NewReader(`{"map":"testmapalpha01a","codes":[{"code":"10","sequence":1}]}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": betaAdminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Administrator access required."`},
		},
		{
			Name:   "no auth is rejected with 401",
			Method: http.MethodPost,
			URL:    "/map/codes/update",
			Body:   strings.NewReader(`{"map":"testmapalpha01a","codes":[{"code":"10","sequence":1}]}`),
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  401,
			ExpectedContent: []string{`"status":401`},
		},
		{
			Name:   "missing map field returns 400",
			Method: http.MethodPost,
			URL:    "/map/codes/update",
			Body:   strings.NewReader(`{"codes":[{"code":"10","sequence":1}]}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  400,
			ExpectedContent: []string{`"Map is required."`},
		},
		{
			Name:   "valid sequence update returns 200",
			Method: http.MethodPost,
			URL:    "/map/codes/update",
			Body:   strings.NewReader(`{"map":"testmapalpha01a","codes":[{"code":"10","sequence":5},{"code":"11","sequence":4},{"code":"12","sequence":3},{"code":"13","sequence":2},{"code":"14","sequence":1}]}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`Address sequences updated successfully`},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestHandleMapDelete(t *testing.T) {
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
			Name:   "conductor cannot delete address code (403)",
			Method: http.MethodPost,
			URL:    "/map/code/delete",
			Body:   strings.NewReader(`{"code":"10","map":"testmapalpha01a"}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Administrator access required."`},
		},
		{
			Name:   "admin from different congregation cannot delete address code (403)",
			Method: http.MethodPost,
			URL:    "/map/code/delete",
			Body:   strings.NewReader(`{"code":"10","map":"testmapalpha01a"}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": betaAdminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Administrator access required."`},
		},
		{
			Name:   "no auth is rejected with 401",
			Method: http.MethodPost,
			URL:    "/map/code/delete",
			Body:   strings.NewReader(`{"code":"10","map":"testmapalpha01a"}`),
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  401,
			ExpectedContent: []string{`"status":401`},
		},
		{
			Name:   "delete last code from single-code map returns 400",
			Method: http.MethodPost,
			URL:    "/map/code/delete",
			Body:   strings.NewReader(`{"code":"99","map":"testmapalphsc01"}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  400,
			ExpectedContent: []string{`"Cannot delete the last address code."`},
		},
		{
			Name:   "delete code 10 from testmapalpha01a succeeds",
			Method: http.MethodPost,
			URL:    "/map/code/delete",
			Body:   strings.NewReader(`{"code":"10","map":"testmapalpha01a"}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`Addresses code deleted successfully`},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				records, err := app.FindRecordsByFilter(
					"addresses",
					"map = 'testmapalpha01a' && code = '10'",
					"", 0, 0,
				)
				if err != nil {
					t.Fatalf("query failed: %v", err)
				}
				if len(records) != 0 {
					t.Errorf("expected address with code '10' to be deleted, but found %d records", len(records))
				}
			},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestHandleMapFloorAdd(t *testing.T) {
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
			Name:   "conductor cannot add floor (403)",
			Method: http.MethodPost,
			URL:    "/map/floor/add",
			Body:   strings.NewReader(`{"map":"testmapalpha01b","add_higher":true}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Administrator access required."`},
		},
		{
			Name:   "admin from different congregation cannot add floor (403)",
			Method: http.MethodPost,
			URL:    "/map/floor/add",
			Body:   strings.NewReader(`{"map":"testmapalpha01b","add_higher":true}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": betaAdminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Administrator access required."`},
		},
		{
			Name:   "no auth is rejected with 401",
			Method: http.MethodPost,
			URL:    "/map/floor/add",
			Body:   strings.NewReader(`{"map":"testmapalpha01b","add_higher":true}`),
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  401,
			ExpectedContent: []string{`"status":401`},
		},
		{
			Name:   "add higher floor to testmapalpha01b creates floor 2 with 5 addresses",
			Method: http.MethodPost,
			URL:    "/map/floor/add",
			Body:   strings.NewReader(`{"map":"testmapalpha01b","add_higher":true}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`Map floor updated successfully`},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				records, err := app.FindRecordsByFilter(
					"addresses",
					"map = 'testmapalpha01b' && floor = 2",
					"", 0, 0,
				)
				if err != nil {
					t.Fatalf("query failed: %v", err)
				}
				if len(records) != 5 {
					t.Errorf("expected 5 addresses on floor 2, got %d", len(records))
				}
			},
		},
		{
			Name:   "add lower floor to testmapalpha02a creates floor 0 with 5 addresses",
			Method: http.MethodPost,
			URL:    "/map/floor/add",
			Body:   strings.NewReader(`{"map":"testmapalpha02a","add_higher":false}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`Map floor updated successfully`},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				// floor below 1 is 0, but handler skips 0 and uses -1
				records, err := app.FindRecordsByFilter(
					"addresses",
					"map = 'testmapalpha02a' && floor = -1",
					"", 0, 0,
				)
				if err != nil {
					t.Fatalf("query failed: %v", err)
				}
				if len(records) != 5 {
					t.Errorf("expected 5 addresses on floor -1, got %d", len(records))
				}
			},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestHandleMapFloorRemove(t *testing.T) {
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
			Name:   "conductor cannot remove floor (403)",
			Method: http.MethodPost,
			URL:    "/map/floor/remove",
			Body:   strings.NewReader(`{"map":"testmapalphcf01","floor":2}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Administrator access required."`},
		},
		{
			Name:   "admin from different congregation cannot remove floor (403)",
			Method: http.MethodPost,
			URL:    "/map/floor/remove",
			Body:   strings.NewReader(`{"map":"testmapalphcf01","floor":2}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": betaAdminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Administrator access required."`},
		},
		{
			Name:   "no auth is rejected with 401",
			Method: http.MethodPost,
			URL:    "/map/floor/remove",
			Body:   strings.NewReader(`{"map":"testmapalpha01a","floor":1}`),
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  401,
			ExpectedContent: []string{`"status":401`},
		},
		{
			Name:   "remove only floor from single-floor map returns 400",
			Method: http.MethodPost,
			URL:    "/map/floor/remove",
			Body:   strings.NewReader(`{"map":"testmapalpha01a","floor":1}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  400,
			ExpectedContent: []string{`"Cannot delete the last floor."`},
		},
		{
			Name:   "remove floor 2 from testmapalphcf01 succeeds",
			Method: http.MethodPost,
			URL:    "/map/floor/remove",
			Body:   strings.NewReader(`{"map":"testmapalphcf01","floor":2}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`Map floor deleted successfully`},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				records, err := app.FindRecordsByFilter(
					"addresses",
					"map = 'testmapalphcf01' && floor = 2",
					"", 0, 0,
				)
				if err != nil {
					t.Fatalf("query failed: %v", err)
				}
				if len(records) != 0 {
					t.Errorf("expected 0 addresses on floor 2 after removal, got %d", len(records))
				}
			},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestHandleMapTerritoryUpdate(t *testing.T) {
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
			Name:   "conductor cannot update map territory (403)",
			Method: http.MethodPost,
			URL:    "/map/territory/update",
			Body:   strings.NewReader(`{"map":"testmapalpha01a","old_territory":"testterralpha01","new_territory":"testterralpha02"}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Administrator access required."`},
		},
		{
			Name:   "admin from different congregation cannot update map territory (403)",
			Method: http.MethodPost,
			URL:    "/map/territory/update",
			Body:   strings.NewReader(`{"map":"testmapalpha01a","old_territory":"testterralpha01","new_territory":"testterralpha02"}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": betaAdminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Administrator access required."`},
		},
		{
			Name:   "no auth is rejected with 401",
			Method: http.MethodPost,
			URL:    "/map/territory/update",
			Body:   strings.NewReader(`{"map":"testmapalpha01a","old_territory":"testterralpha01","new_territory":"testterralpha02"}`),
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  401,
			ExpectedContent: []string{`"status":401`},
		},
		{
			Name:   "valid territory update moves map to new territory",
			Method: http.MethodPost,
			URL:    "/map/territory/update",
			Body:   strings.NewReader(`{"map":"testmapalpha01a","old_territory":"testterralpha01","new_territory":"testterralpha02"}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`Map territory updated successfully`},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				mapRecord, err := app.FindRecordById("maps", "testmapalpha01a")
				if err != nil {
					t.Fatalf("failed to fetch map: %v", err)
				}
				if mapRecord.GetString("territory") != "testterralpha02" {
					t.Errorf("expected map territory to be testterralpha02, got %s", mapRecord.GetString("territory"))
				}
			},
		},
		{
			Name:   "move map back from testterralpha02 to testterralpha01 succeeds",
			Method: http.MethodPost,
			URL:    "/map/territory/update",
			Body:   strings.NewReader(`{"map":"testmapalpha01b","old_territory":"testterralpha01","new_territory":"testterralpha02"}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`Map territory updated successfully`},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				mapRecord, err := app.FindRecordById("maps", "testmapalpha01b")
				if err != nil {
					t.Fatalf("failed to fetch map: %v", err)
				}
				if mapRecord.GetString("territory") != "testterralpha02" {
					t.Errorf("expected map territory to be testterralpha02, got %s", mapRecord.GetString("territory"))
				}
			},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}
