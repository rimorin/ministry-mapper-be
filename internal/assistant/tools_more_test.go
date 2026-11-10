//go:build testdata

package assistant

import (
	"strings"
	"testing"
	"time"

	"ganigeorgiev.com/pocketbase/dbx"
)

func TestAssignments_ActiveAndExpiring(t *testing.T) {
	app := newTestApp(t)
	exec(t, app, `INSERT INTO assignments (id, congregation, map, publisher, type, expiry_date, created, updated)
		VALUES ('testassignsoon01', {:cong}, 'testmapalpha02a', 'Sis Lim', 'normal', {:soon}, {:now}, {:now})`,
		dbx.Params{"cong": alphaCong, "soon": dbTime(testNow.Add(2 * time.Hour)), "now": dbTime(testNow)})
	s := scopeFor(t, app, alphaCong)

	if r := run(t, s, assignments, `{"which": "active", "type": "normal"}`); r.Total != 1 || r.Rows[0]["publisher"] != "Sis Lim" {
		t.Fatalf("active normal links = %v, want Sis Lim's", r.Rows)
	}
	soon := run(t, s, assignments, `{"which": "active", "expiring_within_hours": 24}`)
	if len(soon.Rows) != 1 || soon.Rows[0]["publisher"] != "Sis Lim" || soon.Rows[0]["expires"] != "2026-11-06 14:00" {
		t.Fatalf("expiring = %v", soon.Rows)
	}
}

func TestNotHomeBacklog_AtTheTryLimit(t *testing.T) {
	app := newTestApp(t)
	exec(t, app, `UPDATE addresses SET not_home_tries = 3 WHERE id = 'testalpha01a003'`, nil)
	exec(t, app, `UPDATE addresses SET not_home_tries = 1 WHERE id = 'testalpha01a004'`, nil)
	s := scopeFor(t, app, alphaCong)

	atLimit := run(t, s, notHomeBacklog, `{"territory": "T01", "at_limit_only": true, "group_by": "home"}`)
	if len(atLimit.Rows) != 1 || atLimit.Rows[0]["home"] != "12" || atLimit.Rows[0]["try_limit"] != 3 {
		t.Fatalf("at limit = %v", atLimit.Rows)
	}
	byMap := run(t, s, notHomeBacklog, `{"territory": "T01", "group_by": "map"}`)
	if len(byMap.Rows) != 1 || byMap.Rows[0]["nobody_home"] != 2 || byMap.Rows[0]["at_try_limit"] != 1 {
		t.Fatalf("by map = %v", byMap.Rows)
	}
}

func TestDoNotCallReview_OlderThan(t *testing.T) {
	app := newTestApp(t)
	exec(t, app, `UPDATE addresses SET status = 'do_not_call', dnc_time = {:old} WHERE id = 'testalpha01b001'`,
		dbx.Params{"old": dbTime(testNow.AddDate(0, 0, -400))})
	exec(t, app, `UPDATE addresses SET status = 'do_not_call', dnc_time = {:recent} WHERE id = 'testalpha01b002'`,
		dbx.Params{"recent": dbTime(testNow.AddDate(0, 0, -10))})

	r := run(t, scopeFor(t, app, alphaCong), doNotCallReview, `{"older_than_days": 365, "group_by": "home"}`)
	if len(r.Rows) != 1 || r.Rows[0]["home"] != "20" || r.Rows[0]["marked"] != "2025-10-02" {
		t.Fatalf("rows = %v", r.Rows)
	}
}

func TestFeedback_FindsNotesByText(t *testing.T) {
	app := newTestApp(t)
	exec(t, app, `UPDATE addresses SET notes = 'Beware of the dog', last_notes_updated = {:at}, last_notes_updated_by = 'Bro Tan'
		WHERE id = 'testalpha01a001'`, dbx.Params{"at": dbTime(testNow.AddDate(0, 0, -1))})

	r := run(t, scopeFor(t, app, alphaCong), feedback, `{"kind": "both", "text": "DOG"}`)
	if len(r.Rows) != 1 || r.Rows[0]["kind"] != "note" || r.Rows[0]["home"] != "10" || r.Rows[0]["by"] != "Bro Tan" {
		t.Fatalf("rows = %v", r.Rows)
	}
}

func TestDataQuality_MapOutsideItsBoundary(t *testing.T) {
	app := newTestApp(t)
	exec(t, app, `UPDATE territories SET coordinates = '[{"lat":1.30,"lng":103.80},{"lat":1.30,"lng":103.81},{"lat":1.31,"lng":103.81},{"lat":1.31,"lng":103.80}]'
		WHERE id = 'testterralpha01'`, nil)
	exec(t, app, `UPDATE maps SET coordinates = '{"lat":1.305,"lng":103.805}' WHERE id = 'testmapalpha01a'`, nil)
	exec(t, app, `UPDATE maps SET coordinates = '{"lat":1.40,"lng":103.90}' WHERE id = 'testmapalpha01b'`, nil)

	r := run(t, scopeFor(t, app, alphaCong), dataQuality, `{"check": "maps_outside_boundary"}`)
	if got := column(r, "map"); len(got) != 1 || got[0] != "Blk 100B" {
		t.Fatalf("outside = %v, want only Blk 100B", got)
	}
}

func TestChangeHistory_RolesNameDeletedAccountsByEmail(t *testing.T) {
	app := newTestApp(t)
	exec(t, app, `INSERT INTO roles_log (id, congregation, user, old_role, new_role, action, changed_by, created, updated)
		VALUES ('testrolelog0001', {:cong}, 'testuseralpha02', 'read_only', 'conductor', 'changed', 'testuseralpha01', {:older}, {:older})`,
		dbx.Params{"cong": alphaCong, "older": dbTime(testNow.AddDate(0, 0, -3))})
	exec(t, app, `INSERT INTO roles_log (id, congregation, user, user_email, old_role, new_role, action, changed_by, created, updated)
		VALUES ('testrolelog0002', {:cong}, '', 'gone@alpha.test', 'read_only', '', 'revoked', 'testuseralpha01', {:newer}, {:newer})`,
		dbx.Params{"cong": alphaCong, "newer": dbTime(testNow.AddDate(0, 0, -1))})

	r := run(t, scopeFor(t, app, alphaCong), changeHistory, `{"kind": "role"}`)
	if len(r.Rows) != 2 || r.Rows[0]["person"] != "gone@alpha.test" || r.Rows[1]["person"] != "Alpha Conductor" ||
		r.Rows[1]["new_value"] != "conductor" || r.Rows[1]["by"] != "Alpha Admin" {
		t.Fatalf("rows = %v", r.Rows)
	}
}

func TestInsidePolygon(t *testing.T) {
	square := []point{{0, 0}, {0, 1}, {1, 1}, {1, 0}}
	if !insidePolygon(point{0.5, 0.5}, square) || insidePolygon(point{1.5, 0.5}, square) {
		t.Fatal("ray casting misplaces a point")
	}
}

func TestRunTool_ResolvesNamesEnteredInSeveralLanguages(t *testing.T) {
	app := newTestApp(t)
	for locale, want := range map[string]string{"zh": "富地图", "en": "Rich Map", "ta": "Rich Map"} {
		s := scopeFor(t, app, alphaCong)
		s.Locale = locale
		step, view, err := runTool(s, newPseudonyms(alphaCong), "find_places", `{"text": "rich"}`)
		if err != nil {
			t.Fatal(err)
		}
		if got := step.Result.Rows[0]["map"]; got != want {
			t.Errorf("%s: map = %v, want %q", locale, got, want)
		}
		if !strings.Contains(view, want) || strings.Contains(view, `\"en\"`) {
			t.Errorf("%s: the model must see the resolved name, got %s", locale, view)
		}
	}
	if _, err := mapIDByName(scopeFor(t, app, alphaCong), "", ptr("Rich Map")); err != nil {
		t.Errorf("a map named in several languages must be found by one of its names: %v", err)
	}
}

func ptr(s string) *string { return &s }
