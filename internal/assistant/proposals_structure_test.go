//go:build testdata

package assistant

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProposeMapReset(t *testing.T) {
	s := adminScope(t, alphaCong, "testuseralpha01")
	p := propose(t, s, proposeReset, `{"what": "map", "name": "Blk 100A"}`)
	if p.Kind != "map_reset" || p.Target.ID != "testmapalpha01a" || strings.Join(p.Warnings, ",") != "cannot_undo" {
		t.Fatalf("proposal = %+v", p)
	}
	if p.Data["done"] != 1 || p.Data["not_home"] != 2 {
		t.Fatalf("data = %v, want 1 done and 2 not home on Blk 100A", p.Data)
	}
	_, err := proposeReset.Run(s, json.RawMessage(`{"what": "map", "name": "Blk 200B"}`))
	wantArgError(t, err, "nothing to reset")
}

func TestProposeTerritoryReset(t *testing.T) {
	s := adminScope(t, alphaCong, "testuseralpha01")
	p := propose(t, s, proposeReset, `{"what": "territory", "name": "t01"}`)
	if p.Kind != "territory_reset" || p.Target.Name != "T01" ||
		strings.Join(p.Warnings, ",") != "cannot_undo,starts_round" || p.Data["maps"].(int) < 1 {
		t.Fatalf("proposal = %+v", p)
	}
}

func TestProposeTriesReset(t *testing.T) {
	app := newTestApp(t)
	exec(t, app, `UPDATE addresses SET not_home_tries = 3 WHERE id = 'testalpha01a003'`, nil)
	s := scopeFor(t, app, alphaCong)
	s.Role, s.UserID = "conductor", "testuseralpha02"
	p := propose(t, s, proposeTriesReset, `{"territory": "T01"}`)
	if p.Kind != "tries_reset" || len(p.Items) != 1 || p.Items[0].ID != "testalpha01a003" || p.Items[0].Label != "Blk 100A 12" {
		t.Fatalf("proposal = %+v; only the home at the limit of 3", p)
	}
	var before homeState
	if err := json.Unmarshal([]byte(p.Items[0].Detail), &before); err != nil || before.Tries != 3 ||
		strings.Join(before.Types, ",") != "testoptialpha01" {
		t.Fatalf("detail = %s; the full state is needed to restore it", p.Items[0].Detail)
	}
	if p.Value != `{"status":"not_done","not_home_tries":0}` {
		t.Fatalf("value = %s", p.Value)
	}
	_, err := proposeTriesReset.Run(s, json.RawMessage(`{"map": "Blk 200A"}`))
	wantArgError(t, err, "try limit")
}

func TestProposeHomesStatus(t *testing.T) {
	app := newTestApp(t)
	exec(t, app, `UPDATE addresses SET status = 'invalid' WHERE id = 'testalpha01a002'`, nil)
	s := scopeFor(t, app, alphaCong)
	s.Role, s.UserID = "conductor", "testuseralpha02"

	// Blk 100A has 10 and 11 not done (11 now invalid), 12 and 13 nobody home, 14 done.
	p := propose(t, s, proposeHomesStatus, `{"map": "Blk 100A", "status": "done", "from_floor": 1, "to_floor": 1}`)
	var labels []string
	for _, it := range p.Items {
		labels = append(labels, it.Label)
	}
	if p.Kind != "homes_status" || p.To != "done" || strings.Join(labels, ",") != "Blk 100A 10,Blk 100A 12,Blk 100A 13" {
		t.Fatalf("proposal = %+v; invalid and already-done homes stay out", p)
	}
	if p.Value != `{"status":"done"}` || p.Items[0].Detail == "" {
		t.Fatalf("value = %s, detail = %q", p.Value, p.Items[0].Detail)
	}

	p = propose(t, s, proposeHomesStatus, `{"map": "Blk 100A", "status": "not_home", "units": ["10", "#01-14"]}`)
	if len(p.Items) != 2 || p.Value != `{"not_home_tries":1,"status":"not_home"}` {
		t.Fatalf("proposal = %+v; only the named units, starting at one try", p)
	}

	_, err := proposeHomesStatus.Run(s, json.RawMessage(`{"map": "Blk 100A", "status": "done", "from_floor": 2, "to_floor": 5}`))
	wantArgError(t, err, "no home there would change")
	_, err = proposeHomesStatus.Run(s, json.RawMessage(`{"map": "Blk 100A", "status": "do_not_call"}`))
	wantArgError(t, err, "one home at a time")
	_, err = proposeHomesStatus.Run(s, json.RawMessage(`{"map": "Blk 100A", "status": "done", "from_floor": 5, "to_floor": 1}`))
	wantArgError(t, err, "above")
}

func TestProposeDncReturn(t *testing.T) {
	app := newTestApp(t)
	exec(t, app, `UPDATE addresses SET status = 'do_not_call', dnc_time = '2024-01-01 00:00:00.000Z' WHERE id = 'testalpha01a001'`, nil)
	exec(t, app, `UPDATE addresses SET status = 'do_not_call', dnc_time = '2026-10-01 00:00:00.000Z' WHERE id = 'testalpha01a002'`, nil)
	s := scopeFor(t, app, alphaCong)
	s.Role, s.UserID = "administrator", "testuseralpha01"
	p := propose(t, s, proposeDncReturn, `{"older_than_months": 12}`)
	if p.Kind != "dnc_return" || len(p.Items) != 1 || p.Items[0].ID != "testalpha01a001" ||
		strings.Join(p.Warnings, ",") != "leaves_do_not_call" || p.Data["months"] != 12 {
		t.Fatalf("proposal = %+v; only the home marked over a year ago", p)
	}
	_, err := proposeDncReturn.Run(s, json.RawMessage(`{"older_than_months": 0}`))
	wantArgError(t, err, "between 1 and 120")
}

func TestProposeSettings(t *testing.T) {
	s := adminScope(t, alphaCong, "testuseralpha01")
	p := propose(t, s, proposeSettings, `{"max_tries": 2, "completion_threshold": 80}`)
	var before, after settingsState
	_ = json.Unmarshal([]byte(p.Previous), &before)
	_ = json.Unmarshal([]byte(p.Value), &after)
	if p.Kind != "settings" || before.MaxTries != 3 || after.MaxTries != 2 || after.Threshold != 80 || after.Expiry != before.Expiry {
		t.Fatalf("proposal = %+v", p)
	}
	_, err := proposeSettings.Run(s, json.RawMessage(`{"max_tries": 9}`))
	wantArgError(t, err, "between 1 and 4")
	_, err = proposeSettings.Run(s, json.RawMessage(`{"max_tries": 3}`))
	wantArgError(t, err, "already have those values")
}

func TestProposeMapMove(t *testing.T) {
	s := adminScope(t, alphaCong, "testuseralpha01")
	p := propose(t, s, proposeMapMove, `{"map": "Blk 100A", "to_territory": "t02"}`)
	if p.Kind != "map_move" || p.From != "T01" || p.To != "T02" || p.Previous != "testterralpha01" || p.Value != "testterralpha02" {
		t.Fatalf("proposal = %+v", p)
	}
	_, err := proposeMapMove.Run(s, json.RawMessage(`{"map": "Blk 100A", "to_territory": "T01"}`))
	wantArgError(t, err, "already in")
}

func TestProposeFloorAdd(t *testing.T) {
	s := adminScope(t, alphaCong, "testuseralpha01")
	top := propose(t, s, proposeFloorAdd, `{"map": "Rich Map", "where": "top"}`)
	if top.Kind != "floor_add" || top.Data["floor"] != 2 || strings.Join(top.Data["units"].([]string), ",") != "R01,R02" {
		t.Fatalf("proposal = %+v", top)
	}
	// /map/floor/add skips floor 0 going down.
	if bottom := propose(t, s, proposeFloorAdd, `{"map": "Rich Map", "where": "bottom"}`); bottom.Data["floor"] != -1 {
		t.Fatalf("bottom floor = %v, want -1", bottom.Data["floor"])
	}
	_, err := proposeFloorAdd.Run(s, json.RawMessage(`{"map": "Blk 100A", "where": "top"}`))
	wantArgError(t, err, "map of houses")
}

func TestProposeUnitAdd(t *testing.T) {
	s := adminScope(t, alphaCong, "testuseralpha01")
	p := propose(t, s, proposeUnitAdd, `{"map": "Multi Floor Blk", "units": ["#03", "02", "03", "4A"]}`)
	if p.Kind != "unit_add" || strings.Join(p.Data["units"].([]string), ",") != "03,4A" ||
		strings.Join(p.Data["existing"].([]string), ",") != "02" || p.Data["floors"] != 2 ||
		strings.Join(p.Warnings, ",") != "some_exist" {
		t.Fatalf("proposal = %+v", p)
	}
	_, err := proposeUnitAdd.Run(s, json.RawMessage(`{"map": "Blk 100A", "units": ["10"]}`))
	wantArgError(t, err, "already has 10")
	_, err = proposeUnitAdd.Run(s, json.RawMessage(`{"map": "Blk 100A", "units": ["1 2"]}`))
	wantArgError(t, err, "letters, digits and hyphens")
}
