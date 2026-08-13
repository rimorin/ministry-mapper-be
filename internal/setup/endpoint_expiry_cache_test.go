//go:build testdata

package setup

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// congregationExpiryCache is a package-level cache in the handlers package, so it
// survives across the separate test apps these scenarios create. That is exactly
// the condition the bug needed: without the invalidation hook, an admin's change
// to expiry_hours would go unnoticed until the process restarted.
func TestCongregationExpiryCacheInvalidation(t *testing.T) {
	conductorToken, err := generateToken("conductor@alpha.test")
	if err != nil {
		t.Fatal(err)
	}

	body := `{"territory":"testterralpha01","coordinates":{"lat":1.3521,"lng":103.8198},"publisher":"Test Publisher"}`

	// Reads the assignment created by the request and returns how many hours from
	// now it expires, rounded to the nearest hour.
	expiryHoursOf := func(t testing.TB, app *tests.TestApp, res *http.Response) float64 {
		raw, _ := io.ReadAll(res.Body)
		var out struct {
			LinkID string `json:"linkId"`
		}
		if err := json.Unmarshal(raw, &out); err != nil || out.LinkID == "" {
			t.Fatalf("no linkId in response: %v body=%s", err, raw)
		}
		assignment, err := app.FindRecordById("assignments", out.LinkID)
		if err != nil {
			t.Fatal(err)
		}
		return assignment.GetDateTime("expiry_date").Time().Sub(time.Now().UTC()).Hours()
	}

	scenarios := []tests.ApiScenario{
		{
			// Primes the cache with the seeded expiry_hours of 24.
			Name:   "first quicklink caches the congregation's expiry_hours",
			Method: http.MethodPost,
			URL:    "/territory/link",
			Body:   strings.NewReader(body),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"linkId"`},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				if hours := expiryHoursOf(t, app, res); hours < 23 || hours > 24.5 {
					t.Errorf("expected ~24h expiry from seed, got %.2fh", hours)
				}
			},
		},
		{
			Name:   "changing expiry_hours takes effect without a restart",
			Method: http.MethodPost,
			URL:    "/territory/link",
			Body:   strings.NewReader(body),
			Headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": conductorToken,
			},
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				cong, err := app.FindRecordById("congregations", "testcongalpha01")
				if err != nil {
					t.Fatal(err)
				}
				cong.Set("expiry_hours", 2)
				// Saving fires OnRecordAfterUpdateSuccess, which busts the cache.
				if err := app.Save(cong); err != nil {
					t.Fatal(err)
				}
			},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"linkId"`},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				hours := expiryHoursOf(t, app, res)
				if hours > 2.5 {
					t.Errorf("expected ~2h expiry after the update, got %.2fh — stale cache", hours)
				}
				if hours < 1.5 {
					t.Errorf("expected ~2h expiry after the update, got %.2fh", hours)
				}
			},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}
