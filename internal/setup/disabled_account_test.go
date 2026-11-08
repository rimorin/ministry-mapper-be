//go:build testdata

package setup

import (
	"net/http"
	"strings"
	"testing"

	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"ganigeorgiev.com/pocketbase/pocketbase/tests"
)

func disable(t testing.TB, app core.App, email string) {
	t.Helper()
	user, err := app.FindAuthRecordByEmail("users", email)
	if err != nil {
		t.Fatal(err)
	}
	user.Set("disabled", true)
	// The inactivity job saves without validation; the hook must fire either way.
	if err := app.SaveNoValidate(user); err != nil {
		t.Fatal(err)
	}
}

func signIn(email string) string {
	return `{"identity":"` + email + `","password":"Test1234!"}`
}

func TestDisabledAccount_CannotSignIn(t *testing.T) {
	scenarios := []tests.ApiScenario{
		{
			Name:            "an enabled account signs in",
			Method:          http.MethodPost,
			URL:             "/api/collections/users/auth-with-password",
			Body:            strings.NewReader(signIn("readonly@alpha.test")),
			Headers:         map[string]string{"Content-Type": "application/json"},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"token":`},
		},
		{
			Name:    "a disabled account is refused",
			Method:  http.MethodPost,
			URL:     "/api/collections/users/auth-with-password",
			Body:    strings.NewReader(signIn("readonly@alpha.test")),
			Headers: map[string]string{"Content-Type": "application/json"},
			TestAppFactory: func(t testing.TB) *tests.TestApp {
				app := setupTestApp(t)
				disable(t, app, "readonly@alpha.test")
				return app
			},
			ExpectedStatus:     403,
			ExpectedContent:    []string{"doesn't satisfy the collection requirements to authenticate"},
			NotExpectedContent: []string{`"token":`},
		},
	}
	for _, s := range scenarios {
		s.Test(t)
	}
}

func TestDisabledAccount_LosesItsOpenSessions(t *testing.T) {
	app := setupTestApp(t)
	defer app.Cleanup()

	user, err := app.FindAuthRecordByEmail("users", "readonly@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	token, err := user.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.FindAuthRecordByToken(token, core.TokenTypeAuth); err != nil {
		t.Fatalf("the token must work before the account is disabled: %v", err)
	}

	// Saving without a change to disabled leaves sessions alone.
	user.Set("name", "Alpha ReadOnly")
	if err := app.Save(user); err != nil {
		t.Fatal(err)
	}
	if _, err := app.FindAuthRecordByToken(token, core.TokenTypeAuth); err != nil {
		t.Fatalf("an unrelated save must not sign the user out: %v", err)
	}

	disable(t, app, "readonly@alpha.test")
	if _, err := app.FindAuthRecordByToken(token, core.TokenTypeAuth); err == nil {
		t.Fatal("a token issued before the account was disabled must stop working")
	}
}
