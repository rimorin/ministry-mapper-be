//go:build testdata

package migrations

import (
	"testing"

	"ganigeorgiev.com/pocketbase/dbx"
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"ganigeorgiev.com/pocketbase/pocketbase/tests"
)

func TestBackfillTerritoryCompletions(t *testing.T) {
	app, err := tests.NewTestApp("../test_pb_data")
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	exec := func(query string) {
		t.Helper()
		if _, err := app.DB().NewQuery(query).Execute(); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}
	// The seed runs after this migration, so set up what it would have found.
	exec("UPDATE congregations SET completion_threshold = 90")
	exec("UPDATE territories SET round_started_at = '', last_completed_at = ''")
	// alpha01: complete, with work recorded. alpha02: complete, but nothing
	// recorded. beta: worked, but under the threshold.
	exec("UPDATE territories SET progress = 100 WHERE id IN ('testterralpha01', 'testterralpha02')")
	exec("UPDATE territories SET progress = 50 WHERE id = 'testterrbeta001'")

	logs, err := app.FindCollectionByNameOrId("addresses_log")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct{ territory, oldStatus, newStatus, at string }{
		{"testterralpha01", "not_done", "done", "2026-05-01 10:00:00.000Z"},
		{"testterralpha01", "not_home", "not_home", "2026-05-03 10:00:00.000Z"},
		// A reset afterwards is not work, so it must not move the date.
		{"testterralpha01", "done", "not_done", "2026-05-09 10:00:00.000Z"},
		{"testterrbeta001", "not_done", "done", "2026-05-01 10:00:00.000Z"},
	} {
		record := core.NewRecord(logs)
		record.Set("territory", row.territory)
		record.Set("old_status", row.oldStatus)
		record.Set("new_status", row.newStatus)
		record.SetRaw("created", row.at)
		if err := app.SaveNoValidate(record); err != nil {
			t.Fatal(err)
		}
	}

	// Twice, to check it is idempotent.
	for range 2 {
		if err := backfillTerritoryCompletions(app); err != nil {
			t.Fatal(err)
		}
	}

	rows, err := app.FindRecordsByFilter("territories_log", "action = 'completed'", "", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 backfilled completion, got %d", len(rows))
	}
	if got := rows[0].GetString("territory"); got != "testterralpha01" {
		t.Errorf("backfilled %s, want testterralpha01", got)
	}
	if got := rows[0].GetDateTime("created").String(); got != "2026-05-03 10:00:00.000Z" {
		t.Errorf("completion dated %s, want the last work at 2026-05-03 10:00:00.000Z", got)
	}
	details := map[string]any{}
	if err := rows[0].UnmarshalJSONField("details", &details); err != nil {
		t.Fatal(err)
	}
	if details["backfilled"] != true {
		t.Errorf("details = %v, want backfilled true", details)
	}

	var completed string
	if err := app.DB().NewQuery("SELECT last_completed_at FROM territories WHERE id = {:id}").
		Bind(dbx.Params{"id": "testterralpha01"}).Row(&completed); err != nil {
		t.Fatal(err)
	}
	if completed != "2026-05-03 10:00:00.000Z" {
		t.Errorf("last_completed_at = %q, want 2026-05-03 10:00:00.000Z", completed)
	}
}

func TestTerritoryRoundsMigrationIsReversible(t *testing.T) {
	app, err := tests.NewTestApp("../test_pb_data")
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	var migration *core.Migration
	for _, item := range core.AppMigrations.Items() {
		if item.File == "1791882773_territory_rounds.go" {
			migration = item
		}
	}
	if migration == nil {
		t.Fatal("migration not registered")
	}

	hasField := func(collection, field string) bool {
		c, err := app.FindCollectionByNameOrId(collection)
		return err == nil && c.Fields.GetByName(field) != nil
	}

	// Each direction twice, as both must be idempotent.
	for range 2 {
		if err := migration.Down(app); err != nil {
			t.Fatalf("down: %v", err)
		}
	}
	if _, err := app.FindCollectionByNameOrId("territories_log"); err == nil {
		t.Error("territories_log still exists after down")
	}
	if hasField("territories", "round_started_at") || hasField("maps", "reset_at") || hasField("congregations", "completion_threshold") {
		t.Error("round fields still exist after down")
	}

	for range 2 {
		if err := migration.Up(app); err != nil {
			t.Fatalf("up: %v", err)
		}
	}
	if !hasField("territories", "last_completed_at") || !hasField("maps", "reset_at") || !hasField("territories_log", "details") {
		t.Error("round fields missing after up")
	}
	var threshold int
	if err := app.DB().NewQuery("SELECT completion_threshold FROM congregations WHERE id = 'testcongalpha01'").Row(&threshold); err != nil {
		t.Fatal(err)
	}
	if threshold != 90 {
		t.Errorf("completion_threshold = %d after up, want 90", threshold)
	}
}
