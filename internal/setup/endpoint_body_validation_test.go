//go:build testdata

package setup

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/tests"
)

// Every case below used to reach an unchecked type assertion on the request
// body and panic, which the Sentry middleware turned into a 500 (and a
// LevelFatal event). They must be rejected as 400 client errors instead.
func TestEndpointBodyValidation(t *testing.T) {
	adminToken, err := generateToken("admin@alpha.test")
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name    string
		url     string
		body    string
		message string
	}{
		// /map/add
		{"map/add missing territory", "/map/add",
			`{"type":"single","floors":1,"name":"X","congregation":"testcongalpha01","coordinates":"","sequence":"1"}`,
			`"Territory is required."`},
		{"map/add missing congregation", "/map/add",
			`{"territory":"testterralpha01","type":"single","floors":1,"name":"X","coordinates":"","sequence":"1"}`,
			`"Congregation is required."`},
		{"map/add missing name", "/map/add",
			`{"territory":"testterralpha01","type":"single","floors":1,"congregation":"testcongalpha01","coordinates":"","sequence":"1"}`,
			`"Name is required."`},
		{"map/add empty body", "/map/add", `{}`, `"Territory is required."`},
		{"map/add wrong type for floors", "/map/add",
			`{"territory":"testterralpha01","type":"single","floors":"lots","name":"X","congregation":"testcongalpha01","sequence":"1"}`,
			`"Invalid request body."`},

		// /map/floor/add
		{"map/floor/add missing add_higher", "/map/floor/add",
			`{"map":"testmapalpha01b"}`, `"Add_higher is required."`},
		{"map/floor/add missing map", "/map/floor/add",
			`{"add_higher":true}`, `"Map is required."`},
		{"map/floor/add wrong type for add_higher", "/map/floor/add",
			`{"map":"testmapalpha01b","add_higher":"yes"}`, `"Invalid request body."`},

		// /map/floor/remove
		{"map/floor/remove missing floor", "/map/floor/remove",
			`{"map":"testmapalphcf01"}`, `"Floor is required."`},
		{"map/floor/remove missing map", "/map/floor/remove",
			`{"floor":2}`, `"Map is required."`},

		// /map/code/delete
		{"map/code/delete missing code", "/map/code/delete",
			`{"map":"testmapalpha01a"}`, `"Code is required."`},
		{"map/code/delete missing map", "/map/code/delete",
			`{"code":"10"}`, `"Map is required."`},

		// /map/code/add
		{"map/code/add missing map", "/map/code/add",
			`{"codes":["99"]}`, `"Map is required."`},

		// /map/reset
		{"map/reset missing map", "/map/reset", `{}`, `"Map is required."`},

		// /territory/reset
		{"territory/reset missing territory", "/territory/reset", `{}`, `"Territory is required."`},
	}

	for _, c := range cases {
		scenario := tests.ApiScenario{
			Name:   c.name,
			Method: http.MethodPost,
			URL:    c.url,
			Body:   strings.NewReader(c.body),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory: setupTestApp,
			ExpectedStatus: 400,
			ExpectedContent: []string{
				`"status":400`,
				c.message,
			},
		}
		scenario.Test(t)
	}
}
