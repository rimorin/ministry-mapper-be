//go:build testdata

package migrations

import (
	"testing"

	"ganigeorgiev.com/pocketbase/pocketbase/tests"
)

func TestAssignmentEmailsAreOptIn(t *testing.T) {
	app, err := tests.NewTestApp("../test_pb_data")
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	if users.Fields.GetByName("assignment_emails") == nil {
		t.Error("users have no assignment_emails switch")
	}
	if users.Fields.GetByName("assignment_emails_off") != nil {
		t.Error("the old assignment_emails_off switch is still there")
	}

	user, err := app.FindRecordById("users", "testuseralpha04")
	if err != nil {
		t.Fatal(err)
	}
	if user.GetBool("assignment_emails") {
		t.Error("an existing account starts with the emails on")
	}
}
