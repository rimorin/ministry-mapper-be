//go:build testdata

package assistant

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"ganigeorgiev.com/pocketbase/dbx"
	"ganigeorgiev.com/pocketbase/pocketbase/tests"
)

const (
	alphaCong = "testcongalpha01"
	betaCong  = "testcongbeta001"
)

// testNow is Friday 6 November 2026, 12:00 in Singapore, where both seed
// congregations are.
var testNow = time.Date(2026, 11, 6, 4, 0, 0, 0, time.UTC)

func newTestApp(t testing.TB) *tests.TestApp {
	t.Helper()
	app, err := tests.NewTestApp("../../test_pb_data")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)
	return app
}

func scopeFor(t testing.TB, app *tests.TestApp, congregation string) Scope {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Singapore")
	if err != nil {
		t.Fatal(err)
	}
	// Administrators may use every function; tests of the role policy set
	// another role themselves.
	return Scope{App: app, CongregationID: congregation, Location: loc, Now: testNow, Role: roleAdministrator}
}

func exec(t testing.TB, app *tests.TestApp, query string, params dbx.Params) {
	t.Helper()
	if _, err := app.DB().NewQuery(query).Bind(params).Execute(); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

func run(t testing.TB, s Scope, tool Tool, args string) Result {
	t.Helper()
	r, err := tool.Run(s, json.RawMessage(args))
	if err != nil {
		t.Fatalf("%s(%s): %v", tool.Name, args, err)
	}
	return r
}

func column(r Result, key string) []any {
	out := make([]any, len(r.Rows))
	for i, row := range r.Rows {
		out[i] = row[key]
	}
	return out
}

func logStatus(t testing.TB, app *tests.TestApp, id, address, status, by string, at time.Time) {
	t.Helper()
	exec(t, app, `INSERT INTO addresses_log (id, address, congregation, territory, map, old_status, new_status, changed_by, created, updated)
		SELECT {:id}, a.id, a.congregation, a.territory, a.map, 'not_done', {:status}, {:by}, {:at}, {:at}
		FROM addresses a WHERE a.id = {:address}`,
		dbx.Params{"id": id, "address": address, "status": status, "by": by, "at": dbTime(at)})
}

func logAssigned(t testing.TB, app *tests.TestApp, id, mapID, publisher, kind string, at time.Time) {
	t.Helper()
	exec(t, app, `INSERT INTO assignments_log (id, congregation, map, publisher, type, action, created, updated)
		SELECT {:id}, m.congregation, m.id, {:publisher}, {:type}, 'assigned', {:at}, {:at}
		FROM maps m WHERE m.id = {:map}`,
		dbx.Params{"id": id, "map": mapID, "publisher": publisher, "type": kind, "at": dbTime(at)})
}

func addMalay(t testing.TB, app *tests.TestApp, addresses ...string) {
	t.Helper()
	exec(t, app, `INSERT INTO options (id, congregation, code, description, is_countable, is_default, sequence)
		VALUES ('testoptimalay01', {:cong}, 'MY', 'Malay', TRUE, FALSE, 9)`, dbx.Params{"cong": alphaCong})
	for i, a := range addresses {
		exec(t, app, `INSERT INTO address_options (id, address, option, congregation, map)
			SELECT {:id}, a.id, 'testoptimalay01', a.congregation, a.map FROM addresses a WHERE a.id = {:address}`,
			dbx.Params{"id": "testaomalay00" + string(rune('1'+i)), "address": a})
	}
}

func TestTerritoryOverview_NotCompletedWithin(t *testing.T) {
	app := newTestApp(t)
	exec(t, app, `UPDATE territories SET last_completed_at = {:old}, round_started_at = {:started} WHERE id = 'testterralpha01'`,
		dbx.Params{"old": dbTime(testNow.AddDate(0, 0, -200)), "started": dbTime(testNow.AddDate(0, 0, -100))})
	exec(t, app, `UPDATE territories SET last_completed_at = {:recent} WHERE id = 'testterralpha02'`,
		dbx.Params{"recent": dbTime(testNow.AddDate(0, 0, -10))})

	r := run(t, scopeFor(t, app, alphaCong), territoryOverview, `{"not_completed_within_days": 180}`)
	if got := column(r, "territory"); len(got) != 1 || got[0] != "T01" {
		t.Fatalf("territories = %v, want only T01", got)
	}
	row := r.Rows[0]
	if row["last_completed"] != "2026-04-20" || row["round_open_since"] != "2026-07-29" {
		t.Errorf("last_completed = %v, round_open_since = %v", row["last_completed"], row["round_open_since"])
	}
	if r.Note == "" {
		t.Error("completion dates must carry the tracking note")
	}
}

func TestTerritoryOverview_UnknownCodeIsArgError(t *testing.T) {
	app := newTestApp(t)
	_, err := territoryOverview.Run(scopeFor(t, app, alphaCong), json.RawMessage(`{"territory": "ZZ9"}`))
	var bad argError
	if !errors.As(err, &bad) || !strings.Contains(bad.msg, "find_places") {
		t.Fatalf("err = %v, want an argument error pointing at find_places", err)
	}
}

func TestMapOverview_CountsAndUnassigned(t *testing.T) {
	app := newTestApp(t)
	exec(t, app, `UPDATE maps SET progress = 20, aggregates = '{"total":5,"done":1,"notHome":2,"notDone":2,"dnc":0,"invalid":0,"completed":1}'
		WHERE id = 'testmapalpha01a'`, nil)
	s := scopeFor(t, app, alphaCong)

	all := run(t, s, mapOverview, `{"territory": "t01"}`)
	var blk100a map[string]any
	for _, row := range all.Rows {
		if row["map_id"] == "testmapalpha01a" {
			blk100a = row
		}
	}
	if blk100a == nil || blk100a["homes"] != 5 || blk100a["progress"] != 20 || blk100a["active_links"] != 1 {
		t.Fatalf("Blk 100A = %v", blk100a)
	}

	free := run(t, s, mapOverview, `{"unassigned_only": true}`)
	for _, row := range free.Rows {
		if row["map_id"] == "testmapalpha01a" {
			t.Fatal("a map with an active link is not free to assign")
		}
	}
	if free.Total == 0 {
		t.Fatal("expected other maps to be free")
	}
}

func TestActivity_CountsInCongregationTimeZone(t *testing.T) {
	app := newTestApp(t)
	logStatus(t, app, "testlogact00001", "testalpha01a001", "done", "Bro Tan", time.Date(2026, 11, 5, 17, 30, 0, 0, time.UTC))      // 6 Nov 01:30 SGT
	logStatus(t, app, "testlogact00002", "testalpha01a002", "not_home", "Bro Tan", time.Date(2026, 11, 5, 15, 0, 0, 0, time.UTC))   // 5 Nov 23:00 SGT
	logStatus(t, app, "testlogact00003", "testalpha02a001", "do_not_call", "Sis Lim", time.Date(2026, 11, 6, 3, 0, 0, 0, time.UTC)) // 6 Nov 11:00 SGT
	logStatus(t, app, "testlogact00004", "testalpha02a002", "invalid", "Sis Lim", time.Date(2026, 11, 6, 3, 30, 0, 0, time.UTC))
	logStatus(t, app, "testlogact00005", "testbeta001a001", "done", "Beta Person", time.Date(2026, 11, 6, 3, 0, 0, 0, time.UTC))
	s := scopeFor(t, app, alphaCong)

	total := run(t, s, activity, `{"from": "2026-11-06", "to": "2026-11-06", "group_by": "none"}`)
	want := map[string]any{"visits": 2, "reached": 1, "nobody_home": 0, "do_not_call": 1, "invalid": 1}
	for k, v := range want {
		if total.Rows[0][k] != v {
			t.Errorf("%s = %v, want %v", k, total.Rows[0][k], v)
		}
	}

	byTerritory := run(t, s, activity, `{"from": "2026-11-05", "to": "2026-11-06", "group_by": "territory"}`)
	if got := column(byTerritory, "territory"); len(got) != 2 || got[0] != "T01" {
		t.Errorf("territories = %v, want T01 (2 visits) before T02 (1 visit)", got)
	}

	if _, err := activity.Run(s, json.RawMessage(`{"from": "2026-11-06", "to": "2026-11-01", "group_by": "none"}`)); err == nil {
		t.Error("a period ending before it starts must be refused")
	}
}

func TestActivity_TrendsByWeekAndMonthListQuietOnesToo(t *testing.T) {
	app := newTestApp(t)
	logStatus(t, app, "testlogtrend001", "testalpha01a001", "done", "Bro Tan", sgt(time.October, 30))     // Friday
	logStatus(t, app, "testlogtrend002", "testalpha01a002", "not_home", "Bro Tan", sgt(time.November, 2)) // Monday
	logStatus(t, app, "testlogtrend003", "testalpha02a001", "done", "Sis Lim", sgt(time.November, 5))
	s := scopeFor(t, app, alphaCong)

	weeks := run(t, s, activity, `{"from": "2026-10-19", "to": "2026-11-06", "group_by": "week"}`)
	if got := column(weeks, "week"); !reflect.DeepEqual(got, []any{"2026-10-19", "2026-10-26", "2026-11-02"}) {
		t.Fatalf("weeks = %v, want every Monday in the period", got)
	}
	if got := column(weeks, "visits"); !reflect.DeepEqual(got, []any{0, 1, 2}) {
		t.Errorf("visits by week = %v", got)
	}

	months := run(t, s, activity, `{"from": "2026-09-01", "to": "2026-11-06", "group_by": "month"}`)
	if got := column(months, "month"); !reflect.DeepEqual(got, []any{"2026-09", "2026-10", "2026-11"}) {
		t.Fatalf("months = %v", got)
	}
	if got := column(months, "visits"); !reflect.DeepEqual(got, []any{0, 1, 2}) {
		t.Errorf("visits by month = %v", got)
	}
}

func TestWhoWorked_MergesLinksAndVisits(t *testing.T) {
	app := newTestApp(t)
	day := time.Date(2026, 11, 6, 1, 0, 0, 0, time.UTC)
	logAssigned(t, app, "testasglog00001", "testmapalpha01a", "Bro Tan", "normal", day)
	logAssigned(t, app, "testasglog00002", "testmapalpha01b", " bro tan ", "personal", day)
	logAssigned(t, app, "testasglog00003", "testmapalpha02a", "Sis Lim", "normal", day)
	logAssigned(t, app, "testasglog00004", "testmapalpha02a", "Earlier Person", "normal", day.AddDate(0, 0, -3))
	logStatus(t, app, "testlogwho00001", "testalpha01a001", "done", "Bro Tan", day)
	logStatus(t, app, "testlogwho00002", "testalpha01a002", "not_home", "Bro Tan", day)
	logStatus(t, app, "testlogwho00003", "testalpha02a001", "done", "Alpha Admin", day)

	r := run(t, scopeFor(t, app, alphaCong), whoWorked, `{"from": "2026-11-06", "to": "2026-11-06"}`)
	if got := column(r, "name"); len(got) != 3 || got[0] != "Alpha Admin" || got[1] != "Bro Tan" || got[2] != "Sis Lim" {
		t.Fatalf("names = %v, want alphabetical Alpha Admin, Bro Tan, Sis Lim", got)
	}
	tan := r.Rows[1]
	if tan["links_given"] != 2 || tan["visits_recorded"] != 2 {
		t.Errorf("Bro Tan = %v", tan)
	}
	if !strings.Contains(tan["maps"].(string), "Blk 100A") || !strings.Contains(tan["maps"].(string), "Blk 100B") {
		t.Errorf("maps = %v", tan["maps"])
	}
}

func TestPeopleAndRoles_FiltersByRole(t *testing.T) {
	app := newTestApp(t)
	s := scopeFor(t, app, alphaCong)
	admins := run(t, s, peopleAndRoles, `{"role": "administrator"}`)
	if got := column(admins, "name"); len(got) != 1 || got[0] != "Alpha Admin" {
		t.Fatalf("admins = %v", got)
	}
	everyone := run(t, s, peopleAndRoles, `{}`)
	if got := column(everyone, "role"); len(got) != 3 || got[0] != "administrator" || got[2] != "read_only" {
		t.Fatalf("roles = %v", got)
	}
}

func TestPeopleAndRoles_FindsAMaskedName(t *testing.T) {
	app := newTestApp(t)
	s := scopeFor(t, app, alphaCong)
	// The admin typed the name; the model only ever saw its token.
	token := newPseudonyms(alphaCong).token("Alpha Conductor")
	r := run(t, s, peopleAndRoles, `{"name": "`+token+`"}`)
	if got := column(r, "name"); len(got) != 1 || got[0] != "Alpha Conductor" {
		t.Fatalf("names = %v; the token must search for the name it stands for", got)
	}
}

func TestHomes_MapWithMostOfAType(t *testing.T) {
	app := newTestApp(t)
	addMalay(t, app, "testalpha01a001", "testalpha01a002", "testalpha01b001")
	s := scopeFor(t, app, alphaCong)

	r := run(t, s, homesTool, `{"household_type": "malay", "group_by": "map"}`)
	if len(r.Rows) != 2 || r.Rows[0]["map"] != "Blk 100A" || r.Rows[0]["all_homes"] != 2 || r.Rows[0]["homes"] != 2 {
		t.Fatalf("rows = %v", r.Rows)
	}
	byType := run(t, s, homesTool, `{"household_type": "MY", "group_by": "household_type"}`)
	if len(byType.Rows) != 1 || byType.Rows[0]["household_type"] != "Malay" || byType.Rows[0]["all_homes"] != 3 {
		t.Fatalf("by type = %v", byType.Rows)
	}

	_, err := homesTool.Run(s, json.RawMessage(`{"household_type": "Klingon", "group_by": "map"}`))
	wantArgError(t, err, "Malay")
}

// Blk 100A: 10 and 11 not done, 12 and 13 nobody home, 14 done. Only 12 and
// 13 have a countable household type.
func TestHomes_ListsAndCountsOneMap(t *testing.T) {
	app := newTestApp(t)
	exec(t, app, `UPDATE addresses SET not_home_tries = 3 WHERE id = 'testalpha01a003'`, nil)
	exec(t, app, `UPDATE addresses SET notes = 'Dog' WHERE id = 'testalpha01a001'`, nil)
	s := scopeFor(t, app, alphaCong)

	notDone := run(t, s, homesTool, `{"map": "Blk 100A", "status": ["not_done"], "group_by": "home"}`)
	if notDone.Total != 2 || notDone.Rows[0]["home"] != "10" || notDone.Rows[0]["counted"] != false ||
		notDone.Rows[0]["has_notes"] != true || notDone.Rows[1]["home"] != "11" {
		t.Fatalf("not done = %v", notDone.Rows)
	}
	one := run(t, s, homesTool, `{"map": "Blk 100A", "unit": "12", "group_by": "home"}`)
	if len(one.Rows) != 1 || one.Rows[0]["status"] != "not_home" || one.Rows[0]["tries"] != 3 ||
		one.Rows[0]["at_try_limit"] != true || !strings.Contains(one.Note, "try limit is 3") {
		t.Fatalf("one home = %v, note %q", one.Rows, one.Note)
	}
	first := run(t, s, homesTool, `{"map": "Blk 100A", "group_by": "home", "limit": 1}`)
	if len(first.Rows) != 1 || first.Total != 5 {
		t.Fatalf("limit: %d rows of %d", len(first.Rows), first.Total)
	}

	// The same counts map_details gives, so the two never disagree.
	total := run(t, s, homesTool, `{"map": "Blk 100A", "group_by": "total"}`).Rows[0]
	details := run(t, s, mapDetails, `{"map": "Blk 100A"}`).Rows[0]
	for _, column := range homeCounts {
		if total[column] != details[column] {
			t.Errorf("%s: homes says %v, map_details says %v", column, total[column], details[column])
		}
	}
	if total["homes"] != 2 || total["not_home"] != 2 || total["at_try_limit"] != 1 || total["not_counted"] != 3 || total["all_homes"] != 5 {
		t.Fatalf("total = %v", total)
	}
	left := run(t, s, homesTool, `{"map": "Blk 100A", "counted": false, "group_by": "territory"}`)
	if len(left.Rows) != 1 || left.Rows[0]["territory"] != "T01" || left.Rows[0]["all_homes"] != 3 {
		t.Fatalf("not counted = %v", left.Rows)
	}
	_, err := homesTool.Run(s, json.RawMessage(`{"group_by": "floor"}`))
	wantArgError(t, err, "needs one map")
}

func TestCongregationSettings(t *testing.T) {
	app := newTestApp(t)
	r := run(t, scopeFor(t, app, alphaCong), congregationSettings, `{}`)
	row := r.Rows[0]
	if row["max_nobody_home_tries"] != 3 || row["time_zone"] != "Asia/Singapore" || row["completion_threshold"].(int) <= 0 {
		t.Fatalf("settings = %v", row)
	}
}

func TestFindPlaces_MatchesWithinCongregation(t *testing.T) {
	app := newTestApp(t)
	r := run(t, scopeFor(t, app, alphaCong), findPlaces, `{"text": "blk 100"}`)
	if got := column(r, "map"); len(got) != 2 {
		t.Fatalf("maps = %v, want Blk 100A and Blk 100B", got)
	}
	exact := run(t, scopeFor(t, app, alphaCong), findPlaces, `{"text": "T01"}`)
	if exact.Rows[0]["kind"] != "territory" || exact.Rows[0]["territory"] != "T01" {
		t.Fatalf("the exact territory must come first, got %v", exact.Rows[0])
	}
}

func TestFindPlaces_ForgivesHowPlacesAreWritten(t *testing.T) {
	app := newTestApp(t)
	exec(t, app, `UPDATE maps SET description = '5, Marsiling Drive' WHERE id = 'testmapalpha01a'`, nil)
	s := scopeFor(t, app, alphaCong)
	for text, want := range map[string]string{
		"5 Marsiling Dr":     "5, Marsiling Drive", // abbreviation, no comma
		"marsiling drive 5":  "5, Marsiling Drive", // any word order
		"5 Marsiling Dr T01": "5, Marsiling Drive", // with the territory code
		"block 100b":         "Blk 100B",           // abbreviation the other way
		"rihc map":           "Rich Map",           // a typo, offered as a near miss
	} {
		r := run(t, s, findPlaces, `{"text": "`+text+`"}`)
		if got := column(r, "map"); len(got) == 0 || resolveLocalized(got[0].(string), "en") != want {
			t.Errorf("find_places(%q) = %v, want %q first", text, got, want)
		}
	}

	near := run(t, s, findPlaces, `{"text": "marsilgin drive"}`)
	if got := column(near, "map"); len(got) != 1 || got[0] != "5, Marsiling Drive" || near.Note == "" {
		t.Errorf("a typo must find the closest name with a note to check: %v, note %q", got, near.Note)
	}
	if exact := run(t, s, findPlaces, `{"text": "rich"}`); exact.Note != "" {
		t.Errorf("a real match must not carry the near-miss note")
	}
	// Numbers never match loosely, and a territory code alone does not list its maps.
	if got := column(run(t, s, findPlaces, `{"text": "6 Marsiling Drive"}`), "map"); len(got) != 0 {
		t.Errorf("6 must not match 5: %v", got)
	}
	if got := column(run(t, s, findPlaces, `{"text": "T01"}`), "map"); len(got) != 0 && got[0] != "" {
		t.Errorf("T01 must find the territory, not its maps: %v", got)
	}
}

func TestPlaceMatch(t *testing.T) {
	for _, c := range []struct {
		query, name string
		typos       int
		ok          bool
	}{
		{"5 marsiling dr", "5, Marsiling Drive", 0, true},
		{"marsilgin", "Marsiling", 2, true},
		{"marsilin", "Marsiling", 0, true}, // the start of the word
		{"mrsiling", "Marsiling", 1, true},
		{"rd", "Road", 0, true},
		{"5", "6, Marsiling Drive", 0, false},
		{"cat", "Cut", 0, false}, // too short to forgive
	} {
		typos, ok := placeMatch(placeWords(c.query), placeWords(c.name), nil)
		if ok != c.ok || (ok && typos != c.typos) {
			t.Errorf("placeMatch(%q, %q) = %d, %v; want %d, %v", c.query, c.name, typos, ok, c.typos, c.ok)
		}
	}
}

// Every function, given the broadest arguments, must return nothing from another
// congregation. The table must name every function, so a new one cannot skip it.
func TestTools_NeverReturnAnotherCongregation(t *testing.T) {
	broad := map[string][]string{
		"find_places":        {`{"text": "Blk"}`, `{"text": "T0"}`, `{"text": "Alpha"}`, `{"text": "Beta"}`},
		"territory_overview": {`{}`, `{"territory": "T01"}`},
		"map_overview":       {`{}`},
		"map_details":        {`{"map": "Blk 100A"}`, `{"map": "Blk 300A"}`},
		"activity":           {`{"from": "2026-01-01", "to": "2026-11-06", "group_by": "map"}`, `{"from": "2026-01-01", "to": "2026-11-06", "group_by": "territory"}`},
		"who_worked":         {`{"from": "2026-01-01", "to": "2026-11-06"}`},
		"people_and_roles":   {`{}`},
		"homes": {`{"group_by": "home", "limit": 100}`, `{"group_by": "map"}`, `{"group_by": "territory"}`,
			`{"group_by": "household_type"}`, `{"map": "Blk 300A", "group_by": "home"}`, `{"territory": "T01", "group_by": "total"}`},
		"congregation_settings": {`{}`},
		"assignments":           {`{"which": "active"}`, `{"which": "given", "within_days": 3650}`},
		"feedback":              {`{"kind": "both", "within_days": 3650}`},
		"data_quality":          {`{"check": "maps_without_location"}`, `{"check": "territories_without_boundary"}`, `{"check": "empty_maps"}`},
		"maps_near":             {`{"from": "place", "place": "T01"}`, `{"from": "my_location"}`},
		"propose_role_change":   {`{"person": "Alpha Admin", "role": "read_only"}`, `{"person": "Beta Admin", "role": "read_only"}`},
		"propose_rename": {`{"what": "map", "name": "Blk 100A", "new_name": "New Name"}`, `{"what": "map", "name": "Blk 300A", "new_name": "New Name"}`,
			`{"what": "territory", "name": "T01", "new_name": "New Name"}`},
		"propose_share_link":    {`{"map": "Blk 100A", "publisher": "Someone New", "type": "normal"}`, `{"map": "Blk 300A", "publisher": "Someone New", "type": "normal"}`},
		"propose_pin_message":   {`{"map": "Blk 100A", "message": "Knock softly"}`, `{"map": "Blk 300A", "message": "Knock softly"}`},
		"propose_report_email":  {`{}`},
		"propose_mark_read":     {`{}`, `{"territory": "T01"}`},
		"propose_unpin_message": {`{"map": "Blk 100A"}`, `{"map": "Blk 300A"}`},
		"propose_map_location":  {`{"map": "Blk 100A"}`, `{"map": "Blk 300A"}`},
		"propose_link_revoke":   {`{"map": "Blk 100A"}`, `{"map": "Blk 300A"}`},
		"propose_quick_link":    {`{"territory": "T01", "publisher": "Someone New"}`},
		"propose_home_update":   {`{"map": "Blk 100A", "unit": "1", "status": "done"}`, `{"map": "Blk 300A", "unit": "60", "status": "done"}`},
		"propose_reset":         {`{"what": "map", "name": "Blk 100A"}`, `{"what": "map", "name": "Blk 300A"}`, `{"what": "territory", "name": "T01"}`},
		"propose_tries_reset":   {`{}`},
		"calculate":             {`{"expression": "(1 + 2) * 100"}`},
		"territory_rounds": {`{"view": "rounds", "from": "2026-01-01", "to": "2026-11-06", "territory": null, "limit": null}`,
			`{"view": "territories", "from": null, "to": null, "territory": null, "limit": null}`},
		"propose_homes_status":     {`{"map": "Blk 100A", "status": "done"}`, `{"map": "Blk 300A", "status": "done"}`},
		"propose_dnc_return":       {`{"older_than_months": 1}`},
		"propose_settings":         {`{"max_tries": 2}`},
		"propose_map_move":         {`{"map": "Blk 100A", "to_territory": "T02"}`, `{"map": "Blk 300A", "to_territory": "T01"}`},
		"propose_floor_add":        {`{"map": "Rich Map", "where": "top"}`},
		"propose_unit_add":         {`{"map": "Blk 100A", "units": ["15"]}`, `{"map": "Blk 300A", "units": ["15"]}`},
		"propose_territory_create": {`{"code": "T09", "name": "New Area", "maps": [{"name": "Blk 1", "kind": "houses", "lowest_floor": null, "top_floor": null, "units": [{"from": "1", "to": "3", "only": null, "codes": null}], "skip": [], "address": null}]}`},
		"propose_maps_create":      {`{"territory": "T01", "maps": [{"name": "Blk 9", "kind": "houses", "lowest_floor": null, "top_floor": null, "units": [{"from": "1", "to": "3", "only": null, "codes": null}], "skip": [], "address": null}]}`},
		// Searches every account in the app, as Invite User does, so it is
		// exempt from the cross-congregation check below by design.
		"propose_role_grant": {`{"search": "Alpha", "role": "read_only"}`},
		"change_history":     {`{"kind": "territory", "within_days": 3650}`, `{"kind": "role", "within_days": 3650}`, `{"kind": "assignment", "within_days": 3650}`, `{"kind": "home", "within_days": 3650}`},
	}
	for _, tool := range Tools {
		if _, ok := broad[tool.Name]; !ok {
			t.Errorf("%s is missing from the leak test", tool.Name)
		}
	}

	app := newTestApp(t)
	day := time.Date(2026, 11, 2, 2, 0, 0, 0, time.UTC)
	addMalay(t, app, "testalpha01a001")
	logStatus(t, app, "testlogleak0001", "testalpha01a001", "done", "Alpha Walker", day)
	logStatus(t, app, "testlogleak0002", "testbeta001a001", "done", "Beta Walker", day)
	logAssigned(t, app, "testasgleak0001", "testmapalpha01a", "Alpha Walker", "normal", day)
	logAssigned(t, app, "testasgleak0002", "testmapbeta001a", "Beta Walker", "normal", day)
	logRound(t, app, "testroundleak01", alphaCong, "testterralpha01", "T01", "completed", `{}`, day)
	logRound(t, app, "testroundleak02", betaCong, "testterrbeta001", "B01", "completed", `{}`, day)

	check := func(congregation string, forbidden ...string) {
		s := scopeFor(t, app, congregation)
		s.Origin = &Location{Lat: 1.30, Lng: 103.80}
		s.Role = "administrator"
		returned := 0
		for _, tool := range Tools {
			for _, args := range broad[tool.Name] {
				r, err := tool.Run(s, json.RawMessage(args))
				var bad argError
				if errors.As(err, &bad) {
					continue // e.g. the other congregation's territory code: refused, not leaked
				}
				if err != nil {
					t.Fatalf("%s(%s) as %s: %v", tool.Name, args, congregation, err)
				}
				returned += len(r.Rows)
				if tool.Name == "propose_role_grant" {
					continue
				}
				raw, _ := json.Marshal(struct {
					Result   Result
					Proposal *Proposal
				}{r, r.Proposal})
				text := strings.ToLower(string(raw))
				for _, f := range forbidden {
					if strings.Contains(text, f) {
						t.Errorf("%s(%s) as %s returned %q: %s", tool.Name, args, congregation, f, raw)
					}
				}
			}
		}
		if returned < 10 {
			t.Errorf("as %s the functions returned only %d rows; the test proves nothing if nothing comes back", congregation, returned)
		}
	}
	check(betaCong, "alph", "blk 100", "blk 200", "single code", "multi floor", "malay")
	check(alphaCong, "beta", "blk 300")
}

func TestWhoWorked_RanksByDaysOut(t *testing.T) {
	app := newTestApp(t)
	day1 := time.Date(2026, 11, 2, 2, 0, 0, 0, time.UTC)  // 2 Nov, 10:00 Singapore
	late := time.Date(2026, 11, 2, 17, 0, 0, 0, time.UTC) // 3 Nov, 01:00 Singapore
	day3 := time.Date(2026, 11, 4, 2, 0, 0, 0, time.UTC)
	logAssigned(t, app, "testasgdays0001", "testmapalpha01a", "Sis Lim", "normal", day1)
	logStatus(t, app, "testlogdays0001", "testalpha01a001", "done", "Sis Lim", day1.Add(time.Hour)) // same day: still 1
	logStatus(t, app, "testlogdays0002", "testalpha01a002", "done", "Sis Lim", late)
	logStatus(t, app, "testlogdays0003", "testalpha02a001", "done", "Sis Lim", day3)
	logAssigned(t, app, "testasgdays0002", "testmapalpha02a", "Bro Tan", "normal", day3)
	s := scopeFor(t, app, alphaCong)

	ranked := run(t, s, whoWorked, `{"from": "2026-11-01", "to": "2026-11-06", "sort": "days_out"}`)
	if got := column(ranked, "name"); len(got) != 2 || got[0] != "Sis Lim" {
		t.Fatalf("ranked = %v, want Sis Lim first", got)
	}
	if ranked.Rows[0]["days_out"] != 3 || ranked.Rows[1]["days_out"] != 1 {
		t.Errorf("days_out = %v and %v, want 3 (two events on 2 Nov count once; 01:00 local is 3 Nov) and 1",
			ranked.Rows[0]["days_out"], ranked.Rows[1]["days_out"])
	}

	alphabetical := run(t, s, whoWorked, `{"from": "2026-11-01", "to": "2026-11-06"}`)
	if got := column(alphabetical, "name"); got[0] != "Bro Tan" {
		t.Errorf("the default order must stay alphabetical, got %v", got)
	}
}

func TestRestoreNames_PutsBackOnlyKnownPeople(t *testing.T) {
	app := newTestApp(t)
	s := scopeFor(t, app, alphaCong)
	token := newPseudonyms(alphaCong).token("Alpha Admin")

	restored, people, err := RestoreNames(s, []string{token + " came out.", "Person ZZZZZZZZ was not found.", ""})
	if err != nil {
		t.Fatal(err)
	}
	if restored[0] != "Alpha Admin came out." || restored[1] != "Person ZZZZZZZZ was not found." || restored[2] != "" {
		t.Fatalf("restored = %q", restored)
	}
	if len(people) != 1 || people[token] != "Alpha Admin" {
		t.Fatalf("people = %v, want only the token used", people)
	}
}
