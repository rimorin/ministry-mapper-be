//go:build testdata

package migrations

import (
	"testing"

	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"ganigeorgiev.com/pocketbase/pocketbase/tests"
)

func TestLogUserEmailsMigration(t *testing.T) {
	app, err := tests.NewTestApp("../test_pb_data")
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	var migration *core.Migration
	for _, item := range core.AppMigrations.Items() {
		if item.File == "1793628820_log_user_emails.go" {
			migration = item
		}
	}
	if migration == nil {
		t.Fatal("migration not registered")
	}

	// A known user, and a user deleted before the migration.
	ids := map[string]string{"testlogknown001": "testuseralpha01", "testloggone0001": "testuserdeleted"}
	want := map[string]string{"testlogknown001": "admin@alpha.test", "testloggone0001": ""}

	// Each direction twice, as both must be idempotent.
	for range 2 {
		if err := migration.Down(app); err != nil {
			t.Fatalf("down: %v", err)
		}
	}
	for table, fields := range logEmailFields {
		collection, err := app.FindCollectionByNameOrId(table)
		if err != nil {
			t.Fatal(err)
		}
		for _, field := range fields {
			if collection.Fields.GetByName(field+"_email") != nil {
				t.Errorf("%s.%s_email still exists after down", table, field)
			}
		}
		for row, user := range ids {
			record := core.NewRecord(collection)
			record.Id = row
			for _, field := range fields {
				record.Set(field, user)
			}
			if err := app.SaveNoValidate(record); err != nil {
				t.Fatalf("%s: %v", table, err)
			}
		}
	}

	for range 2 {
		if err := migration.Up(app); err != nil {
			t.Fatalf("up: %v", err)
		}
	}
	for table, fields := range logEmailFields {
		for row, email := range want {
			record, err := app.FindRecordById(table, row)
			if err != nil {
				t.Fatalf("%s %s: %v", table, row, err)
			}
			for _, field := range fields {
				if got := record.GetString(field + "_email"); got != email {
					t.Errorf("%s %s: %s_email = %q, want %q", table, row, field, got, email)
				}
			}
		}
	}
}
