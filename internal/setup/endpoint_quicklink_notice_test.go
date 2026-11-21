//go:build testdata

package setup

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"ganigeorgiev.com/pocketbase/dbx"
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"ganigeorgiev.com/pocketbase/pocketbase/tests"
	"ganigeorgiev.com/pocketbase/pocketbase/tools/types"
)

// A quick link tells the conductor who on it still holds an open link in the
// territory, and refuses only when the territory has nothing left to work.
func TestQuicklink_SaysWhoAlreadyHoldsALinkAndStopsWhenFinished(t *testing.T) {
	conductorToken, err := generateToken("conductor@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	headers := map[string]string{"Authorization": conductorToken, "Content-Type": "application/json"}

	// An open (or expired) normal link on Blk 100B, made out as given.
	withLink := func(publisher, assignees string, expiresIn time.Duration) func(testing.TB, *tests.TestApp, *core.ServeEvent) {
		return func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
			expiry, _ := types.ParseDateTime(time.Now().Add(expiresIn))
			_, err := app.DB().NewQuery(`INSERT INTO assignments
				(id, congregation, map, user, publisher, type, assignees, expiry_date, created, updated)
				VALUES ('testassignheld1', 'testcongalpha01', 'testmapalpha01b', 'testuseralpha02', {:publisher},
				        'normal', {:assignees}, {:expiry}, {:now}, {:now})`).
				Bind(dbx.Params{"publisher": publisher, "assignees": assignees, "expiry": expiry.String(),
					"now": types.NowDateTime().String()}).Execute()
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	ask := func(body string) *strings.Reader { return strings.NewReader(body) }

	scenarios := []tests.ApiScenario{
		{
			Name:   "a name already on an open link is reported, however it was typed",
			Method: http.MethodPost, URL: "/territory/link", Headers: headers,
			Body:            ask(`{"territory":"testterralpha01","publisher":"sis  ONG"}`),
			TestAppFactory:  setupTestApp,
			BeforeTestFunc:  withLink("Sis Ong , Bro Tan", "", time.Hour),
			ExpectedStatus:  200,
			ExpectedContent: []string{`"linkId"`, `"already":[{"publisher":"Sis Ong , Bro Tan","map":"Blk 100B"}]`},
		},
		{
			Name:   "an account already on an open link is reported under any name",
			Method: http.MethodPost, URL: "/territory/link", Headers: headers,
			Body:            ask(`{"territory":"testterralpha01","publisher":"Alpha Publisher","assignees":["` + publisherUserID + `"]}`),
			TestAppFactory:  setupTestApp,
			BeforeTestFunc:  withLink("Bro P", `["`+publisherUserID+`"]`, time.Hour),
			ExpectedStatus:  200,
			ExpectedContent: []string{`"linkId"`, `"already":[{"publisher":"Bro P","map":"Blk 100B"}]`},
		},
		{
			Name:   "someone else's link, and one that has expired, are not reported",
			Method: http.MethodPost, URL: "/territory/link", Headers: headers,
			Body:            ask(`{"territory":"testterralpha01","publisher":"Sis Ong"}`),
			TestAppFactory:  setupTestApp,
			BeforeTestFunc:  withLink("Sis Ong", "", -time.Hour),
			ExpectedStatus:  200,
			ExpectedContent: []string{`"linkId"`, `"already":[]`},
		},
		{
			Name:   "a territory with no home left gives no link",
			Method: http.MethodPost, URL: "/territory/link", Headers: headers,
			Body:           ask(`{"territory":"testterralpha01","publisher":"Sis Ong"}`),
			TestAppFactory: setupTestApp,
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				if _, err := app.DB().NewQuery(`UPDATE maps SET progress = 100, aggregates = '{"total":5,"completed":5}'
					WHERE territory = 'testterralpha01'`).Execute(); err != nil {
					t.Fatal(err)
				}
			},
			ExpectedStatus:  http.StatusConflict,
			ExpectedContent: []string{`"Every map in this territory is finished."`},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, _ *http.Response) {
				if _, err := app.FindFirstRecordByFilter("assignments", `publisher = "Sis Ong"`); err == nil {
					t.Error("a refused quick link still made a link")
				}
			},
		},
		{
			Name:   "maps past the threshold that still have homes are given",
			Method: http.MethodPost, URL: "/territory/link", Headers: headers,
			Body:           ask(`{"territory":"testterralpha01","publisher":"Sis Ong"}`),
			TestAppFactory: setupTestApp,
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				if _, err := app.DB().NewQuery(`UPDATE maps SET progress = 95, aggregates = '{"total":20,"completed":19}'
					WHERE territory = 'testterralpha01'`).Execute(); err != nil {
					t.Fatal(err)
				}
			},
			ExpectedStatus:  200,
			ExpectedContent: []string{`"linkId"`},
		},
	}
	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}
