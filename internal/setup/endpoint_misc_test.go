package setup

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/tests"
)

func TestHandleOptionUpdate(t *testing.T) {
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

	validPayload := `{
		"congregation":"testcongalpha01",
		"options":[
			{"id":"testoptialpha01","code":"NH","description":"Not Home","sequence":1,"is_default":false,"is_countable":true},
			{"id":"testoptialpha02","code":"DNC","description":"Do Not Call","sequence":2,"is_default":false,"is_countable":false},
			{"id":"testoptialpha03","code":"LN","description":"Language Note Updated","sequence":3,"is_default":true,"is_countable":true}
		]
	}`

	noDefaultPayload := `{
		"congregation":"testcongalpha01",
		"options":[
			{"id":"testoptialpha01","code":"NH","description":"Not Home","sequence":1,"is_default":false,"is_countable":true},
			{"id":"testoptialpha02","code":"DNC","description":"Do Not Call","sequence":2,"is_default":false,"is_countable":false},
			{"id":"testoptialpha03","code":"LN","description":"Language Note","sequence":3,"is_default":false,"is_countable":true}
		]
	}`

	scenarios := []tests.ApiScenario{
		{
			Name:   "no auth is rejected with 401",
			Method: http.MethodPost,
			URL:    "/options/update",
			Body:   strings.NewReader(validPayload),
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  401,
			ExpectedContent: []string{`"status":401`},
		},
		{
			Name:   "conductor cannot update options (403)",
			Method: http.MethodPost,
			URL:    "/options/update",
			Body:   strings.NewReader(validPayload),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Administrator access required."`},
		},
		{
			Name:   "admin from different congregation cannot update options (403)",
			Method: http.MethodPost,
			URL:    "/options/update",
			Body:   strings.NewReader(validPayload),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": betaAdminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Administrator access required."`},
		},
		{
			Name:   "no default option returns 400",
			Method: http.MethodPost,
			URL:    "/options/update",
			Body:   strings.NewReader(noDefaultPayload),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  400,
			ExpectedContent: []string{`"Exactly one option must be marked as default."`},
		},
		{
			Name:   "valid options update returns 200",
			Method: http.MethodPost,
			URL:    "/options/update",
			Body:   strings.NewReader(validPayload),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"Options processed successfully"`},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestHandleGenerateReport(t *testing.T) {
	adminToken, err := generateToken("admin@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	conductorToken, err := generateToken("conductor@alpha.test")
	if err != nil {
		t.Fatal(err)
	}

	scenarios := []tests.ApiScenario{
		{
			Name:   "no auth is rejected with 401",
			Method: http.MethodPost,
			URL:    "/report/generate",
			Body:   strings.NewReader(`{"congregation":"testcongalpha01"}`),
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  401,
			ExpectedContent: []string{`"status":401`},
		},
		{
			Name:   "conductor is not admin so report returns 403",
			Method: http.MethodPost,
			URL:    "/report/generate",
			Body:   strings.NewReader(`{"congregation":"testcongalpha01"}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Not an administrator for this congregation."`},
		},
		{
			Name:   "admin can trigger report generation (202)",
			Method: http.MethodPost,
			URL:    "/report/generate",
			Body:   strings.NewReader(`{"congregation":"testcongalpha01"}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  202,
			ExpectedContent: []string{`"Report generation started. You will receive an email shortly."`},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestHandleDBHealth(t *testing.T) {
	scenarios := []tests.ApiScenario{
		{
			Name:            "health check returns 200 without auth",
			Method:          http.MethodGet,
			URL:             "/api/db-health",
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"code":200`, `"Database is healthy."`, `"data":{}`},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}
