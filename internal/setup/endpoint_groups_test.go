//go:build testdata

package setup

import (
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"ganigeorgiev.com/pocketbase/pocketbase/tests"
)

const seedGroupID = "testgroupalpha1"

func TestGroups(t *testing.T) {
	token := func(email string) string {
		t.Helper()
		token, err := generateToken(email)
		if err != nil {
			t.Fatal(err)
		}
		return token
	}
	admin := token("admin@alpha.test")
	conductor := token("conductor@alpha.test")
	otherAdmin := token("admin@beta.test")

	as := func(token string) map[string]string {
		return map[string]string{"Authorization": token, "Content-Type": "application/json"}
	}
	records := "/api/collections/groups/records"
	group := records + "/" + seedGroupID
	list := records + "?fields=id,name&filter=" + url.QueryEscape(`congregation="testcongalpha01"`)
	create := func(name string, members ...string) *strings.Reader {
		return strings.NewReader(`{"congregation":"testcongalpha01","name":"` + name +
			`","members":["` + strings.Join(members, `","`) + `"]}`)
	}

	scenarios := []tests.ApiScenario{
		{
			Name:            "an administrator makes a group of members",
			Method:          http.MethodPost,
			URL:             records,
			Body:            create("Mandarin", "testuseralpha03", "testuseralpha04"),
			Headers:         as(admin),
			ExpectedStatus:  200,
			ExpectedContent: []string{`"name":"Mandarin"`},
		},
		{
			Name:            "a conductor cannot make a group",
			Method:          http.MethodPost,
			URL:             records,
			Body:            create("Mandarin", "testuseralpha04"),
			Headers:         as(conductor),
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Administrator access required."`},
		},
		{
			Name:            "another congregation's administrator cannot make one here",
			Method:          http.MethodPost,
			URL:             records,
			Body:            create("Mandarin", "testuseralpha04"),
			Headers:         as(otherAdmin),
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Administrator access required."`},
		},
		{
			Name:            "someone from another congregation cannot be added",
			Method:          http.MethodPost,
			URL:             records,
			Body:            create("Mandarin", "testuseralpha04", "testuserbeta001"),
			Headers:         as(admin),
			ExpectedStatus:  400,
			ExpectedContent: []string{`"A member is not in this congregation."`},
		},
		{
			Name:            "two groups cannot share a name",
			Method:          http.MethodPost,
			URL:             records,
			Body:            create("Group 1", "testuseralpha03"),
			Headers:         as(admin),
			ExpectedStatus:  400,
			ExpectedContent: []string{`"name"`},
		},
		{
			Name:            "an administrator renames a group and adds to it",
			Method:          http.MethodPatch,
			URL:             group,
			Body:            strings.NewReader(`{"name":"Group 3","members+":"testuseralpha03"}`),
			Headers:         as(admin),
			ExpectedStatus:  200,
			ExpectedContent: []string{`"name":"Group 3"`, `"testuseralpha03"`},
		},
		{
			Name:            "a group stays in its congregation",
			Method:          http.MethodPatch,
			URL:             group,
			Body:            strings.NewReader(`{"congregation":"testcongbeta001"}`),
			Headers:         as(admin),
			ExpectedStatus:  400,
			ExpectedContent: []string{`"A group cannot be moved to another congregation."`},
		},
		{
			Name:            "another congregation's administrator cannot change it",
			Method:          http.MethodPatch,
			URL:             group,
			Body:            strings.NewReader(`{"name":"Taken"}`),
			Headers:         as(otherAdmin),
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Administrator access required."`},
		},
		{
			Name:            "a conductor lists the congregation's groups",
			Method:          http.MethodGet,
			URL:             list,
			Headers:         as(conductor),
			ExpectedStatus:  200,
			ExpectedContent: []string{`"name":"Group 1"`},
		},
		{
			Name:               "another congregation cannot list them",
			Method:             http.MethodGet,
			URL:                list,
			Headers:            as(otherAdmin),
			ExpectedStatus:     403,
			ExpectedContent:    []string{`"Unauthorized."`},
			NotExpectedContent: []string{`"Group 1"`},
		},
		{
			Name:               "a list must name its congregation",
			Method:             http.MethodGet,
			URL:                records + "?fields=id,name",
			Headers:            as(conductor),
			ExpectedStatus:     403,
			ExpectedContent:    []string{`"Unauthorized."`},
			NotExpectedContent: []string{`"Group 1"`},
		},
		{
			Name:            "a conductor cannot delete a group",
			Method:          http.MethodDelete,
			URL:             group,
			Headers:         as(conductor),
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Administrator access required."`},
		},
		{
			Name:           "an administrator deletes a group",
			Method:         http.MethodDelete,
			URL:            group,
			Headers:        as(admin),
			ExpectedStatus: 204,
		},
		{
			Name:           "someone who leaves the congregation leaves its groups",
			Method:         http.MethodDelete,
			URL:            "/api/collections/roles/records/testrolexcng01d",
			Headers:        as(admin),
			ExpectedStatus: 204,
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, _ *http.Response) {
				group, err := app.FindRecordById("groups", seedGroupID)
				if err != nil {
					t.Fatal(err)
				}
				if members := group.GetStringSlice("members"); !slices.Equal(members, []string{"testuseralpha02"}) {
					t.Errorf("members = %v, want only the conductor", members)
				}
			},
		},
	}

	for _, scenario := range scenarios {
		scenario.TestAppFactory = setupTestApp
		scenario.Test(t)
	}
}
