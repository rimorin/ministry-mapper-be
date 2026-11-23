//go:build testdata

package setup

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"testing"

	"ganigeorgiev.com/pocketbase/dbx"
	"ganigeorgiev.com/pocketbase/pocketbase/tests"
)

// A list's total and its pages must speak only of rows the caller may see.
// The hooks used to drop hidden rows after the query had counted them, so a
// filter widened with "|| id != ”" got a total of everything it matched,
// and a count is enough to spell out a hidden value one question at a time.
func TestListTotalsCountOnlyVisibleRows(t *testing.T) {
	conductorToken, err := generateToken("conductor@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	publisherToken, err := generateToken(publisherEmail)
	if err != nil {
		t.Fatal(err)
	}

	// The total a response reports, against a count taken from the database.
	totalIs := func(countSQL string, params dbx.Params) func(testing.TB, *tests.TestApp, *http.Response) {
		return func(t testing.TB, app *tests.TestApp, res *http.Response) {
			body, err := io.ReadAll(res.Body)
			if err != nil {
				t.Fatal(err)
			}
			var page struct {
				TotalItems int `json:"totalItems"`
			}
			if err := json.Unmarshal(body, &page); err != nil {
				t.Fatalf("%v: %s", err, body)
			}
			var want int
			if err := app.DB().NewQuery(countSQL).Bind(params).Row(&want); err != nil {
				t.Fatal(err)
			}
			if page.TotalItems != want {
				t.Errorf("totalItems = %d, want %d: the total counts rows the caller may not see", page.TotalItems, want)
			}
		}
	}
	// A page far past the end, where no row is returned and only the total speaks.
	list := func(collection, filter string) string {
		return "/api/collections/" + collection + "/records?fields=id&perPage=1&page=500&filter=" + url.QueryEscape(filter)
	}
	collectionID := func(t testing.TB, name string) string {
		app := setupTestApp(t)
		defer app.Cleanup()
		collection, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			t.Fatal(err)
		}
		return collection.Id
	}

	scenarios := []tests.ApiScenario{
		{
			Name:            "a conductor's widened assignments filter counts their congregation's links only",
			Method:          http.MethodGet,
			URL:             list("assignments", `user="testuseralpha02" || id != ""`),
			Headers:         map[string]string{"Authorization": conductorToken},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"totalItems"`},
			AfterTestFunc: totalIs(`SELECT COUNT(*) FROM assignments
				WHERE congregation = 'testcongalpha01' AND expiry_date > datetime('now')`, nil),
		},
		{
			// The same, with the collection named by its id.
			Name:            "naming the collection by its id changes nothing",
			Method:          http.MethodGet,
			URL:             list(collectionID(t, "assignments"), `user="testuseralpha02" || id != ""`),
			Headers:         map[string]string{"Authorization": conductorToken},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"totalItems"`},
			AfterTestFunc: totalIs(`SELECT COUNT(*) FROM assignments
				WHERE congregation = 'testcongalpha01' AND expiry_date > datetime('now')`, nil),
		},
		{
			// An account with no staff role, as anyone gets by signing up.
			Name:            "an account with no staff role counts nothing but its own",
			Method:          http.MethodGet,
			URL:             list("assignments", `user="`+publisherUserID+`" || id != ""`),
			Headers:         map[string]string{"Authorization": publisherToken},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"totalItems":0`},
		},
		{
			Name:            "a widened roles filter counts the caller's congregations only",
			Method:          http.MethodGet,
			URL:             list("roles", `user="testuseralpha02" || id != ""`),
			Headers:         map[string]string{"Authorization": conductorToken},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"totalItems"`},
			AfterTestFunc: totalIs(`SELECT COUNT(*) FROM roles
				WHERE congregation = 'testcongalpha01' OR user = 'testuseralpha02'`, nil),
		},
		{
			Name:            "a widened maps filter counts the caller's congregations only",
			Method:          http.MethodGet,
			URL:             list("maps", `congregation="testcongalpha01" || id != ""`),
			Headers:         map[string]string{"Authorization": conductorToken},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"totalItems"`},
			AfterTestFunc:   totalIs(`SELECT COUNT(*) FROM maps WHERE congregation = 'testcongalpha01'`, nil),
		},
		{
			Name:            "a share link's widened addresses filter counts its one map only",
			Method:          http.MethodGet,
			URL:             list("addresses", `map="testmapalpha01a" || id != ""`),
			Headers:         map[string]string{"link-id": "testassignalpha01"},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"totalItems"`},
			AfterTestFunc:   totalIs(`SELECT COUNT(*) FROM addresses WHERE map = 'testmapalpha01a'`, nil),
		},
		{
			Name:            "a share link's widened messages filter counts its one map only",
			Method:          http.MethodGet,
			URL:             list("messages", `map="testmapalpha01a" || id != ""`),
			Headers:         map[string]string{"link-id": "testassignalpha01"},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"totalItems"`},
			AfterTestFunc:   totalIs(`SELECT COUNT(*) FROM messages WHERE map = 'testmapalpha01a'`, nil),
		},
	}
	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}
