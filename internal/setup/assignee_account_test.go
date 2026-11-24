//go:build testdata

package setup

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"ganigeorgiev.com/pocketbase/pocketbase/tests"
)

const readOnlyUserID = "testuseralpha03"

func TestAssignees_MustBeAbleToSignIn(t *testing.T) {
	conductorToken, err := generateToken("conductor@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	headers := map[string]string{"Authorization": conductorToken, "Content-Type": "application/json"}
	set := func(userID, field string, value bool) func(testing.TB, *tests.TestApp, *core.ServeEvent) {
		return func(t testing.TB, app *tests.TestApp, e *core.ServeEvent) {
			seedPublisherLinks(t, app, e)
			user, err := app.FindRecordById("users", userID)
			if err != nil {
				t.Fatal(err)
			}
			user.Set(field, value)
			if err := app.SaveNoValidate(user); err != nil {
				t.Fatal(err)
			}
		}
	}
	create := func(assignees ...string) *strings.Reader {
		raw, _ := json.Marshal(map[string]any{
			"map": "testmapalpha01a", "congregation": "testcongalpha01", "user": "testuseralpha02",
			"assignees": assignees, "type": "normal", "publisher": "Alpha ReadOnly",
			"expiry_date": "2099-01-01 00:00:00.000Z",
		})
		return strings.NewReader(string(raw))
	}
	change := func(assignees ...string) *strings.Reader {
		raw, _ := json.Marshal(map[string]any{
			"assignment": publisherLinkID, "assignees": assignees, "publisher": "Alpha ReadOnly",
		})
		return strings.NewReader(string(raw))
	}
	refused := []string{`"An assignee is not a member of this congregation."`}

	scenarios := []tests.ApiScenario{
		{
			Name:            "a new link is not given to a disabled account",
			Method:          http.MethodPost,
			URL:             "/api/collections/assignments/records",
			Body:            create(readOnlyUserID),
			Headers:         headers,
			TestAppFactory:  setupTestApp,
			BeforeTestFunc:  set(readOnlyUserID, "disabled", true),
			ExpectedStatus:  400,
			ExpectedContent: refused,
		},
		{
			Name:            "a new link is not given to an unverified account",
			Method:          http.MethodPost,
			URL:             "/api/collections/assignments/records",
			Body:            create(readOnlyUserID),
			Headers:         headers,
			TestAppFactory:  setupTestApp,
			BeforeTestFunc:  set(readOnlyUserID, "verified", false),
			ExpectedStatus:  400,
			ExpectedContent: refused,
		},
		{
			Name:            "a disabled account is not added to a link",
			Method:          http.MethodPost,
			URL:             "/assignment/assignees",
			Body:            change(publisherUserID, readOnlyUserID),
			Headers:         headers,
			TestAppFactory:  setupTestApp,
			BeforeTestFunc:  set(readOnlyUserID, "disabled", true),
			ExpectedStatus:  400,
			ExpectedContent: refused,
		},
		{
			Name:            "an unverified account is not added to a link",
			Method:          http.MethodPost,
			URL:             "/assignment/assignees",
			Body:            change(publisherUserID, readOnlyUserID),
			Headers:         headers,
			TestAppFactory:  setupTestApp,
			BeforeTestFunc:  set(readOnlyUserID, "verified", false),
			ExpectedStatus:  400,
			ExpectedContent: refused,
		},
		{
			Name:            "someone on a link stays on it after their account is disabled",
			Method:          http.MethodPost,
			URL:             "/assignment/assignees",
			Body:            change(publisherUserID, readOnlyUserID),
			Headers:         headers,
			TestAppFactory:  setupTestApp,
			BeforeTestFunc:  set(publisherUserID, "disabled", true),
			ExpectedStatus:  200,
			ExpectedContent: []string{`"assignees":["testuseralpha04","testuseralpha03"]`},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, _ *http.Response) {
				link, err := app.FindRecordById("assignments", publisherLinkID)
				if err != nil {
					t.Fatal(err)
				}
				if got := link.GetStringSlice("assignees"); len(got) != 2 {
					t.Errorf("assignees = %v, want both accounts", got)
				}
			},
		},
	}
	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}
