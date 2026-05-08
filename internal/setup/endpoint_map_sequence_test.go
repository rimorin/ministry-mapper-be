package setup

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/tests"
)

func TestHandleUpdateTerritoryMapSequence(t *testing.T) {
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

	// testterralpha01 has 4 maps: testmapalpha01a (seq 1), testmapalpha01b (seq 2),
	// testmapalphsc01 (seq 10), testmapalphcf01 (seq 11)
	allMaps := `["testmapalpha01a","testmapalpha01b","testmapalphsc01","testmapalphcf01"]`
	reversedMaps := `["testmapalphcf01","testmapalphsc01","testmapalpha01b","testmapalpha01a"]`

	scenarios := []tests.ApiScenario{
		{
			Name:            "guest (no auth) is rejected",
			Method:          http.MethodPost,
			URL:             "/maps/sequence",
			Body:            strings.NewReader(`{"territory_id":"testterralpha01","map_ids":` + allMaps + `}`),
			Headers:         map[string]string{"Content-Type": "application/json"},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  401,
			ExpectedContent: []string{`"status":401`},
		},
		{
			Name:   "conductor is rejected (403)",
			Method: http.MethodPost,
			URL:    "/maps/sequence",
			Body:   strings.NewReader(`{"territory_id":"testterralpha01","map_ids":` + allMaps + `}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Administrator access required."`},
		},
		{
			Name:   "admin from different congregation is rejected (403)",
			Method: http.MethodPost,
			URL:    "/maps/sequence",
			Body:   strings.NewReader(`{"territory_id":"testterralpha01","map_ids":` + allMaps + `}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": betaAdminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Administrator access required."`},
		},
		{
			Name:   "non-existent territory returns 404",
			Method: http.MethodPost,
			URL:    "/maps/sequence",
			Body:   strings.NewReader(`{"territory_id":"doesnotexist01","map_ids":` + allMaps + `}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  404,
			ExpectedContent: []string{`"status":404`},
		},
		{
			Name:   "missing territory_id returns 400",
			Method: http.MethodPost,
			URL:    "/maps/sequence",
			Body:   strings.NewReader(`{"map_ids":` + allMaps + `}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  400,
			ExpectedContent: []string{`"status":400`},
		},
		{
			Name:   "empty map_ids returns 400",
			Method: http.MethodPost,
			URL:    "/maps/sequence",
			Body:   strings.NewReader(`{"territory_id":"testterralpha01","map_ids":[]}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  400,
			ExpectedContent: []string{`"status":400`},
		},
		{
			Name:   "duplicate map_ids returns 400",
			Method: http.MethodPost,
			URL:    "/maps/sequence",
			Body:   strings.NewReader(`{"territory_id":"testterralpha01","map_ids":["testmapalpha01a","testmapalpha01a","testmapalphsc01","testmapalphcf01"]}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  400,
			ExpectedContent: []string{`"status":400`},
		},
		{
			Name:   "partial map_ids (missing one) returns 400",
			Method: http.MethodPost,
			URL:    "/maps/sequence",
			Body:   strings.NewReader(`{"territory_id":"testterralpha01","map_ids":["testmapalpha01a","testmapalpha01b"]}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  400,
			ExpectedContent: []string{`"status":400`},
		},
		{
			Name:   "map from different territory returns 400",
			Method: http.MethodPost,
			URL:    "/maps/sequence",
			Body:   strings.NewReader(`{"territory_id":"testterralpha01","map_ids":["testmapalpha01a","testmapalpha01b","testmapalphsc01","testmapalpha02a"]}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  400,
			ExpectedContent: []string{`"status":400`},
		},
		{
			Name:   "admin reorders maps successfully",
			Method: http.MethodPost,
			URL:    "/maps/sequence",
			Body:   strings.NewReader(`{"territory_id":"testterralpha01","map_ids":` + reversedMaps + `}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  http.StatusOK,
			ExpectedContent: []string{`Map sequences updated`},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				expected := map[string]int{
					"testmapalphcf01":  1,
					"testmapalphsc01":  2,
					"testmapalpha01b":  3,
					"testmapalpha01a":  4,
				}
				for id, wantSeq := range expected {
					rec, err := app.FindRecordById("maps", id)
					if err != nil {
						t.Errorf("could not fetch map %s: %v", id, err)
						continue
					}
					if got := rec.GetInt("sequence"); got != wantSeq {
						t.Errorf("map %s: expected sequence %d, got %d", id, wantSeq, got)
					}
				}
			},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}
