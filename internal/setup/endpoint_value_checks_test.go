//go:build testdata

package setup

import (
	"net/http"
	"strings"
	"testing"

	"ganigeorgiev.com/pocketbase/pocketbase/tests"
)

func TestAddressRoutesCheckValues(t *testing.T) {
	// testassignalpha01 is linked to testmapalpha01a
	linkHeaders := map[string]string{
		"Content-Type": "application/json",
		"link-id":      "testassignalpha01",
	}
	update := func(fields string) *strings.Reader {
		return strings.NewReader(`{"address_id":"testalpha01a001","map_id":"testmapalpha01a",` + fields + `}`)
	}
	add := func(fields string) *strings.Reader {
		return strings.NewReader(`{"map_id":"testmapalpha01a","code":"97",` + fields + `}`)
	}
	unchanged := func(t testing.TB, app *tests.TestApp, _ *http.Response) {
		addr, err := app.FindRecordById("addresses", "testalpha01a001")
		if err != nil {
			t.Fatal(err)
		}
		if addr.GetString("status") != "not_done" || addr.GetString("notes") != "" {
			t.Errorf("home was changed: status=%q notes=%q", addr.GetString("status"), addr.GetString("notes"))
		}
		foreign, err := app.FindRecordsByFilter("address_options", `option = "testoptibeta001" || option = "nosuchoption001"`, "", 1, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(foreign) != 0 {
			t.Errorf("an option outside the congregation was stored on a home")
		}
	}

	refusals := []struct {
		name, url string
		body      *strings.Reader
		want      string
	}{
		{"update: unknown status", "/address/update", update(`"status":"finished"`), "Invalid status"},
		{"update: missing status", "/address/update", update(`"notes":"x"`), "Invalid status"},
		{"update: oversized notes", "/address/update", update(`"status":"done","notes":"` + strings.Repeat("a", 5001) + `"`), "The note is too long"},
		{"update: negative tries", "/address/update", update(`"status":"not_home","not_home_tries":-1`), "Invalid not_home_tries"},
		{"update: huge tries", "/address/update", update(`"status":"not_home","not_home_tries":100000`), "Invalid not_home_tries"},
		{"update: dnc_time not a date", "/address/update", update(`"status":"do_not_call","dnc_time":"soon"`), "Invalid dnc_time"},
		{"update: coordinates not a point", "/address/update", update(`"status":"done","coordinates":{"lat":"x"}`), "Invalid coordinates"},
		{"update: coordinates out of range", "/address/update", update(`"status":"done","coordinates":{"lat":91,"lng":0}`), "Invalid coordinates"},
		{"update: coordinates an arbitrary document", "/address/update", update(`"status":"done","coordinates":[1,2,3]`), "Invalid coordinates"},
		{"update: another congregation's option", "/address/update", update(`"status":"done","add_option_ids":["testoptibeta001"]`), "Invalid option"},
		{"update: made-up option", "/address/update", update(`"status":"done","add_option_ids":["nosuchoption001"]`), "Invalid option"},
		{"add: unknown status", "/address/add", add(`"status":"finished"`), "Invalid status"},
		{"add: floor out of range", "/address/add", add(`"floor":100000`), "Invalid floor"},
		{"add: address_id not an id", "/address/add", add(`"address_id":"../../etc"`), "Invalid address_id"},
		{"add: another congregation's option", "/address/add", add(`"add_option_ids":["testoptibeta001"]`), "Invalid option"},
	}

	scenarios := make([]tests.ApiScenario, 0, len(refusals)+2)
	for _, r := range refusals {
		scenarios = append(scenarios, tests.ApiScenario{
			Name:            r.name,
			Method:          http.MethodPost,
			URL:             r.url,
			Body:            r.body,
			Headers:         linkHeaders,
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  422,
			ExpectedContent: []string{r.want},
			AfterTestFunc:   unchanged,
		})
	}

	scenarios = append(scenarios,
		tests.ApiScenario{
			Name:   "update: a full, valid change is saved",
			Method: http.MethodPost,
			URL:    "/address/update",
			Body: update(`"status":"do_not_call","notes":"Call back","not_home_tries":2,` +
				`"dnc_time":"2026-11-23T04:00:00.000Z","coordinates":{"lat":1.37,"lng":103.84},` +
				`"add_option_ids":["testoptialpha03"]`),
			Headers:        linkHeaders,
			TestAppFactory: setupTestApp,
			ExpectedStatus: http.StatusNoContent,
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, _ *http.Response) {
				addr, err := app.FindRecordById("addresses", "testalpha01a001")
				if err != nil {
					t.Fatal(err)
				}
				if addr.GetString("status") != "do_not_call" || addr.GetInt("not_home_tries") != 2 || addr.GetDateTime("dnc_time").IsZero() {
					t.Errorf("change not saved: %v", addr.PublicExport())
				}
			},
		},
		tests.ApiScenario{
			Name:            "add: a valid home with the app's own id is created",
			Method:          http.MethodPost,
			URL:             "/address/add",
			Body:            add(`"address_id":"abcde12345fghij","floor":1,"status":"not_done","coordinates":null`),
			Headers:         linkHeaders,
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  201,
			ExpectedContent: []string{`"abcde12345fghij"`},
		},
	)

	for _, s := range scenarios {
		s.Test(t)
	}
}

func TestNewMapRefusesForeignTerritory(t *testing.T) {
	adminToken, err := generateToken("admin@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	body := func(territory string) *strings.Reader {
		return strings.NewReader(`{"territory":"` + territory + `","congregation":"testcongalpha01","type":"single","floors":1,"name":"Blk 889 Test","coordinates":"1.3714,103.8494","sequence":"01,02"}`)
	}
	headers := map[string]string{"Content-Type": "application/json", "Authorization": adminToken}
	noNewMap := func(t testing.TB, app *tests.TestApp, _ *http.Response) {
		if maps, _ := app.FindRecordsByFilter("maps", `description = "Blk 889 Test"`, "", 1, 0); len(maps) != 0 {
			t.Errorf("a map was created under %q", maps[0].GetString("territory"))
		}
	}

	scenarios := []tests.ApiScenario{
		{
			Name:            "another congregation's territory is refused",
			Method:          http.MethodPost,
			URL:             "/map/add",
			Body:            body("testterrbeta001"),
			Headers:         headers,
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  400,
			ExpectedContent: []string{"Invalid territory"},
			AfterTestFunc:   noNewMap,
		},
		{
			Name:            "a territory that does not exist is refused the same way",
			Method:          http.MethodPost,
			URL:             "/map/add",
			Body:            body("nosuchterritory"),
			Headers:         headers,
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  400,
			ExpectedContent: []string{"Invalid territory"},
			AfterTestFunc:   noNewMap,
		},
	}
	for _, s := range scenarios {
		s.Test(t)
	}
}
