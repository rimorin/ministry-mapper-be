//go:build testdata

package setup

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ministry-mapper/internal/handlers"

	"ganigeorgiev.com/pocketbase/dbx"
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"ganigeorgiev.com/pocketbase/pocketbase/tests"
	"ganigeorgiev.com/pocketbase/pocketbase/tools/types"
)

const (
	roundTerritory = "testterralpha01"
	roundAdminID   = "testuseralpha01"
)

var roundMaps = []string{"testmapalpha01a", "testmapalpha01b", "testmapalphsc01", "testmapalphcf01"}

type roundFixture struct {
	app   *tests.TestApp
	mux   http.Handler
	token string
}

// newRoundFixture starts testterralpha01 as a territory that predates round
// tracking, with all 15 of its addresses countable so progress moves in
// steps of one address.
func newRoundFixture(t *testing.T) *roundFixture {
	t.Helper()

	token, err := generateToken("admin@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	app := setupTestApp(t)
	t.Cleanup(app.Cleanup)

	f := &roundFixture{app: app, mux: buildTestMux(t, app), token: token}
	f.exec(t, `INSERT INTO address_options (id, address, map, congregation, option, created, updated)
		SELECT 'rnd' || substr(a.id, 4), a.id, a.map, a.congregation, 'testoptialpha01', a.created, a.updated
		FROM addresses a
		WHERE a.territory = {:territory}
		AND NOT EXISTS (SELECT 1 FROM address_options ao WHERE ao.address = a.id)`)
	f.exec(t, "UPDATE territories SET round_started_at = '', last_completed_at = '' WHERE id = {:territory}")
	f.exec(t, "UPDATE maps SET reset_at = '' WHERE territory = {:territory}")
	return f
}

func (f *roundFixture) exec(t *testing.T, query string, params ...dbx.Params) {
	t.Helper()
	bind := dbx.Params{"territory": roundTerritory}
	for _, p := range params {
		for k, v := range p {
			bind[k] = v
		}
	}
	if _, err := f.app.DB().NewQuery(query).Bind(bind).Execute(); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

// setStatus sets the status of every address on the given maps (all of the
// territory's maps when none are given) without firing hooks.
func (f *roundFixture) setStatus(t *testing.T, status string, maps ...string) {
	t.Helper()
	if len(maps) == 0 {
		maps = roundMaps
	}
	for _, mapID := range maps {
		f.exec(t, "UPDATE addresses SET status = {:status} WHERE map = {:map}", dbx.Params{"status": status, "map": mapID})
	}
}

// work records a publisher marking an address on mapID as done at the given time.
func (f *roundFixture) work(t *testing.T, mapID string, at types.DateTime) {
	t.Helper()
	f.logStatus(t, mapID, "not_done", "done", at)
}

func (f *roundFixture) logStatus(t *testing.T, mapID, oldStatus, newStatus string, at types.DateTime) {
	t.Helper()
	collection, err := f.app.FindCollectionByNameOrId("addresses_log")
	if err != nil {
		t.Fatal(err)
	}
	address, err := f.app.FindFirstRecordByFilter("addresses", "map = {:map}", dbx.Params{"map": mapID})
	if err != nil {
		t.Fatal(err)
	}
	record := core.NewRecord(collection)
	record.Set("address", address.Id)
	record.Set("congregation", address.GetString("congregation"))
	record.Set("territory", roundTerritory)
	record.Set("map", mapID)
	record.Set("old_status", oldStatus)
	record.Set("new_status", newStatus)
	record.SetRaw("created", at)
	if err := f.app.SaveNoValidate(record); err != nil {
		t.Fatal(err)
	}
}

// recount recalculates every map and then the territory, the way the address
// hooks do after a publisher's update.
func (f *roundFixture) recount(t *testing.T) {
	t.Helper()
	for _, mapID := range roundMaps {
		if err := handlers.ProcessMapAggregates(mapID, f.app, false); err != nil {
			t.Fatal(err)
		}
	}
	if err := handlers.ProcessTerritoryAggregates(roundTerritory, f.app); err != nil {
		t.Fatal(err)
	}
}

func (f *roundFixture) post(t *testing.T, url, body string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, url, strings.NewReader(body))
	req.Header.Set("content-type", "application/json")
	req.Header.Set("Authorization", f.token)
	recorder := httptest.NewRecorder()
	f.mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("POST %s: status %d: %s", url, recorder.Code, recorder.Body.String())
	}
}

func (f *roundFixture) territory(t *testing.T) *core.Record {
	t.Helper()
	record, err := f.app.FindRecordById("territories", roundTerritory)
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func (f *roundFixture) logRows(t *testing.T, action string) []*core.Record {
	t.Helper()
	rows, err := f.app.FindRecordsByFilter("territories_log",
		"territory = {:territory} && action = {:action}", "created", 0, 0,
		dbx.Params{"territory": roundTerritory, "action": action})
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

// waitForCompletions polls for completions made by the background recheck that
// a congregation settings change starts.
func (f *roundFixture) waitForCompletions(t *testing.T, want int) []*core.Record {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		rows := f.logRows(t, "completed")
		if len(rows) >= want || time.Now().After(deadline) {
			return rows
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func (f *roundFixture) saveCongregation(t *testing.T, field string, value int) {
	t.Helper()
	congregation, err := f.app.FindRecordById("congregations", "testcongalpha01")
	if err != nil {
		t.Fatal(err)
	}
	congregation.Set(field, value)
	if err := f.app.Save(congregation); err != nil {
		t.Fatal(err)
	}
}

// ago returns a time d before now, at the millisecond precision dates are stored with.
func ago(d time.Duration) types.DateTime {
	at, _ := types.ParseDateTime(time.Now().Add(-d).Truncate(time.Millisecond))
	return at
}

func TestTerritoryRound_CompletesAtDefaultThresholdDatedByLastWork(t *testing.T) {
	f := newRoundFixture(t)
	f.setStatus(t, "done")
	f.work(t, "testmapalpha01a", ago(2*time.Hour))
	lastWork := ago(time.Hour)
	f.work(t, "testmapalpha01b", lastWork)
	f.recount(t)

	rows := f.logRows(t, "completed")
	if len(rows) != 1 {
		t.Fatalf("expected 1 completion, got %d", len(rows))
	}
	if got := rows[0].GetDateTime("created"); got.String() != lastWork.String() {
		t.Errorf("completion dated %s, want the last work at %s", got, lastWork)
	}
	if got := f.territory(t).GetDateTime("last_completed_at"); got.String() != lastWork.String() {
		t.Errorf("last_completed_at = %s, want %s", got, lastWork)
	}
	details := map[string]int{}
	if err := rows[0].UnmarshalJSONField("details", &details); err != nil {
		t.Fatal(err)
	}
	if details["threshold"] != handlers.DefaultCompletionThreshold || details["progress"] != 100 {
		t.Errorf("details = %v, want the default threshold and progress 100", details)
	}

	// Work on leftovers after completion belongs to no round.
	f.work(t, "testmapalpha01a", ago(time.Minute))
	f.recount(t)
	if got := len(f.logRows(t, "completed")); got != 1 {
		t.Errorf("work after completion added a completion: got %d", got)
	}
}

func TestTerritoryRound_NeedsRecordedWork(t *testing.T) {
	f := newRoundFixture(t)
	f.setStatus(t, "done")
	f.recount(t)

	if got := len(f.logRows(t, "completed")); got != 0 {
		t.Errorf("completed with no recorded work: got %d completions", got)
	}
}

func TestTerritoryRound_ThresholdChangeRechecksOpenRound(t *testing.T) {
	f := newRoundFixture(t)
	f.setStatus(t, "done")
	// 13 of 15 done: 87%, under the default 90.
	f.exec(t, "UPDATE addresses SET status = 'not_done' WHERE id IN ('testalpha01b001', 'testalpha01b002')")
	lastWork := ago(time.Hour)
	f.work(t, "testmapalpha01a", lastWork)
	f.recount(t)

	if got := f.territory(t).GetInt("progress"); got != 87 {
		t.Fatalf("progress = %d, want 87", got)
	}
	if got := len(f.logRows(t, "completed")); got != 0 {
		t.Fatalf("completed under the threshold: got %d completions", got)
	}

	f.saveCongregation(t, "completion_threshold", 85)

	rows := f.waitForCompletions(t, 1)
	if len(rows) != 1 {
		t.Fatalf("expected the recheck to complete the round, got %d completions", len(rows))
	}
	if got := rows[0].GetDateTime("created"); got.String() != lastWork.String() {
		t.Errorf("completion dated %s, want the last work at %s", got, lastWork)
	}
}

func TestTerritoryRound_MaxTriesChangeRecountsProgress(t *testing.T) {
	f := newRoundFixture(t)
	// Every address is a not-home on its second try; max_tries is 3, so none count.
	f.exec(t, "UPDATE addresses SET status = 'not_home', not_home_tries = 2 WHERE territory = {:territory}")
	lastWork := ago(time.Hour)
	f.logStatus(t, "testmapalpha01a", "not_done", "not_home", lastWork)
	f.recount(t)

	if got := f.territory(t).GetInt("progress"); got != 0 {
		t.Fatalf("progress = %d, want 0", got)
	}

	f.saveCongregation(t, "max_tries", 2)

	rows := f.waitForCompletions(t, 1)
	if len(rows) != 1 {
		t.Fatalf("expected the recount to complete the round, got %d completions", len(rows))
	}
	if got := f.territory(t).GetInt("progress"); got != 100 {
		t.Errorf("progress = %d after lowering max_tries, want 100", got)
	}
	if got := rows[0].GetDateTime("created"); got.String() != lastWork.String() {
		t.Errorf("completion dated %s, want the last work at %s", got, lastWork)
	}
}

func TestTerritoryRound_CompletesTwiceInOneYear(t *testing.T) {
	f := newRoundFixture(t)
	f.setStatus(t, "done")
	f.work(t, "testmapalpha01a", ago(30*24*time.Hour))
	f.recount(t)
	if got := len(f.logRows(t, "completed")); got != 1 {
		t.Fatalf("first round: got %d completions, want 1", got)
	}

	f.post(t, "/territory/reset", `{"territory":"`+roundTerritory+`"}`)

	territory := f.territory(t)
	started := territory.GetDateTime("round_started_at")
	if started.IsZero() {
		t.Fatal("reset did not start a round")
	}
	if got := territory.GetInt("progress"); got != 0 {
		t.Errorf("progress = %d after reset, want 0", got)
	}
	for _, mapID := range roundMaps {
		mapRecord, err := f.app.FindRecordById("maps", mapID)
		if err != nil {
			t.Fatal(err)
		}
		if got := mapRecord.GetDateTime("reset_at"); !got.Equal(started) {
			t.Errorf("map %s reset_at = %s, want the round start %s", mapID, got, started)
		}
	}
	resets := f.logRows(t, "reset")
	if len(resets) != 1 {
		t.Fatalf("expected 1 reset row, got %d", len(resets))
	}
	if got := resets[0].GetString("changed_by"); got != roundAdminID {
		t.Errorf("reset changed_by = %q, want %q", got, roundAdminID)
	}

	f.setStatus(t, "done")
	f.work(t, "testmapalpha01b", ago(0))
	f.recount(t)

	rows := f.logRows(t, "completed")
	if len(rows) != 2 {
		t.Fatalf("second round: got %d completions, want 2", len(rows))
	}
	if rows[1].GetDateTime("created").Before(started) {
		t.Errorf("second completion %s is before its round started at %s", rows[1].GetDateTime("created"), started)
	}
}

func TestTerritoryRound_StaleProgressAfterResetDoesNotComplete(t *testing.T) {
	f := newRoundFixture(t)
	f.setStatus(t, "done")
	f.work(t, "testmapalpha01a", ago(time.Hour))
	f.recount(t)

	f.post(t, "/territory/reset", `{"territory":"`+roundTerritory+`"}`)

	// Counts left over from before the reset, as if the recount had not run yet.
	f.exec(t, `UPDATE maps SET aggregates = '{"completed":5,"total":5}' WHERE territory = {:territory}`)
	if err := handlers.ProcessTerritoryAggregates(roundTerritory, f.app); err != nil {
		t.Fatal(err)
	}

	if got := f.territory(t).GetInt("progress"); got != 100 {
		t.Fatalf("progress = %d, want the stale 100", got)
	}
	if got := len(f.logRows(t, "completed")); got != 1 {
		t.Errorf("stale progress completed the new round: got %d completions, want 1", got)
	}
}

func TestTerritoryRound_MapByMapReset(t *testing.T) {
	f := newRoundFixture(t)
	f.post(t, "/territory/reset", `{"territory":"`+roundTerritory+`"}`)
	firstStart := f.territory(t).GetDateTime("round_started_at")
	f.setStatus(t, "done")
	f.work(t, "testmapalpha01a", ago(0))
	f.recount(t)
	if got := len(f.logRows(t, "completed")); got != 1 {
		t.Fatalf("first round: got %d completions, want 1", got)
	}

	// Resetting one map of a completed round starts the next round.
	time.Sleep(2 * time.Millisecond)
	f.post(t, "/map/reset", `{"map":"testmapalpha01a"}`)

	started := f.territory(t).GetDateTime("round_started_at")
	if !started.After(firstStart) {
		t.Fatalf("map reset after completion did not start a round: start %s, first round %s", started, firstStart)
	}
	mapResets := f.logRows(t, "map_reset")
	if len(mapResets) != 1 {
		t.Fatalf("expected 1 map_reset row, got %d", len(mapResets))
	}
	details := map[string]any{}
	if err := mapResets[0].UnmarshalJSONField("details", &details); err != nil {
		t.Fatal(err)
	}
	if details["map"] != "testmapalpha01a" || details["new_round"] != true {
		t.Errorf("map_reset details = %v, want map testmapalpha01a and new_round true", details)
	}
	untouched, err := f.app.FindRecordById("maps", "testmapalpha01b")
	if err != nil {
		t.Fatal(err)
	}
	if !untouched.GetDateTime("reset_at").Before(started) {
		t.Error("a worked map that was not reset counts as reset in the new round")
	}

	// The reset map is worked again while the others still show last round's
	// work: progress is 100, but the round can't complete on maps not reset yet.
	f.setStatus(t, "done", "testmapalpha01a")
	f.work(t, "testmapalpha01a", ago(0))
	f.recount(t)
	if got := f.territory(t).GetInt("progress"); got != 100 {
		t.Fatalf("progress = %d, want 100", got)
	}
	if got := len(f.logRows(t, "completed")); got != 1 {
		t.Fatalf("completed before every map was reset: got %d completions", got)
	}

	for _, mapID := range roundMaps[1:] {
		f.post(t, "/map/reset", `{"map":"`+mapID+`"}`)
	}
	if got := f.territory(t).GetDateTime("round_started_at"); !got.Equal(started) {
		t.Errorf("resetting maps within an open round moved its start from %s to %s", started, got)
	}

	f.setStatus(t, "done")
	f.work(t, "testmapalpha01b", ago(0))
	f.recount(t)
	if got := len(f.logRows(t, "completed")); got != 2 {
		t.Errorf("expected the round to complete once every map was reset and worked, got %d completions", got)
	}
}

// A territory that predates tracking has no round to extend, so its first map
// reset starts one even though nothing has completed yet.
func TestTerritoryRound_MapResetStartsFirstRound(t *testing.T) {
	f := newRoundFixture(t)
	f.post(t, "/map/reset", `{"map":"testmapalpha01a"}`)

	if f.territory(t).GetDateTime("round_started_at").IsZero() {
		t.Error("map reset on an untracked territory did not start a round")
	}
}

func TestTerritoryRound_NewTerritoryStartsRound(t *testing.T) {
	app := setupTestApp(t)
	defer app.Cleanup()

	collection, err := app.FindCollectionByNameOrId("territories")
	if err != nil {
		t.Fatal(err)
	}
	territory := core.NewRecord(collection)
	territory.Set("congregation", "testcongalpha01")
	territory.Set("code", "T99")
	if err := app.SaveNoValidate(territory); err != nil {
		t.Fatal(err)
	}
	if territory.GetDateTime("round_started_at").IsZero() {
		t.Error("new territory has no round_started_at")
	}
}

func TestTerritoryRound_DeleteIsLogged(t *testing.T) {
	f := newRoundFixture(t)
	f.post(t, "/territory/delete", `{"territory":"`+roundTerritory+`"}`)

	rows := f.logRows(t, "deleted")
	if len(rows) != 1 {
		t.Fatalf("expected 1 deleted row, got %d", len(rows))
	}
	if got := rows[0].GetString("territory_code"); got != "T01" {
		t.Errorf("territory_code = %q, want T01", got)
	}
	if got := rows[0].GetString("changed_by"); got != roundAdminID {
		t.Errorf("changed_by = %q, want %q", got, roundAdminID)
	}
}

func TestTerritoryRound_MovedMapCountsAsReset(t *testing.T) {
	f := newRoundFixture(t)
	f.post(t, "/map/territory/update", `{"map":"testmapalpha01a","new_territory":"testterralpha02"}`)

	mapRecord, err := f.app.FindRecordById("maps", "testmapalpha01a")
	if err != nil {
		t.Fatal(err)
	}
	if mapRecord.GetDateTime("reset_at").IsZero() {
		t.Error("moved map has no reset_at")
	}
}
