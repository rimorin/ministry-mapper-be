package setup

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/tests"
)

func TestAuthHook_MapsListRequest(t *testing.T) {
	adminToken, err := generateToken("admin@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	conductorToken, err := generateToken("conductor@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	readonlyToken, err := generateToken("readonly@alpha.test")
	if err != nil {
		t.Fatal(err)
	}

	filteredURL := "/api/collections/maps/records?filter=" + url.QueryEscape(`congregation="testcongalpha01"`)
	territoryFilteredURL := "/api/collections/maps/records?filter=" + url.QueryEscape(`territory="testterralpha01"`)
	betaFilteredURL := "/api/collections/maps/records?filter=" + url.QueryEscape(`congregation="testcongbeta001"`)
	injectionURL := "/api/collections/maps/records?filter=" + url.QueryEscape(`congregation="testcongalpha01" || congregation="testcongbeta001"`)

	scenarios := []tests.ApiScenario{
		{
			Name:            "unauthenticated request is rejected with 403",
			Method:          http.MethodGet,
			URL:             filteredURL,
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
		{
			Name:   "request without congregation filter is rejected with 403",
			Method: http.MethodGet,
			URL:    "/api/collections/maps/records",
			Headers: map[string]string{
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Missing congregation or territory filter."`},
		},
		{
			Name:   "conductor with valid congregation filter gets map list",
			Method: http.MethodGet,
			URL:    filteredURL,
			Headers: map[string]string{
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"items"`},
		},
		{
			Name:   "admin with valid congregation filter gets map list",
			Method: http.MethodGet,
			URL:    filteredURL,
			Headers: map[string]string{
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"items"`},
		},
		{
			Name:   "conductor with territory filter gets map list",
			Method: http.MethodGet,
			URL:    territoryFilteredURL,
			Headers: map[string]string{
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"items"`},
		},
		{
			Name:   "conductor querying different congregation is rejected with 403",
			Method: http.MethodGet,
			URL:    betaFilteredURL,
			Headers: map[string]string{
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
		{
			Name:   "readonly user with valid congregation filter gets map list",
			Method: http.MethodGet,
			URL:    filteredURL,
			Headers: map[string]string{
				"Authorization": readonlyToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"items"`},
		},
		{
			Name:   "filter injection with two congregations is rejected with 403",
			Method: http.MethodGet,
			URL:    injectionURL,
			Headers: map[string]string{
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestAuthHook_TerritoriesListRequest(t *testing.T) {
	adminToken, err := generateToken("admin@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	conductorToken, err := generateToken("conductor@alpha.test")
	if err != nil {
		t.Fatal(err)
	}

	filteredURL := "/api/collections/territories/records?filter=" + url.QueryEscape(`congregation="testcongalpha01"`)
	injectionURL := "/api/collections/territories/records?filter=" + url.QueryEscape(`congregation="testcongalpha01" || congregation="testcongbeta001"`)

	scenarios := []tests.ApiScenario{
		{
			Name:            "unauthenticated request is rejected with 403",
			Method:          http.MethodGet,
			URL:             filteredURL,
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
		{
			Name:   "request without congregation filter is rejected with 403",
			Method: http.MethodGet,
			URL:    "/api/collections/territories/records",
			Headers: map[string]string{
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Missing congregation filter."`},
		},
		{
			Name:   "admin with congregation filter gets territory list",
			Method: http.MethodGet,
			URL:    filteredURL,
			Headers: map[string]string{
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"items"`},
		},
		{
			Name:   "conductor with valid congregation filter gets territory list",
			Method: http.MethodGet,
			URL:    filteredURL,
			Headers: map[string]string{
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"items"`},
		},
		{
			Name:   "filter injection with two congregations is rejected with 403",
			Method: http.MethodGet,
			URL:    injectionURL,
			Headers: map[string]string{
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestAuthHook_UsersListRequest(t *testing.T) {
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
			Name:            "unauthenticated request is rejected with 403",
			Method:          http.MethodGet,
			URL:             "/api/collections/users/records",
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
		{
			Name:   "conductor cannot list all users (403)",
			Method: http.MethodGet,
			URL:    "/api/collections/users/records",
			Headers: map[string]string{
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Administrator access required."`},
		},
		{
			Name:   "admin can list users",
			Method: http.MethodGet,
			URL:    "/api/collections/users/records",
			Headers: map[string]string{
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"items"`},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestAuthHook_RolesCreateRequest(t *testing.T) {
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
			Name:   "unauthenticated request is rejected with 403",
			Method: http.MethodPost,
			URL:    "/api/collections/roles/records",
			Body:   strings.NewReader(`{"user":"testuseralpha01","congregation":"testcongalpha01","role":"conductor"}`),
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  400,
			ExpectedContent: []string{`"status":400`},
		},
		{
			Name:   "conductor cannot create roles (403)",
			Method: http.MethodPost,
			URL:    "/api/collections/roles/records",
			Body:   strings.NewReader(`{"user":"testuseralpha01","congregation":"testcongalpha01","role":"conductor"}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Administrator access required."`},
		},
		{
			Name:   "admin can create role",
			Method: http.MethodPost,
			URL:    "/api/collections/roles/records",
			Body:   strings.NewReader(`{"user":"testuseralpha03","congregation":"testcongalpha01","role":"conductor"}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"testcongalpha01"`},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestAuthHook_RolesDeleteRequest(t *testing.T) {
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
			Name:            "unauthenticated request is rejected with 403",
			Method:          http.MethodDelete,
			URL:             "/api/collections/roles/records/testrolexcng01b",
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  404,
			ExpectedContent: []string{`"status":404`},
		},
		{
			Name:   "conductor cannot delete roles (403)",
			Method: http.MethodDelete,
			URL:    "/api/collections/roles/records/testrolexcng01b",
			Headers: map[string]string{
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Administrator access required."`},
		},
		{
			Name:   "admin can delete role",
			Method: http.MethodDelete,
			URL:    "/api/collections/roles/records/testrolexcng01b",
			Headers: map[string]string{
				"Authorization": adminToken,
			},
			TestAppFactory: setupTestApp,
			ExpectedStatus: 204,
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestAuthHook_AssignmentsCreateRequest(t *testing.T) {
	adminToken, err := generateToken("admin@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	conductorToken, err := generateToken("conductor@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	readonlyToken, err := generateToken("readonly@alpha.test")
	if err != nil {
		t.Fatal(err)
	}

	assignmentBody := `{"map":"testmapalpha01b","congregation":"testcongalpha01","publisher":"Test Publisher"}`

	scenarios := []tests.ApiScenario{
		{
			Name:   "unauthenticated request is rejected with 403",
			Method: http.MethodPost,
			URL:    "/api/collections/assignments/records",
			Body:   strings.NewReader(assignmentBody),
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  400,
			ExpectedContent: []string{`"status":400`},
		},
		{
			Name:   "readonly user cannot create assignments (403)",
			Method: http.MethodPost,
			URL:    "/api/collections/assignments/records",
			Body:   strings.NewReader(assignmentBody),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": readonlyToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Administrator or conductor access required."`},
		},
		{
			Name:   "conductor can create assignment",
			Method: http.MethodPost,
			URL:    "/api/collections/assignments/records",
			Body:   strings.NewReader(assignmentBody),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"testmapalpha01b"`},
		},
		{
			Name:   "admin can create assignment",
			Method: http.MethodPost,
			URL:    "/api/collections/assignments/records",
			Body:   strings.NewReader(assignmentBody),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"testmapalpha01b"`},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestAuthHook_RolesListRequest(t *testing.T) {
	conductorToken, err := generateToken("conductor@alpha.test")
	if err != nil {
		t.Fatal(err)
	}

	congFilterURL := "/api/collections/roles/records?filter=" + url.QueryEscape(`congregation="testcongalpha01"`)
	betaCongFilterURL := "/api/collections/roles/records?filter=" + url.QueryEscape(`congregation="testcongbeta001"`)
	// testuseralpha02 is the conductor's own user ID
	selfUserFilterURL := "/api/collections/roles/records?filter=" + url.QueryEscape(`user="testuseralpha02"`)
	otherUserFilterURL := "/api/collections/roles/records?filter=" + url.QueryEscape(`user="testuseralpha01"`)
	injectionURL := "/api/collections/roles/records?filter=" + url.QueryEscape(`congregation="testcongalpha01" || congregation="testcongbeta001"`)

	scenarios := []tests.ApiScenario{
		{
			Name:            "unauthenticated request is rejected with 403",
			Method:          http.MethodGet,
			URL:             congFilterURL,
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
		{
			Name:   "conductor with own congregation filter gets role list",
			Method: http.MethodGet,
			URL:    congFilterURL,
			Headers: map[string]string{
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"items"`},
		},
		{
			Name:   "conductor querying different congregation is rejected with 403",
			Method: http.MethodGet,
			URL:    betaCongFilterURL,
			Headers: map[string]string{
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
		{
			Name:   "conductor querying own user filter is allowed",
			Method: http.MethodGet,
			URL:    selfUserFilterURL,
			Headers: map[string]string{
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"items"`},
		},
		{
			Name:   "conductor querying another user filter is rejected with 403",
			Method: http.MethodGet,
			URL:    otherUserFilterURL,
			Headers: map[string]string{
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
		{
			Name:   "filter injection with two congregations is rejected with 403",
			Method: http.MethodGet,
			URL:    injectionURL,
			Headers: map[string]string{
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestAuthHook_AssignmentsListRequest(t *testing.T) {
	conductorToken, err := generateToken("conductor@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	readonlyToken, err := generateToken("readonly@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	betaConductorToken, err := generateToken("xcong@beta.test")
	if err != nil {
		t.Fatal(err)
	}

	alphaMapFilterURL := "/api/collections/assignments/records?filter=" + url.QueryEscape(`map="testmapalpha01a"`)
	betaMapFilterURL := "/api/collections/assignments/records?filter=" + url.QueryEscape(`map="testmapbeta001a"`)
	// testuseralpha02 is conductor, testuseralpha03 is readonly, testuseralpha01 is admin
	selfConductorFilterURL := "/api/collections/assignments/records?filter=" + url.QueryEscape(`user="testuseralpha02"`)
	selfReadonlyFilterURL := "/api/collections/assignments/records?filter=" + url.QueryEscape(`user="testuseralpha03"`)
	otherUserFilterURL := "/api/collections/assignments/records?filter=" + url.QueryEscape(`user="testuseralpha01"`)

	scenarios := []tests.ApiScenario{
		{
			Name:            "unauthenticated request is rejected with 403",
			Method:          http.MethodGet,
			URL:             alphaMapFilterURL,
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
		{
			Name:   "conductor with their map filter gets assignment list",
			Method: http.MethodGet,
			URL:    alphaMapFilterURL,
			Headers: map[string]string{
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"items"`},
		},
		{
			Name:   "conductor querying different congregation map is rejected",
			Method: http.MethodGet,
			URL:    betaMapFilterURL,
			Headers: map[string]string{
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
		{
			Name:   "conductor querying own user filter is allowed",
			Method: http.MethodGet,
			URL:    selfConductorFilterURL,
			Headers: map[string]string{
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"items"`},
		},
		{
			Name:   "readonly user querying own user filter is allowed",
			Method: http.MethodGet,
			URL:    selfReadonlyFilterURL,
			Headers: map[string]string{
				"Authorization": readonlyToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"items"`},
		},
		{
			Name:   "readonly user querying other user filter is rejected",
			Method: http.MethodGet,
			URL:    otherUserFilterURL,
			Headers: map[string]string{
				"Authorization": readonlyToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
		{
			Name:   "conductor querying other user filter is allowed (admin/conductor anywhere)",
			Method: http.MethodGet,
			URL:    otherUserFilterURL,
			Headers: map[string]string{
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"items"`},
		},
		{
			Name:   "beta conductor querying alpha map is rejected",
			Method: http.MethodGet,
			URL:    alphaMapFilterURL,
			Headers: map[string]string{
				"Authorization": betaConductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestAuthHook_UsersViewRequest(t *testing.T) {
	adminToken, err := generateToken("admin@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	conductorToken, err := generateToken("conductor@alpha.test")
	if err != nil {
		t.Fatal(err)
	}

	// testuseralpha01 = admin, testuseralpha02 = conductor
	scenarios := []tests.ApiScenario{
		{
			Name:            "unauthenticated request cannot view user (404)",
			Method:          http.MethodGet,
			URL:             "/api/collections/users/records/testuseralpha01",
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  404,
			ExpectedContent: []string{`"status":404`},
		},
		{
			Name:   "conductor can view own profile",
			Method: http.MethodGet,
			URL:    "/api/collections/users/records/testuseralpha02",
			Headers: map[string]string{
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"testuseralpha02"`},
		},
		{
			Name:   "admin can view another user",
			Method: http.MethodGet,
			URL:    "/api/collections/users/records/testuseralpha02",
			Headers: map[string]string{
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"testuseralpha02"`},
		},
		{
			Name:   "conductor cannot view another user (403)",
			Method: http.MethodGet,
			URL:    "/api/collections/users/records/testuseralpha01",
			Headers: map[string]string{
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Administrator access required."`},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestAuthHook_MapsViewRequest(t *testing.T) {
	conductorToken, err := generateToken("conductor@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	betaConductorToken, err := generateToken("xcong@beta.test")
	if err != nil {
		t.Fatal(err)
	}

	scenarios := []tests.ApiScenario{
		{
			Name:            "unauthenticated request without link-id cannot view map (404)",
			Method:          http.MethodGet,
			URL:             "/api/collections/maps/records/testmapalpha01a",
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  404,
			ExpectedContent: []string{`"status":404`},
		},
		{
			Name:   "conductor in correct congregation can view map",
			Method: http.MethodGet,
			URL:    "/api/collections/maps/records/testmapalpha01a",
			Headers: map[string]string{
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"testmapalpha01a"`},
		},
		{
			Name:   "conductor from different congregation is rejected with 403",
			Method: http.MethodGet,
			URL:    "/api/collections/maps/records/testmapalpha01a",
			Headers: map[string]string{
				"Authorization": betaConductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
		{
			Name:   "valid link-id allows map view without auth token",
			Method: http.MethodGet,
			URL:    "/api/collections/maps/records/testmapalpha01a",
			Headers: map[string]string{
				"link-id": "testassignalpha01",
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"testmapalpha01a"`},
		},
		{
			Name:   "invalid link-id is rejected with 403",
			Method: http.MethodGet,
			URL:    "/api/collections/maps/records/testmapalpha01a",
			Headers: map[string]string{
				"link-id": "invalidlinkid000",
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestAuthHook_CongregationsViewRequest(t *testing.T) {
	adminToken, err := generateToken("admin@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	conductorToken, err := generateToken("conductor@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	betaConductorToken, err := generateToken("xcong@beta.test")
	if err != nil {
		t.Fatal(err)
	}

	scenarios := []tests.ApiScenario{
		{
			Name:            "unauthenticated request without link-id cannot view congregation (404)",
			Method:          http.MethodGet,
			URL:             "/api/collections/congregations/records/testcongalpha01",
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  404,
			ExpectedContent: []string{`"status":404`},
		},
		{
			Name:   "admin can view own congregation",
			Method: http.MethodGet,
			URL:    "/api/collections/congregations/records/testcongalpha01",
			Headers: map[string]string{
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"testcongalpha01"`},
		},
		{
			Name:   "conductor can view own congregation",
			Method: http.MethodGet,
			URL:    "/api/collections/congregations/records/testcongalpha01",
			Headers: map[string]string{
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"testcongalpha01"`},
		},
		{
			Name:   "conductor from different congregation is rejected with 403",
			Method: http.MethodGet,
			URL:    "/api/collections/congregations/records/testcongalpha01",
			Headers: map[string]string{
				"Authorization": betaConductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
		{
			Name:   "valid link-id for congregation allows view without auth token",
			Method: http.MethodGet,
			URL:    "/api/collections/congregations/records/testcongalpha01",
			Headers: map[string]string{
				"link-id": "testassignalpha01",
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"testcongalpha01"`},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestAuthHook_OptionsListRequest(t *testing.T) {
	conductorToken, err := generateToken("conductor@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	betaConductorToken, err := generateToken("xcong@beta.test")
	if err != nil {
		t.Fatal(err)
	}

	filteredURL := "/api/collections/options/records?filter=" + url.QueryEscape(`congregation="testcongalpha01"`)
	betaFilteredURL := "/api/collections/options/records?filter=" + url.QueryEscape(`congregation="testcongbeta001"`)
	injectionURL := "/api/collections/options/records?filter=" + url.QueryEscape(`congregation="testcongalpha01" || congregation="testcongbeta001"`)

	scenarios := []tests.ApiScenario{
		{
			Name:            "unauthenticated request is rejected with 403",
			Method:          http.MethodGet,
			URL:             filteredURL,
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
		{
			Name:   "conductor with congregation filter gets options list",
			Method: http.MethodGet,
			URL:    filteredURL,
			Headers: map[string]string{
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"items"`},
		},
		{
			Name:   "request without congregation filter is rejected with 403",
			Method: http.MethodGet,
			URL:    "/api/collections/options/records",
			Headers: map[string]string{
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
		{
			Name:   "conductor querying different congregation is rejected with 403",
			Method: http.MethodGet,
			URL:    betaFilteredURL,
			Headers: map[string]string{
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
		{
			Name:   "beta conductor with beta congregation filter gets options list",
			Method: http.MethodGet,
			URL:    betaFilteredURL,
			Headers: map[string]string{
				"Authorization": betaConductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"items"`},
		},
		{
			Name:   "valid link-id for congregation allows options list",
			Method: http.MethodGet,
			URL:    filteredURL,
			Headers: map[string]string{
				"link-id": "testassignalpha01",
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"items"`},
		},
		{
			Name:   "filter injection with two congregations is rejected with 403",
			Method: http.MethodGet,
			URL:    injectionURL,
			Headers: map[string]string{
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
		{
			Name:   "link-id with multi-congregation filter injection is rejected with 403",
			Method: http.MethodGet,
			URL:    injectionURL,
			Headers: map[string]string{
				"link-id": "testassignalpha01",
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestAuthHook_OptionsViewRequest(t *testing.T) {
	conductorToken, err := generateToken("conductor@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	betaConductorToken, err := generateToken("xcong@beta.test")
	if err != nil {
		t.Fatal(err)
	}

	scenarios := []tests.ApiScenario{
		{
			Name:            "unauthenticated request without link-id cannot view option (404)",
			Method:          http.MethodGet,
			URL:             "/api/collections/options/records/testoptialpha01",
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  404,
			ExpectedContent: []string{`"status":404`},
		},
		{
			Name:   "conductor can view option in own congregation",
			Method: http.MethodGet,
			URL:    "/api/collections/options/records/testoptialpha01",
			Headers: map[string]string{
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"testoptialpha01"`},
		},
		{
			Name:   "conductor from different congregation is rejected with 403",
			Method: http.MethodGet,
			URL:    "/api/collections/options/records/testoptialpha01",
			Headers: map[string]string{
				"Authorization": betaConductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
		{
			Name:   "valid link-id for congregation allows option view",
			Method: http.MethodGet,
			URL:    "/api/collections/options/records/testoptialpha01",
			Headers: map[string]string{
				"link-id": "testassignalpha01",
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"testoptialpha01"`},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestAuthHook_AddressOptionsViewRequest(t *testing.T) {
	conductorToken, err := generateToken("conductor@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	betaConductorToken, err := generateToken("xcong@beta.test")
	if err != nil {
		t.Fatal(err)
	}

	// testaoalph01001: map=testmapalpha01a, congregation=testcongalpha01
	scenarios := []tests.ApiScenario{
		{
			Name:            "unauthenticated request without link-id cannot view address option (404)",
			Method:          http.MethodGet,
			URL:             "/api/collections/address_options/records/testaoalph01001",
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  404,
			ExpectedContent: []string{`"status":404`},
		},
		{
			Name:   "conductor in correct congregation can view address option",
			Method: http.MethodGet,
			URL:    "/api/collections/address_options/records/testaoalph01001",
			Headers: map[string]string{
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"testaoalph01001"`},
		},
		{
			Name:   "conductor from different congregation is rejected with 403",
			Method: http.MethodGet,
			URL:    "/api/collections/address_options/records/testaoalph01001",
			Headers: map[string]string{
				"Authorization": betaConductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
		{
			Name:   "valid link-id for map allows address option view",
			Method: http.MethodGet,
			URL:    "/api/collections/address_options/records/testaoalph01001",
			Headers: map[string]string{
				"link-id": "testassignalpha01",
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"testaoalph01001"`},
		},
		{
			Name:   "invalid link-id is rejected with 403",
			Method: http.MethodGet,
			URL:    "/api/collections/address_options/records/testaoalph01001",
			Headers: map[string]string{
				"link-id": "invalidlinkid000",
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestAuthHook_AddressesListRequest(t *testing.T) {
	conductorToken, err := generateToken("conductor@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	betaConductorToken, err := generateToken("xcong@beta.test")
	if err != nil {
		t.Fatal(err)
	}

	mapFilterURL := "/api/collections/addresses/records?filter=" + url.QueryEscape(`map="testmapalpha01a"`)
	betaMapFilterURL := "/api/collections/addresses/records?filter=" + url.QueryEscape(`map="testmapbeta001a"`)
	injectionURL := "/api/collections/addresses/records?filter=" + url.QueryEscape(`map="testmapalpha01a" || map="testmapbeta001a"`)

	scenarios := []tests.ApiScenario{
		{
			Name:            "unauthenticated request is rejected with 403",
			Method:          http.MethodGet,
			URL:             mapFilterURL,
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
		{
			Name:   "conductor with valid map filter gets address list",
			Method: http.MethodGet,
			URL:    mapFilterURL,
			Headers: map[string]string{
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"items"`},
		},
		{
			Name:   "request without map filter is rejected with 403",
			Method: http.MethodGet,
			URL:    "/api/collections/addresses/records",
			Headers: map[string]string{
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
		{
			Name:   "conductor from different congregation is rejected with 403",
			Method: http.MethodGet,
			URL:    betaMapFilterURL,
			Headers: map[string]string{
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
		{
			Name:   "valid link-id for map allows address list",
			Method: http.MethodGet,
			URL:    mapFilterURL,
			Headers: map[string]string{
				"link-id": "testassignalpha01",
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"items"`},
		},
		{
			Name:   "invalid link-id is rejected with 403",
			Method: http.MethodGet,
			URL:    mapFilterURL,
			Headers: map[string]string{
				"link-id": "invalidlinkid000",
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
		{
			Name:   "filter injection with two maps is rejected with 403",
			Method: http.MethodGet,
			URL:    injectionURL,
			Headers: map[string]string{
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
		{
			Name:   "link-id with multi-map filter injection is rejected with 403",
			Method: http.MethodGet,
			URL:    injectionURL,
			Headers: map[string]string{
				"link-id": "testassignalpha01",
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
	}

	_ = betaConductorToken
	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestAuthHook_AddressOptionsListRequest(t *testing.T) {
	conductorToken, err := generateToken("conductor@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	betaConductorToken, err := generateToken("xcong@beta.test")
	if err != nil {
		t.Fatal(err)
	}

	mapFilterURL := "/api/collections/address_options/records?filter=" + url.QueryEscape(`map="testmapalpha01a"`)
	injectionURL := "/api/collections/address_options/records?filter=" + url.QueryEscape(`map="testmapalpha01a" || map="testmapbeta001a"`)

	scenarios := []tests.ApiScenario{
		{
			Name:            "unauthenticated request is rejected with 403",
			Method:          http.MethodGet,
			URL:             mapFilterURL,
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
		{
			Name:   "conductor with valid map filter gets address_options list",
			Method: http.MethodGet,
			URL:    mapFilterURL,
			Headers: map[string]string{
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"items"`},
		},
		{
			Name:   "request without map filter is rejected with 403",
			Method: http.MethodGet,
			URL:    "/api/collections/address_options/records",
			Headers: map[string]string{
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
		{
			Name:   "conductor from different congregation is rejected with 403",
			Method: http.MethodGet,
			URL:    mapFilterURL,
			Headers: map[string]string{
				"Authorization": betaConductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
		{
			Name:   "valid link-id for map allows address_options list",
			Method: http.MethodGet,
			URL:    mapFilterURL,
			Headers: map[string]string{
				"link-id": "testassignalpha01",
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"items"`},
		},
		{
			Name:   "filter injection with two maps is rejected with 403",
			Method: http.MethodGet,
			URL:    injectionURL,
			Headers: map[string]string{
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
		{
			Name:   "link-id with multi-map filter injection is rejected with 403",
			Method: http.MethodGet,
			URL:    injectionURL,
			Headers: map[string]string{
				"link-id": "testassignalpha01",
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestAuthHook_MessagesListRequest(t *testing.T) {
	conductorToken, err := generateToken("conductor@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	betaConductorToken, err := generateToken("xcong@beta.test")
	if err != nil {
		t.Fatal(err)
	}

	mapFilterURL := "/api/collections/messages/records?filter=" + url.QueryEscape(`map="testmapalpha01a"`)
	injectionURL := "/api/collections/messages/records?filter=" + url.QueryEscape(`map="testmapalpha01a" || map="testmapbeta001a"`)

	scenarios := []tests.ApiScenario{
		{
			Name:            "unauthenticated request is rejected with 403",
			Method:          http.MethodGet,
			URL:             mapFilterURL,
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
		{
			Name:   "conductor with valid map filter gets message list",
			Method: http.MethodGet,
			URL:    mapFilterURL,
			Headers: map[string]string{
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"items"`},
		},
		{
			Name:   "request without map filter is rejected with 403",
			Method: http.MethodGet,
			URL:    "/api/collections/messages/records",
			Headers: map[string]string{
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
		{
			Name:   "conductor from different congregation is rejected with 403",
			Method: http.MethodGet,
			URL:    mapFilterURL,
			Headers: map[string]string{
				"Authorization": betaConductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
		{
			Name:   "valid link-id for map allows message list",
			Method: http.MethodGet,
			URL:    mapFilterURL,
			Headers: map[string]string{
				"link-id": "testassignalpha01",
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"items"`},
		},
		{
			Name:   "filter injection with two maps is rejected with 403",
			Method: http.MethodGet,
			URL:    injectionURL,
			Headers: map[string]string{
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
		{
			Name:   "link-id with multi-map filter injection is rejected with 403",
			Method: http.MethodGet,
			URL:    injectionURL,
			Headers: map[string]string{
				"link-id": "testassignalpha01",
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestAuthHook_AssignmentsViewRequest(t *testing.T) {
	conductorToken, err := generateToken("conductor@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	betaConductorToken, err := generateToken("xcong@beta.test")
	if err != nil {
		t.Fatal(err)
	}

	// testassignalpha01: congregation=testcongalpha01, expiry=2099 (from seed)
	scenarios := []tests.ApiScenario{
		{
			Name:            "unauthenticated request without link-id cannot view assignment (404)",
			Method:          http.MethodGet,
			URL:             "/api/collections/assignments/records/testassignalpha01",
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  404,
			ExpectedContent: []string{`"status":404`},
		},
		{
			Name:   "conductor in correct congregation can view assignment",
			Method: http.MethodGet,
			URL:    "/api/collections/assignments/records/testassignalpha01",
			Headers: map[string]string{
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"testassignalpha01"`},
		},
		{
			Name:   "conductor from different congregation is rejected with 403",
			Method: http.MethodGet,
			URL:    "/api/collections/assignments/records/testassignalpha01",
			Headers: map[string]string{
				"Authorization": betaConductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
		{
			Name:   "link-id matching assignment ID allows view without auth token",
			Method: http.MethodGet,
			URL:    "/api/collections/assignments/records/testassignalpha01",
			Headers: map[string]string{
				"link-id": "testassignalpha01",
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"testassignalpha01"`},
		},
		{
			Name:   "link-id not matching assignment ID is rejected with 403",
			Method: http.MethodGet,
			URL:    "/api/collections/assignments/records/testassignalpha01",
			Headers: map[string]string{
				"link-id": "wronglinkid00000",
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestAuthHook_TerritoriesCreateRequest(t *testing.T) {
	adminToken, err := generateToken("admin@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	conductorToken, err := generateToken("conductor@alpha.test")
	if err != nil {
		t.Fatal(err)
	}

	body := `{"congregation":"testcongalpha01","code":"T99","description":"Test Territory"}`

	scenarios := []tests.ApiScenario{
		{
			Name:   "unauthenticated request is rejected with 400",
			Method: http.MethodPost,
			URL:    "/api/collections/territories/records",
			Body:   strings.NewReader(body),
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  400,
			ExpectedContent: []string{`"status":400`},
		},
		{
			Name:   "conductor cannot create territory (403)",
			Method: http.MethodPost,
			URL:    "/api/collections/territories/records",
			Body:   strings.NewReader(body),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Administrator access required."`},
		},
		{
			Name:   "admin can create territory",
			Method: http.MethodPost,
			URL:    "/api/collections/territories/records",
			Body:   strings.NewReader(body),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"testcongalpha01"`},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestAuthHook_TerritoriesUpdateRequest(t *testing.T) {
	adminToken, err := generateToken("admin@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	conductorToken, err := generateToken("conductor@alpha.test")
	if err != nil {
		t.Fatal(err)
	}

	body := `{"description":"Updated description"}`

	scenarios := []tests.ApiScenario{
		{
			Name:            "unauthenticated request cannot find territory (404)",
			Method:          http.MethodPatch,
			URL:             "/api/collections/territories/records/testterralpha01",
			Body:            strings.NewReader(body),
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  404,
			ExpectedContent: []string{`"status":404`},
		},
		{
			Name:   "conductor cannot update territory (403)",
			Method: http.MethodPatch,
			URL:    "/api/collections/territories/records/testterralpha01",
			Body:   strings.NewReader(body),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Administrator access required."`},
		},
		{
			Name:   "admin can update territory",
			Method: http.MethodPatch,
			URL:    "/api/collections/territories/records/testterralpha01",
			Body:   strings.NewReader(body),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"testterralpha01"`},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestAuthHook_TerritoriesDeleteRequest(t *testing.T) {
	adminToken, err := generateToken("admin@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	conductorToken, err := generateToken("conductor@alpha.test")
	if err != nil {
		t.Fatal(err)
	}

	// testterralpha02 has maps testmapalpha02a/testmapalpha02b; cascade delete handles them
	scenarios := []tests.ApiScenario{
		{
			Name:            "unauthenticated request cannot find territory (404)",
			Method:          http.MethodDelete,
			URL:             "/api/collections/territories/records/testterralpha02",
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  404,
			ExpectedContent: []string{`"status":404`},
		},
		{
			Name:   "conductor cannot delete territory (403)",
			Method: http.MethodDelete,
			URL:    "/api/collections/territories/records/testterralpha02",
			Headers: map[string]string{
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Administrator access required."`},
		},
		{
			Name:   "admin can delete territory",
			Method: http.MethodDelete,
			URL:    "/api/collections/territories/records/testterralpha02",
			Headers: map[string]string{
				"Authorization": adminToken,
			},
			TestAppFactory: setupTestApp,
			ExpectedStatus: 204,
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestAuthHook_MapsUpdateRequest(t *testing.T) {
	adminToken, err := generateToken("admin@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	conductorToken, err := generateToken("conductor@alpha.test")
	if err != nil {
		t.Fatal(err)
	}

	body := `{"description":"Updated description"}`

	scenarios := []tests.ApiScenario{
		{
			Name:            "unauthenticated request cannot find map (404)",
			Method:          http.MethodPatch,
			URL:             "/api/collections/maps/records/testmapalpha01a",
			Body:            strings.NewReader(body),
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  404,
			ExpectedContent: []string{`"status":404`},
		},
		{
			Name:   "conductor cannot update map (403)",
			Method: http.MethodPatch,
			URL:    "/api/collections/maps/records/testmapalpha01a",
			Body:   strings.NewReader(body),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Administrator access required."`},
		},
		{
			Name:   "admin can update map",
			Method: http.MethodPatch,
			URL:    "/api/collections/maps/records/testmapalpha01a",
			Body:   strings.NewReader(body),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"testmapalpha01a"`},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestAuthHook_MapsDeleteRequest(t *testing.T) {
	adminToken, err := generateToken("admin@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	conductorToken, err := generateToken("conductor@alpha.test")
	if err != nil {
		t.Fatal(err)
	}

	// testmapalpha02b has addresses; cascade delete handles them
	scenarios := []tests.ApiScenario{
		{
			Name:            "unauthenticated request cannot find map (404)",
			Method:          http.MethodDelete,
			URL:             "/api/collections/maps/records/testmapalpha02b",
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  404,
			ExpectedContent: []string{`"status":404`},
		},
		{
			Name:   "conductor cannot delete map (403)",
			Method: http.MethodDelete,
			URL:    "/api/collections/maps/records/testmapalpha02b",
			Headers: map[string]string{
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Administrator access required."`},
		},
		{
			Name:   "admin can delete map",
			Method: http.MethodDelete,
			URL:    "/api/collections/maps/records/testmapalpha02b",
			Headers: map[string]string{
				"Authorization": adminToken,
			},
			TestAppFactory: setupTestApp,
			ExpectedStatus: 204,
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestAuthHook_RolesUpdateRequest(t *testing.T) {
	adminToken, err := generateToken("admin@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	conductorToken, err := generateToken("conductor@alpha.test")
	if err != nil {
		t.Fatal(err)
	}

	// testrolexcng01c is the read_only role for testuseralpha03
	body := `{"role":"read_only"}`

	scenarios := []tests.ApiScenario{
		{
			Name:            "unauthenticated request cannot find role (404)",
			Method:          http.MethodPatch,
			URL:             "/api/collections/roles/records/testrolexcng01c",
			Body:            strings.NewReader(body),
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  404,
			ExpectedContent: []string{`"status":404`},
		},
		{
			Name:   "conductor cannot update role (403)",
			Method: http.MethodPatch,
			URL:    "/api/collections/roles/records/testrolexcng01c",
			Body:   strings.NewReader(body),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Administrator access required."`},
		},
		{
			Name:   "admin can update role",
			Method: http.MethodPatch,
			URL:    "/api/collections/roles/records/testrolexcng01c",
			Body:   strings.NewReader(body),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"testrolexcng01c"`},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestAuthHook_CongregationsUpdateRequest(t *testing.T) {
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

	body := `{"name":"Alpha Congregation Updated"}`

	scenarios := []tests.ApiScenario{
		{
			Name:            "unauthenticated request cannot find congregation (404)",
			Method:          http.MethodPatch,
			URL:             "/api/collections/congregations/records/testcongalpha01",
			Body:            strings.NewReader(body),
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  404,
			ExpectedContent: []string{`"status":404`},
		},
		{
			Name:   "conductor cannot update congregation (403)",
			Method: http.MethodPatch,
			URL:    "/api/collections/congregations/records/testcongalpha01",
			Body:   strings.NewReader(body),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Administrator access required."`},
		},
		{
			Name:   "admin from different congregation cannot update congregation (403)",
			Method: http.MethodPatch,
			URL:    "/api/collections/congregations/records/testcongalpha01",
			Body:   strings.NewReader(body),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": betaAdminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Administrator access required."`},
		},
		{
			Name:   "admin can update own congregation",
			Method: http.MethodPatch,
			URL:    "/api/collections/congregations/records/testcongalpha01",
			Body:   strings.NewReader(body),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"testcongalpha01"`},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestAuthHook_AddressesCreateRequest(t *testing.T) {
	conductorToken, err := generateToken("conductor@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	readonlyToken, err := generateToken("readonly@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	betaConductorToken, err := generateToken("xcong@beta.test")
	if err != nil {
		t.Fatal(err)
	}

	body := `{"map_id":"testmapalpha01a","code":"09","status":"not_done","floor":1}`

	scenarios := []tests.ApiScenario{
		{
			Name:   "unauthenticated request is rejected with 403",
			Method: http.MethodPost,
			URL:    "/address/add",
			Body:   strings.NewReader(body),
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
		{
			Name:   "conductor can create address",
			Method: http.MethodPost,
			URL:    "/address/add",
			Body:   strings.NewReader(body),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  201,
			ExpectedContent: []string{`"id"`},
		},
		{
			Name:   "readonly user can create address (any role is allowed)",
			Method: http.MethodPost,
			URL:    "/address/add",
			Body:   strings.NewReader(body),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": readonlyToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  201,
			ExpectedContent: []string{`"id"`},
		},
		{
			Name:   "conductor from different congregation is rejected with 403",
			Method: http.MethodPost,
			URL:    "/address/add",
			Body:   strings.NewReader(body),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": betaConductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
		{
			Name:   "valid link-id allows address creation without auth token",
			Method: http.MethodPost,
			URL:    "/address/add",
			Body:   strings.NewReader(body),
			Headers: map[string]string{
				"Content-Type": "application/json",
				"link-id":      "testassignalpha01",
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  201,
			ExpectedContent: []string{`"id"`},
		},
		{
			Name:   "client-generated address_id is used as record id",
			Method: http.MethodPost,
			URL:    "/address/add",
			Body:   strings.NewReader(`{"address_id":"clientgenid0001","map_id":"testmapalpha01a","code":"88","status":"not_done","floor":1}`),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			TestAppFactory: setupTestApp,
			ExpectedStatus: 201,
			ExpectedContent: []string{`"id":"clientgenid0001"`},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestAuthHook_AddressesUpdateRequest(t *testing.T) {
	conductorToken, err := generateToken("conductor@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	betaConductorToken, err := generateToken("xcong@beta.test")
	if err != nil {
		t.Fatal(err)
	}

	body := `{"address_id":"testalpha01a001","map_id":"testmapalpha01a","status":"not_home"}`

	scenarios := []tests.ApiScenario{
		{
			Name:   "unauthenticated request is rejected with 403",
			Method: http.MethodPost,
			URL:    "/address/update",
			Body:   strings.NewReader(body),
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
		{
			Name:   "conductor can update address",
			Method: http.MethodPost,
			URL:    "/address/update",
			Body:   strings.NewReader(body),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			TestAppFactory: setupTestApp,
			ExpectedStatus: 204,
		},
		{
			Name:   "conductor from different congregation is rejected with 403",
			Method: http.MethodPost,
			URL:    "/address/update",
			Body:   strings.NewReader(body),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": betaConductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
		{
			Name:   "valid link-id allows address update without auth token",
			Method: http.MethodPost,
			URL:    "/address/update",
			Body:   strings.NewReader(body),
			Headers: map[string]string{
				"Content-Type": "application/json",
				"link-id":      "testassignalpha01",
			},
			TestAppFactory: setupTestApp,
			ExpectedStatus: 204,
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

// TestAuthHook_AddressOptionsCreateRequest verifies that the address_options
// collection has createRule=null (superuser-only). Direct REST API creation is
// blocked for all callers; address_options must be managed via /address/add
// or /address/update.
func TestAuthHook_AddressOptionsCreateRequest(t *testing.T) {
	conductorToken, err := generateToken("conductor@alpha.test")
	if err != nil {
		t.Fatal(err)
	}

	body := `{"address":"testalpha01a001","map":"testmapalpha01a","congregation":"testcongalpha01","option":"testoptialpha02"}`

	scenarios := []tests.ApiScenario{
		{
			Name:   "unauthenticated request is rejected with 403",
			Method: http.MethodPost,
			URL:    "/api/collections/address_options/records",
			Body:   strings.NewReader(body),
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
		{
			Name:   "authenticated conductor is rejected with 403 — use /address/update or /address/add",
			Method: http.MethodPost,
			URL:    "/api/collections/address_options/records",
			Body:   strings.NewReader(body),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
		{
			Name:   "link-id request is rejected with 403 — use /address/update or /address/add",
			Method: http.MethodPost,
			URL:    "/api/collections/address_options/records",
			Body:   strings.NewReader(body),
			Headers: map[string]string{
				"Content-Type": "application/json",
				"link-id":      "testassignalpha01",
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

// TestAuthHook_AddressOptionsDeleteRequest verifies that the address_options
// collection has deleteRule=null (superuser-only). Direct REST API deletion is
// blocked for all callers; address_options must be managed via /address/update.
func TestAuthHook_AddressOptionsDeleteRequest(t *testing.T) {
	conductorToken, err := generateToken("conductor@alpha.test")
	if err != nil {
		t.Fatal(err)
	}

	scenarios := []tests.ApiScenario{
		{
			Name:            "unauthenticated request is rejected with 403",
			Method:          http.MethodDelete,
			URL:             "/api/collections/address_options/records/testaoalph01001",
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
		{
			Name:   "authenticated conductor is rejected with 403 — use /address/update",
			Method: http.MethodDelete,
			URL:    "/api/collections/address_options/records/testaoalph01001",
			Headers: map[string]string{
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
		{
			Name:   "link-id request is rejected with 403 — use /address/update",
			Method: http.MethodDelete,
			URL:    "/api/collections/address_options/records/testaoalph01002",
			Headers: map[string]string{
				"link-id": "testassignalpha01",
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestAuthHook_MessagesCreateRequest(t *testing.T) {
	conductorToken, err := generateToken("conductor@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	betaConductorToken, err := generateToken("xcong@beta.test")
	if err != nil {
		t.Fatal(err)
	}

	body := `{"map":"testmapalpha01a","congregation":"testcongalpha01","message":"Test feedback","type":"publisher","read":false,"created_by":"Test Publisher"}`

	scenarios := []tests.ApiScenario{
		{
			Name:   "unauthenticated request is rejected with 400",
			Method: http.MethodPost,
			URL:    "/api/collections/messages/records",
			Body:   strings.NewReader(body),
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  400,
			ExpectedContent: []string{`"status":400`},
		},
		{
			Name:   "conductor can create message",
			Method: http.MethodPost,
			URL:    "/api/collections/messages/records",
			Body:   strings.NewReader(body),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"testmapalpha01a"`},
		},
		{
			Name:   "conductor from different congregation is rejected with 403",
			Method: http.MethodPost,
			URL:    "/api/collections/messages/records",
			Body:   strings.NewReader(body),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": betaConductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
		{
			Name:   "valid link-id allows message creation without auth token",
			Method: http.MethodPost,
			URL:    "/api/collections/messages/records",
			Body:   strings.NewReader(body),
			Headers: map[string]string{
				"Content-Type": "application/json",
				"link-id":      "testassignalpha01",
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"testmapalpha01a"`},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestAuthHook_AssignmentsDeleteRequest(t *testing.T) {
	adminToken, err := generateToken("admin@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	conductorToken, err := generateToken("conductor@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	readonlyToken, err := generateToken("readonly@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	betaConductorToken, err := generateToken("xcong@beta.test")
	if err != nil {
		t.Fatal(err)
	}

	// testassignalpha01: congregation=testcongalpha01 (from seed); each scenario gets fresh DB
	scenarios := []tests.ApiScenario{
		{
			Name:            "unauthenticated request cannot find assignment (404)",
			Method:          http.MethodDelete,
			URL:             "/api/collections/assignments/records/testassignalpha01",
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  404,
			ExpectedContent: []string{`"status":404`},
		},
		{
			Name:   "readonly user cannot delete assignment (403)",
			Method: http.MethodDelete,
			URL:    "/api/collections/assignments/records/testassignalpha01",
			Headers: map[string]string{
				"Authorization": readonlyToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Administrator or conductor access required."`},
		},
		{
			Name:   "conductor from different congregation cannot delete assignment (403)",
			Method: http.MethodDelete,
			URL:    "/api/collections/assignments/records/testassignalpha01",
			Headers: map[string]string{
				"Authorization": betaConductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Administrator or conductor access required."`},
		},
		{
			Name:   "conductor in correct congregation can delete assignment",
			Method: http.MethodDelete,
			URL:    "/api/collections/assignments/records/testassignalpha01",
			Headers: map[string]string{
				"Authorization": conductorToken,
			},
			TestAppFactory: setupTestApp,
			ExpectedStatus: 204,
		},
		{
			Name:   "admin can delete assignment",
			Method: http.MethodDelete,
			URL:    "/api/collections/assignments/records/testassignalpha01",
			Headers: map[string]string{
				"Authorization": adminToken,
			},
			TestAppFactory: setupTestApp,
			ExpectedStatus: 204,
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestAuthHook_MessagesUpdateRequest(t *testing.T) {
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

	body := `{"read":true}`
	// mutationBody tries to change congregation to beta — hook uses Original() so alpha admin still governs
	mutationBody := `{"congregation":"testcongbeta001","read":true}`

	// testmsgalpha01a: congregation=testcongalpha01 (from seed); each scenario gets fresh DB
	scenarios := []tests.ApiScenario{
		{
			Name:            "unauthenticated request cannot find message (404)",
			Method:          http.MethodPatch,
			URL:             "/api/collections/messages/records/testmsgalpha01a",
			Body:            strings.NewReader(body),
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  404,
			ExpectedContent: []string{`"status":404`},
		},
		{
			Name:   "conductor cannot update message (403)",
			Method: http.MethodPatch,
			URL:    "/api/collections/messages/records/testmsgalpha01a",
			Body:   strings.NewReader(body),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Administrator access required."`},
		},
		{
			Name:   "admin from different congregation cannot update message (403)",
			Method: http.MethodPatch,
			URL:    "/api/collections/messages/records/testmsgalpha01a",
			Body:   strings.NewReader(body),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": betaAdminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Administrator access required."`},
		},
		{
			Name:   "admin can update message",
			Method: http.MethodPatch,
			URL:    "/api/collections/messages/records/testmsgalpha01a",
			Body:   strings.NewReader(body),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": adminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"testmsgalpha01a"`},
		},
		{
			Name:   "congregation mutation in body does not bypass auth (403 for beta admin)",
			Method: http.MethodPatch,
			URL:    "/api/collections/messages/records/testmsgalpha01a",
			Body:   strings.NewReader(mutationBody),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": betaAdminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Administrator access required."`},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestAuthHook_MessagesDeleteRequest(t *testing.T) {
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

	// testmsgalpha01a: congregation=testcongalpha01 (from seed); each scenario gets fresh DB
	scenarios := []tests.ApiScenario{
		{
			Name:            "unauthenticated request cannot find message (404)",
			Method:          http.MethodDelete,
			URL:             "/api/collections/messages/records/testmsgalpha01a",
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  404,
			ExpectedContent: []string{`"status":404`},
		},
		{
			Name:   "conductor cannot delete message (403)",
			Method: http.MethodDelete,
			URL:    "/api/collections/messages/records/testmsgalpha01a",
			Headers: map[string]string{
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Administrator access required."`},
		},
		{
			Name:   "admin from different congregation cannot delete message (403)",
			Method: http.MethodDelete,
			URL:    "/api/collections/messages/records/testmsgalpha01a",
			Headers: map[string]string{
				"Authorization": betaAdminToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"Administrator access required."`},
		},
		{
			Name:   "admin can delete message",
			Method: http.MethodDelete,
			URL:    "/api/collections/messages/records/testmsgalpha01a",
			Headers: map[string]string{
				"Authorization": adminToken,
			},
			TestAppFactory: setupTestApp,
			ExpectedStatus: 204,
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

