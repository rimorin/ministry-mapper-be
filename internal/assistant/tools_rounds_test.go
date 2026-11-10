//go:build testdata

package assistant

import (
	"testing"
	"time"

	"ganigeorgiev.com/pocketbase/dbx"
	"ganigeorgiev.com/pocketbase/pocketbase/tests"
)

func logRound(t testing.TB, app *tests.TestApp, id, congregation, territory, code, action, details string, at time.Time) {
	t.Helper()
	exec(t, app, `INSERT INTO territories_log (id, congregation, territory, territory_code, action, details, created, updated)
		VALUES ({:id}, {:congregation}, {:territory}, {:code}, {:action}, {:details}, {:at}, {:at})`,
		dbx.Params{"id": id, "congregation": congregation, "territory": territory, "code": code,
			"action": action, "details": details, "at": dbTime(at)})
}

// sgt is a time on a local date in Singapore, where the seed congregations are.
func sgt(month time.Month, day int) time.Time {
	return time.Date(2026, month, day, 2, 0, 0, 0, time.UTC) // 10:00 SGT
}

func TestTerritoryRounds(t *testing.T) {
	app := newTestApp(t)
	// T01: a completion from before the history began, a timed round, and
	// a round open since 20 October.
	logRound(t, app, "testround000001", alphaCong, "testterralpha01", "T01", "completed", `{}`, sgt(time.September, 3))
	logRound(t, app, "testround000002", alphaCong, "testterralpha01", "T01", "reset", ``, sgt(time.September, 10))
	logStatus(t, app, "testloground001", "testalpha01a001", "done", "Bro Tan", sgt(time.September, 15))
	logRound(t, app, "testround000003", alphaCong, "testterralpha01", "T01", "map_reset", `{"new_round": false}`, sgt(time.September, 20))
	logRound(t, app, "testround000004", alphaCong, "testterralpha01", "T01", "completed", `{}`, sgt(time.October, 5))
	logRound(t, app, "testround000005", alphaCong, "testterralpha01", "T01", "map_reset", `{"new_round": true}`, sgt(time.October, 20))
	logRound(t, app, "testround000006", betaCong, "testterrbeta001", "B01", "completed", `{}`, sgt(time.October, 1))
	exec(t, app, `UPDATE territories SET round_started_at = {:started}, last_completed_at = {:completed} WHERE id = 'testterralpha01'`,
		dbx.Params{"started": dbTime(sgt(time.October, 20)), "completed": dbTime(sgt(time.October, 5))})
	s := scopeFor(t, app, alphaCong)

	rounds := run(t, s, territoryRounds, `{"view": "rounds", "from": null, "to": null, "territory": null, "limit": null}`)
	want := []map[string]any{
		{"territory": "T01", "started": "", "work_began": "", "completed": "2026-09-03", "days": nil},
		{"territory": "T01", "started": "2026-09-10", "work_began": "2026-09-15", "completed": "2026-10-05", "days": 20},
		{"territory": "T01", "started": "2026-10-20", "work_began": "", "completed": "", "days": nil},
	}
	if len(rounds.Rows) != len(want) {
		t.Fatalf("rounds = %v", rounds.Rows)
	}
	for i, w := range want {
		for k, v := range w {
			if rounds.Rows[i][k] != v {
				t.Errorf("round %d %s = %v, want %v", i, k, rounds.Rows[i][k], v)
			}
		}
	}

	byTerritory := run(t, s, territoryRounds, `{"view": "territories", "from": null, "to": null, "territory": "T01", "limit": null}`)
	if len(byTerritory.Rows) != 1 {
		t.Fatalf("rows = %v", byTerritory.Rows)
	}
	row := byTerritory.Rows[0]
	if row["completed_rounds"] != 2 || row["average_days"] != 20 || row["last_completed"] != "2026-10-05" || row["open_since"] != "2026-10-20" {
		t.Errorf("T01 = %v", row)
	}

	// A period before the service year holds none of these rounds.
	earlier := run(t, s, territoryRounds, `{"view": "rounds", "from": "2026-06-01", "to": "2026-08-31", "territory": null, "limit": null}`)
	if len(earlier.Rows) != 0 {
		t.Errorf("rounds before September = %v", earlier.Rows)
	}
}

func TestServiceYearStart(t *testing.T) {
	for day, want := range map[time.Time]string{
		time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC): "2026-09-01",
		time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC):  "2026-09-01",
		time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC): "2025-09-01",
	} {
		if got := serviceYearStart(day).Format(time.DateOnly); got != want {
			t.Errorf("serviceYearStart(%s) = %s, want %s", day.Format(time.DateOnly), got, want)
		}
	}
}
