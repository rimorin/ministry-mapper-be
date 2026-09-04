//go:build testdata

package jobs

import (
	"testing"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// seedStatusChange inserts an addresses_log row for the given address, dated
// daysAgo days before now. The seed has no log rows, so the activity figures
// below are entirely under this test's control.
func seedStatusChange(t testing.TB, app core.App, addressID, newStatus string, daysAgo int) {
	t.Helper()

	addr, err := app.FindRecordById("addresses", addressID)
	if err != nil {
		t.Fatal(err)
	}
	col, err := app.FindCollectionByNameOrId("addresses_log")
	if err != nil {
		t.Fatal(err)
	}

	logRecord := core.NewRecord(col)
	logRecord.Set("address", addr.Id)
	logRecord.Set("congregation", addr.Get("congregation"))
	logRecord.Set("territory", addr.Get("territory"))
	logRecord.Set("map", addr.Get("map"))
	logRecord.Set("old_status", "not_done")
	logRecord.Set("new_status", newStatus)
	if err := app.SaveNoValidate(logRecord); err != nil {
		t.Fatal(err)
	}

	if daysAgo > 0 {
		created := time.Now().UTC().AddDate(0, 0, -daysAgo).Format("2006-01-02 15:04:05.000Z")
		_, err := app.DB().NewQuery("UPDATE addresses_log SET created = {:created} WHERE id = {:id}").
			Bind(dbx.Params{"created": created, "id": logRecord.Id}).Execute()
		if err != nil {
			t.Fatal(err)
		}
	}
}

// TestBuildSummaryData_SeedData pins the figures BuildSummaryData derives for the
// alpha congregation. Every number below is checked against the seed in
// 1780000000_seed_test_data.go, so a change to the analytics views or to the
// summary queries that alters any of them fails here.
func TestBuildSummaryData_SeedData(t *testing.T) {
	app, err := tests.NewTestApp("../../test_pb_data")
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	// In the 30-day window: T01 gets two done and one not_home, T02 one do_not_call.
	seedStatusChange(t, app, "testalpha01a001", "done", 0)
	seedStatusChange(t, app, "testalpha01a002", "done", 3)
	seedStatusChange(t, app, "testalpha01a003", "not_home", 10)
	seedStatusChange(t, app, "testalpha02a001", "do_not_call", 29)
	// Outside the window and in another congregation: both must be ignored.
	seedStatusChange(t, app, "testalpha01a004", "done", 45)
	seedStatusChange(t, app, "testbeta001a001", "done", 1)

	congregation, err := app.FindRecordById("congregations", "testcongalpha01")
	if err != nil {
		t.Fatal(err)
	}

	data, err := BuildSummaryData(app, congregation, RollingDays(OnDemandReportDays))
	if err != nil {
		t.Fatal(err)
	}

	if data.Available {
		t.Error("Available must stay false until the LLM call succeeds")
	}
	if data.CongregationName != "Alpha Congregation" {
		t.Errorf("CongregationName = %q; want %q", data.CongregationName, "Alpha Congregation")
	}

	// --- Activity (from addresses_log) ---
	wantActivity := map[string]int{"done": 2, "not_home": 1, "do_not_call": 1}
	if len(data.Activity) != len(wantActivity) {
		t.Fatalf("Activity = %+v; want %d statuses", data.Activity, len(wantActivity))
	}
	for _, a := range data.Activity {
		if want, ok := wantActivity[a.Status]; !ok || a.Count != want {
			t.Errorf("Activity[%s] = %d; want %d", a.Status, a.Count, want)
		}
	}
	if data.HouseholdsReached != 2 {
		t.Errorf("HouseholdsReached = %d; want 2", data.HouseholdsReached)
	}
	if data.Visits != 4 {
		t.Errorf("Visits = %d; want 4", data.Visits)
	}
	if data.ActiveTerritories != 2 {
		t.Errorf("ActiveTerritories = %d; want 2", data.ActiveTerritories)
	}

	wantMonthly := []TerritoryMonthlyActivity{
		{TerritoryCode: "T01", Done: 2, NotHome: 1, DNC: 0},
		{TerritoryCode: "T02", Done: 0, NotHome: 0, DNC: 1},
	}
	if len(data.MonthlyByTerritory) != len(wantMonthly) {
		t.Fatalf("MonthlyByTerritory = %+v; want %+v", data.MonthlyByTerritory, wantMonthly)
	}
	for i, want := range wantMonthly {
		if data.MonthlyByTerritory[i] != want {
			t.Errorf("MonthlyByTerritory[%d] = %+v; want %+v", i, data.MonthlyByTerritory[i], want)
		}
	}

	// --- Territory snapshot (from addresses) ---
	if len(data.Territories) != 2 {
		t.Fatalf("Territories = %+v; want 2 rows", data.Territories)
	}
	t01, t02 := data.Territories[0], data.Territories[1]
	wantT01 := TerritoryProgress{Id: "testterralpha01", Code: "T01", Description: "Alpha Territory 01",
		Progress: 0, Total: 15, Done: 1, NotDone: 12, NotHome: 2, DNC: 0, Invalid: 0, IsComplete: false}
	if t01 != wantT01 {
		t.Errorf("Territories[0] = %+v; want %+v", t01, wantT01)
	}
	// T02's progress is recomputed asynchronously from map aggregates while the
	// seed runs, so only the address counts are pinned here.
	if t02.Code != "T02" || t02.Total != 12 || t02.Done != 1 || t02.NotDone != 8 || t02.NotHome != 2 || t02.DNC != 0 || t02.Invalid != 0 {
		t.Errorf("Territories[1] = %+v; want T02 with total=12 done=1 not_done=8 not_home=2 dnc=0 invalid=0", t02)
	}

	// --- Not-home fatigue (max_tries = 3 for the alpha congregation) ---
	wantFatigue := []NotHomeFatigue{
		{TerritoryCode: "T01", MaxedOut: 0, Retrying: 2, Stale: 0, MaxedOutPct: 0},
		{TerritoryCode: "T02", MaxedOut: 1, Retrying: 1, Stale: 0, MaxedOutPct: 50},
	}
	if len(data.NotHomeFatigue) != len(wantFatigue) {
		t.Fatalf("NotHomeFatigue = %+v; want %+v", data.NotHomeFatigue, wantFatigue)
	}
	for i, want := range wantFatigue {
		if data.NotHomeFatigue[i] != want {
			t.Errorf("NotHomeFatigue[%d] = %+v; want %+v", i, data.NotHomeFatigue[i], want)
		}
	}

	// --- Map health (from maps.aggregates) ---
	if len(data.StalledMaps) != 0 {
		t.Errorf("StalledMaps = %+v; want none (no seeded map has progress 0 with not_done > 0 in aggregates)", data.StalledMaps)
	}
	if len(data.CompletedMaps) != 1 || data.CompletedMaps[0].MapCode != "R" || data.CompletedMaps[0].TerritoryCode != "T02" {
		t.Errorf("CompletedMaps = %+v; want exactly the rich map R in T02", data.CompletedMaps)
	}
	if len(data.HighDNCMaps) != 0 {
		t.Errorf("HighDNCMaps = %+v; want none", data.HighDNCMaps)
	}
}
