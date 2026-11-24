//go:build testdata

package migrations

import (
	"testing"

	"ganigeorgiev.com/pocketbase/pocketbase/tests"
)

func TestUsersSignInLength(t *testing.T) {
	app, err := tests.NewTestApp("../test_pb_data")
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	duration := func() int64 {
		t.Helper()
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			t.Fatal(err)
		}
		return users.AuthToken.Duration
	}
	longer := func(current int64) bool { return current < signInSeconds }

	// The migration has run on the seeded database.
	if got := duration(); got != signInSeconds {
		t.Fatalf("after migrating, sign-in lasts %d s, want %d", got, signInSeconds)
	}

	// A shorter value is raised.
	if err := setUsersSignInLength(app, previousSignInSeconds, func(int64) bool { return true }); err != nil {
		t.Fatal(err)
	}
	if err := setUsersSignInLength(app, signInSeconds, longer); err != nil {
		t.Fatal(err)
	}
	if got := duration(); got != signInSeconds {
		t.Errorf("a 14-day sign-in became %d s, want %d", got, signInSeconds)
	}

	// A longer one, set by hand, is kept.
	const byHand = 90 * 24 * 60 * 60
	if err := setUsersSignInLength(app, byHand, func(int64) bool { return true }); err != nil {
		t.Fatal(err)
	}
	if err := setUsersSignInLength(app, signInSeconds, longer); err != nil {
		t.Fatal(err)
	}
	if got := duration(); got != byHand {
		t.Errorf("a 90-day sign-in became %d s, want it kept", got)
	}
}
