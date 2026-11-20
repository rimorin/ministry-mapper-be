//go:build testdata

package setup

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"ganigeorgiev.com/pocketbase/pocketbase/tests"
)

// Apple posts its reply to the backend, which hands it to the web app's
// sign-in page and nowhere else.
func TestAppleCallback(t *testing.T) {
	form := func(values map[string]string) *strings.Reader {
		body := url.Values{}
		for k, v := range values {
			body.Set(k, v)
		}
		return strings.NewReader(body.Encode())
	}
	headers := map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
	appURL := func(t testing.TB, _ *tests.TestApp, _ *core.ServeEvent) {
		t.Setenv("PB_APP_URL", "https://app.example.test/")
	}
	// The redirect's query, checked as values so encoding does not matter.
	sentTo := func(want map[string]string) func(testing.TB, *tests.TestApp, *http.Response) {
		return func(t testing.TB, _ *tests.TestApp, res *http.Response) {
			to, err := url.Parse(res.Header.Get("Location"))
			if err != nil {
				t.Fatal(err)
			}
			if to.Scheme+"://"+to.Host+to.Path != "https://app.example.test/auth/callback" {
				t.Fatalf("redirected to %s", to)
			}
			got := to.Query()
			if len(got) != len(want) {
				t.Fatalf("query = %v, want %v", got, want)
			}
			for k, v := range want {
				if got.Get(k) != v {
					t.Fatalf("query %s = %q, want %q", k, got.Get(k), v)
				}
			}
		}
	}

	scenarios := []tests.ApiScenario{
		{
			Name:   "a first sign-in passes on the code, the state and the name",
			Method: http.MethodPost, URL: "/auth/apple/callback", Headers: headers,
			Body: form(map[string]string{
				"code": "c1", "state": "s1",
				"user": `{"name":{"firstName":" Mei Ling ","lastName":"Tan"},"email":"x@privaterelay.appleid.com"}`,
			}),
			TestAppFactory: setupTestApp, BeforeTestFunc: appURL,
			ExpectedStatus: http.StatusSeeOther,
			AfterTestFunc:  sentTo(map[string]string{"code": "c1", "state": "s1", "name": "Mei Ling Tan"}),
		},
		{
			Name:   "a later sign-in carries no name",
			Method: http.MethodPost, URL: "/auth/apple/callback", Headers: headers,
			Body:           form(map[string]string{"code": "c1", "state": "s1"}),
			TestAppFactory: setupTestApp, BeforeTestFunc: appURL,
			ExpectedStatus: http.StatusSeeOther,
			AfterTestFunc:  sentTo(map[string]string{"code": "c1", "state": "s1"}),
		},
		{
			Name:   "a sign-in the person cancelled passes on only the error",
			Method: http.MethodPost, URL: "/auth/apple/callback", Headers: headers,
			Body:           form(map[string]string{"error": "user_cancelled_authorize", "state": "s1", "code": "c1"}),
			TestAppFactory: setupTestApp, BeforeTestFunc: appURL,
			ExpectedStatus: http.StatusSeeOther,
			AfterTestFunc:  sentTo(map[string]string{"error": "user_cancelled_authorize"}),
		},
		{
			Name:   "a reply without a code is an error, not a sign-in",
			Method: http.MethodPost, URL: "/auth/apple/callback", Headers: headers,
			Body:           form(map[string]string{"state": "s1"}),
			TestAppFactory: setupTestApp, BeforeTestFunc: appURL,
			ExpectedStatus: http.StatusSeeOther,
			AfterTestFunc:  sentTo(map[string]string{"error": "invalid_request"}),
		},
		{
			Name:   "without the web app's address there is nowhere to send the person",
			Method: http.MethodPost, URL: "/auth/apple/callback", Headers: headers,
			Body:            form(map[string]string{"code": "c1", "state": "s1"}),
			TestAppFactory:  setupTestApp,
			BeforeTestFunc:  func(t testing.TB, _ *tests.TestApp, _ *core.ServeEvent) { t.Setenv("PB_APP_URL", "") },
			ExpectedStatus:  http.StatusServiceUnavailable,
			ExpectedContent: []string{"Sign in with Apple is not configured"},
		},
	}
	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}
