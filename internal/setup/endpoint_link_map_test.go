//go:build testdata

package setup

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/tests"
)

func TestHandleGetLinkMap(t *testing.T) {
	scenarios := []tests.ApiScenario{
		// ── Auth ─────────────────────────────────────────────────────────────

		{
			Name:   "missing link-id header returns 401",
			Method: http.MethodPost,
			URL:    "/link/map",
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  401,
			ExpectedContent: []string{`"error":"Unauthorized"`},
		},
		{
			Name:   "non-existent link-id returns 401",
			Method: http.MethodPost,
			URL:    "/link/map",
			Headers: map[string]string{
				"Content-Type": "application/json",
				"link-id":      "doesnotexist000",
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  401,
			ExpectedContent: []string{`"error":"Unauthorized"`},
		},
		{
			Name:   "expired link-id returns 401",
			Method: http.MethodPost,
			URL:    "/link/map",
			Headers: map[string]string{
				"Content-Type": "application/json",
				"link-id":      "testassignexprd01",
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  401,
			ExpectedContent: []string{`"error":"Unauthorized"`},
		},

		// ── Happy path ────────────────────────────────────────────────────────

		{
			Name:   "valid alpha link-id returns map, congregation, addresses and pinned flag",
			Method: http.MethodPost,
			URL:    "/link/map",
			Headers: map[string]string{
				"Content-Type": "application/json",
				"link-id":      "testassignalpha01",
			},
			TestAppFactory: setupTestApp,
			ExpectedStatus: 200,
			ExpectedContent: []string{
				`"publisher":"Test Publisher Alpha"`,
				`"id":"testmapalpha01a"`,
				`"type":"single"`,
				`"id":"testcongalpha01"`,
				`"has_pinned_messages":true`,
				`"description":"Blk 100A"`,
			},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				var body struct {
					ExpiryDate string `json:"expiry_date"`
					Publisher  string `json:"publisher"`
					Map        struct {
						Id   string `json:"id"`
						Type string `json:"type"`
					} `json:"map"`
					Congregation struct {
						Id      string `json:"id"`
						Options []struct {
							Id       string `json:"id"`
							Sequence int    `json:"sequence"`
						} `json:"options"`
					} `json:"congregation"`
					Addresses []struct {
						Id string `json:"id"`
					} `json:"addresses"`
					HasPinnedMessages bool `json:"has_pinned_messages"`
				}
				if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
					t.Fatalf("failed to decode response body: %v", err)
				}

				if body.ExpiryDate == "" {
					t.Error("expected expiry_date to be set")
				}
				if body.Map.Id != "testmapalpha01a" {
					t.Errorf("expected map.id = testmapalpha01a, got %s", body.Map.Id)
				}
				if body.Congregation.Id != "testcongalpha01" {
					t.Errorf("expected congregation.id = testcongalpha01, got %s", body.Congregation.Id)
				}

				// Alpha congregation has 3 options; verify they arrive sorted by sequence.
				if len(body.Congregation.Options) != 3 {
					t.Errorf("expected 3 congregation options, got %d", len(body.Congregation.Options))
				}
				for i := 1; i < len(body.Congregation.Options); i++ {
					if body.Congregation.Options[i].Sequence < body.Congregation.Options[i-1].Sequence {
						t.Errorf("options not sorted by sequence at index %d", i)
					}
				}

				// testmapalpha01a has 5 addresses in seed data.
				if len(body.Addresses) != 5 {
					t.Errorf("expected 5 addresses, got %d", len(body.Addresses))
				}

				if !body.HasPinnedMessages {
					t.Error("expected has_pinned_messages = true for testmapalpha01a")
				}
			},
		},
		{
			Name:   "valid beta link-id returns correct congregation and no pinned messages",
			Method: http.MethodPost,
			URL:    "/link/map",
			Headers: map[string]string{
				"Content-Type": "application/json",
				"link-id":      "testassignbeta001",
			},
			TestAppFactory: setupTestApp,
			ExpectedStatus: 200,
			ExpectedContent: []string{
				`"publisher":"Test Publisher Beta"`,
				`"id":"testmapbeta001a"`,
				`"id":"testcongbeta001"`,
				`"has_pinned_messages":false`,
			},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				var body struct {
					Congregation struct {
						Options []struct {
							Id string `json:"id"`
						} `json:"options"`
					} `json:"congregation"`
					Addresses []struct {
						Id string `json:"id"`
					} `json:"addresses"`
					HasPinnedMessages bool `json:"has_pinned_messages"`
				}
				if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
					t.Fatalf("failed to decode response body: %v", err)
				}

				// Beta congregation has 2 options.
				if len(body.Congregation.Options) != 2 {
					t.Errorf("expected 2 congregation options, got %d", len(body.Congregation.Options))
				}

				// testmapbeta001a has 5 addresses in seed data.
				if len(body.Addresses) != 5 {
					t.Errorf("expected 5 addresses, got %d", len(body.Addresses))
				}

				if body.HasPinnedMessages {
					t.Error("expected has_pinned_messages = false for testmapbeta001a")
				}
			},
		},

		// ── Address options included in response ──────────────────────────────

		{
			Name:   "addresses include associated address_options",
			Method: http.MethodPost,
			URL:    "/link/map",
			Headers: map[string]string{
				"Content-Type": "application/json",
				"link-id":      "testassignalpha01",
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"aoId":"testaoalph01001"`, `"aoId":"testaoalph01002"`},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				var body struct {
					Addresses []struct {
						Id      string `json:"id"`
						Options []struct {
							AoId string `json:"aoId"`
							Id   string `json:"id"`
						} `json:"options"`
					} `json:"addresses"`
				}
				if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
					t.Fatalf("failed to decode response body: %v", err)
				}

				// Seed: testalpha01a003 and testalpha01a004 each have one address_option.
				optionCounts := map[string]int{}
				for _, addr := range body.Addresses {
					optionCounts[addr.Id] = len(addr.Options)
				}
				if optionCounts["testalpha01a003"] != 1 {
					t.Errorf("expected testalpha01a003 to have 1 option, got %d", optionCounts["testalpha01a003"])
				}
				if optionCounts["testalpha01a004"] != 1 {
					t.Errorf("expected testalpha01a004 to have 1 option, got %d", optionCounts["testalpha01a004"])
				}
				// Addresses without options should have an empty array, not null.
				if optionCounts["testalpha01a001"] != 0 {
					t.Errorf("expected testalpha01a001 to have 0 options, got %d", optionCounts["testalpha01a001"])
				}
			},
		},

		// ── Map field permutations ────────────────────────────────────────────

		{
			// JSON description passes through as a raw JSON object (not re-encoded as string).
			// coordinates and aggregates are present and non-null.
			// progress is non-zero.
			// Addresses carry rich fields: not_home_tries, notes, coordinates, dnc_time, updated_by.
			// has_pinned_messages is false — no messages exist for this map.
			Name:   "rich map: JSON description, coordinates, aggregates, rich address fields",
			Method: http.MethodPost,
			URL:    "/link/map",
			Headers: map[string]string{
				"Content-Type": "application/json",
				"link-id":      "testassignrich1",
			},
			TestAppFactory: setupTestApp,
			ExpectedStatus: 200,
			ExpectedContent: []string{
				`"publisher":"Test Publisher Rich"`,
				`"progress":40`,
				`"type":"multi"`,
				`"has_pinned_messages":false`,
				`"notes":"Speaks Mandarin"`,
				`"aoId":"testaorichaddr1"`,
			},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				var body struct {
					Map struct {
						Id          string          `json:"id"`
						Type        string          `json:"type"`
						Progress    int             `json:"progress"`
						Description json.RawMessage `json:"description"`
						Coordinates json.RawMessage `json:"coordinates"`
						Aggregates  json.RawMessage `json:"aggregates"`
					} `json:"map"`
					Addresses []struct {
						Id           string          `json:"id"`
						Notes        string          `json:"notes"`
						NotHomeTries int             `json:"not_home_tries"`
						DncTime      string          `json:"dnc_time"`
						UpdatedBy    string          `json:"updated_by"`
						Coordinates  json.RawMessage `json:"coordinates"`
						Options      []struct {
							AoId string `json:"aoId"`
						} `json:"options"`
					} `json:"addresses"`
					HasPinnedMessages bool `json:"has_pinned_messages"`
				}
				if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
					t.Fatalf("failed to decode response body: %v", err)
				}

				if body.Map.Id != "testmapalphrich1" {
					t.Errorf("expected map.id = testmapalphrich1, got %s", body.Map.Id)
				}
				if body.Map.Type != "multi" {
					t.Errorf("expected map.type = multi, got %s", body.Map.Type)
				}
				if body.Map.Progress != 40 {
					t.Errorf("expected map.progress = 40, got %d", body.Map.Progress)
				}

				// description must be a JSON object with en/zh keys
				var desc map[string]string
				if err := json.Unmarshal(body.Map.Description, &desc); err != nil {
					t.Fatalf("description is not a JSON object: %v (raw: %s)", err, body.Map.Description)
				}
				if desc["en"] != "Rich Map" {
					t.Errorf("expected description.en = Rich Map, got %s", desc["en"])
				}
				if desc["zh"] != "富地图" {
					t.Errorf("expected description.zh = 富地图, got %s", desc["zh"])
				}

				// coordinates must be a JSON object with lat/lng
				var coords map[string]float64
				if err := json.Unmarshal(body.Map.Coordinates, &coords); err != nil {
					t.Fatalf("map.coordinates is not a JSON object: %v", err)
				}
				if coords["lat"] == 0 || coords["lng"] == 0 {
					t.Errorf("expected non-zero map.coordinates, got %v", coords)
				}

				// aggregates must be a JSON object with notDone/notHome keys
				var agg map[string]any
				if err := json.Unmarshal(body.Map.Aggregates, &agg); err != nil {
					t.Fatalf("map.aggregates is not a JSON object: %v", err)
				}
				if _, ok := agg["notDone"]; !ok {
					t.Error("expected map.aggregates to contain notDone key")
				}
				if _, ok := agg["notHome"]; !ok {
					t.Error("expected map.aggregates to contain notHome key")
				}

				if len(body.Addresses) != 2 {
					t.Fatalf("expected 2 addresses for testmapalphrich1, got %d", len(body.Addresses))
				}

				seen := map[string]bool{}
				for _, a := range body.Addresses {
					seen[a.Id] = true
					switch a.Id {
					case "testalpharich01":
						if a.Notes != "Speaks Mandarin" {
							t.Errorf("testalpharich01.notes: want Speaks Mandarin, got %s", a.Notes)
						}
						if a.NotHomeTries != 2 {
							t.Errorf("testalpharich01.not_home_tries: want 2, got %d", a.NotHomeTries)
						}
						var addrCoords map[string]float64
						if err := json.Unmarshal(a.Coordinates, &addrCoords); err != nil {
							t.Fatalf("testalpharich01.coordinates is not a JSON object: %v", err)
						}
						if addrCoords["lat"] == 0 {
							t.Error("expected testalpharich01.coordinates.lat to be non-zero")
						}
						if len(a.Options) != 1 || a.Options[0].AoId != "testaorichaddr1" {
							t.Errorf("testalpharich01 options: want [testaorichaddr1], got %+v", a.Options)
						}
					case "testalpharich02":
						if a.DncTime == "" {
							t.Error("expected testalpharich02.dnc_time to be set")
						}
						if a.UpdatedBy != "testuseralpha01" {
							t.Errorf("testalpharich02.updated_by: want testuseralpha01, got %s", a.UpdatedBy)
						}
						if len(a.Options) != 0 {
							t.Errorf("testalpharich02 options: want 0, got %d", len(a.Options))
						}
					}
				}
				for _, id := range []string{"testalpharich01", "testalpharich02"} {
					if !seen[id] {
						t.Errorf("address %s missing from response", id)
					}
				}

				if body.HasPinnedMessages {
					t.Error("expected has_pinned_messages = false for testmapalphrich1")
				}
			},
		},
		{
			// Empty description serialises as null (not "").
			// Missing coordinates and aggregates serialise as null.
			// No addresses — empty array, not null.
			// Unpinned admin message present — has_pinned_messages must still be false.
			Name:   "empty map: null description/coordinates/aggregates, empty addresses, unpinned admin msg",
			Method: http.MethodPost,
			URL:    "/link/map",
			Headers: map[string]string{
				"Content-Type": "application/json",
				"link-id":      "testassignmpty1",
			},
			TestAppFactory: setupTestApp,
			ExpectedStatus: 200,
			ExpectedContent: []string{
				`"publisher":"Test Publisher Empty"`,
				`"description":null`,
				`"coordinates":null`,
				`"aggregates":null`,
				`"addresses":[]`,
				`"has_pinned_messages":false`,
			},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				var body struct {
					Map struct {
						Id          string          `json:"id"`
						Description json.RawMessage `json:"description"`
						Coordinates json.RawMessage `json:"coordinates"`
						Aggregates  json.RawMessage `json:"aggregates"`
						Progress    int             `json:"progress"`
					} `json:"map"`
					Addresses []struct {
						Id string `json:"id"`
					} `json:"addresses"`
					HasPinnedMessages bool `json:"has_pinned_messages"`
				}
				if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
					t.Fatalf("failed to decode response body: %v", err)
				}

				if body.Map.Id != "testmapalphempt1" {
					t.Errorf("expected map.id = testmapalphempt1, got %s", body.Map.Id)
				}
				if string(body.Map.Description) != "null" {
					t.Errorf("expected description = null, got %s", body.Map.Description)
				}
				if string(body.Map.Coordinates) != "null" {
					t.Errorf("expected coordinates = null, got %s", body.Map.Coordinates)
				}
				if string(body.Map.Aggregates) != "null" {
					t.Errorf("expected aggregates = null, got %s", body.Map.Aggregates)
				}
				if body.Map.Progress != 0 {
					t.Errorf("expected progress = 0, got %d", body.Map.Progress)
				}
				if len(body.Addresses) != 0 {
					t.Errorf("expected empty addresses array, got %d addresses", len(body.Addresses))
				}
				// Unpinned admin message must NOT set has_pinned_messages.
				if body.HasPinnedMessages {
					t.Error("expected has_pinned_messages = false when only unpinned admin message exists")
				}
			},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}
