//go:build testdata

package assistant

import (
	"encoding/json"
	"strings"
	"testing"
)

func numberRun(from, to, only string) unitPart {
	p := unitPart{From: ptr(from), To: ptr(to)}
	if only != "" {
		p.Only = ptr(only)
	}
	return p
}

// The numbering seen in real maps: odd stacks, both sides of a street,
// counting down, letters, skips, zero-padding and lists as typed.
func TestExpandUnits_RealNumbering(t *testing.T) {
	for _, c := range []struct {
		name  string
		parts []unitPart
		skip  []string
		want  string
	}{
		{"odd stack", []unitPart{numberRun("671", "685", "odd")}, nil, "671,673,675,677,679,681,683,685"},
		{"even from an odd start", []unitPart{numberRun("1", "9", "even")}, nil, "2,4,6,8"},
		{"odd side then even side", []unitPart{numberRun("61", "65", "odd"), numberRun("88", "92", "even")}, nil, "61,63,65,88,90,92"},
		{"counting down", []unitPart{numberRun("795", "789", "odd")}, nil, "795,793,791,789"},
		{"letters in place", []unitPart{numberRun("1", "3", ""), {Codes: []string{"3A"}}, numberRun("4", "5", "")}, nil, "1,2,3,3A,4,5"},
		{"skips", []unitPart{numberRun("1", "15", "")}, []string{"4", "13", "#14"}, "1,2,3,5,6,7,8,9,10,11,12,15"},
		{"zero padding kept", []unitPart{numberRun("01", "04", "")}, nil, "01,02,03,04"},
		{"listed as typed", []unitPart{{Codes: []string{"27", "#29", "32", "23B"}}}, nil, "27,29,32,23B"},
	} {
		got, err := expandUnits(c.parts, c.skip)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if strings.Join(got, ",") != c.want {
			t.Errorf("%s = %s, want %s", c.name, strings.Join(got, ","), c.want)
		}
	}

	for _, c := range []struct {
		name  string
		parts []unitPart
		want  string
	}{
		{"repeated unit", []unitPart{numberRun("1", "3", ""), {Codes: []string{"02"}}}, "appears twice"},
		{"letters in a run", []unitPart{numberRun("1A", "5A", "")}, "whole number"},
		{"bad characters", []unitPart{{Codes: []string{"1/2"}}}, "letters, digits and hyphens"},
		{"too many", []unitPart{numberRun("1", "600", "")}, "at most 500"},
		{"empty part", []unitPart{{}}, "needs from and to, or codes"},
	} {
		_, err := expandUnits(c.parts, nil)
		wantArgError(t, err, c.want)
	}
}

func TestFloorCount_SkipsFloorZero(t *testing.T) {
	if floorCount(2, 12) != 11 || floorCount(1, 1) != 1 || floorCount(-1, 3) != 4 {
		t.Fatalf("floor counts wrong: %d %d %d", floorCount(2, 12), floorCount(1, 1), floorCount(-1, 3))
	}
}

const blk412 = `{"name": "Blk 412", "kind": "building", "lowest_floor": 2, "top_floor": 13,
	"units": [{"from": "671", "to": "685", "only": "odd", "codes": null}], "skip": [], "address": "412 Bedok North Ave 2"}`

func TestProposeTerritoryCreate(t *testing.T) {
	s := adminScope(t, alphaCong, "testuseralpha01")
	p := propose(t, s, proposeTerritoryCreate, `{"code": "T05", "name": "Bedok North", "maps": [`+blk412+`,
		{"name": "Jalan Bahagia", "kind": "houses", "lowest_floor": null, "top_floor": null,
		 "units": [{"from": "1", "to": "5", "only": "odd", "codes": null}, {"from": null, "to": null, "only": null, "codes": ["5A"]}],
		 "skip": [], "address": null}]}`)
	if p.Kind != "territory_create" || p.Target.Name != "T05" || p.Value != "Bedok North" || p.Data["homes"] != 12*8+4 {
		t.Fatalf("proposal = %+v", p)
	}
	maps := p.Data["maps"].([]map[string]any)
	if maps[0]["type"] != "multi" || maps[0]["lowest_floor"] != 2 || maps[0]["floors"] != 12 ||
		maps[0]["address"] != "412 Bedok North Ave 2" || len(maps[0]["units"].([]string)) != 8 {
		t.Fatalf("building = %v", maps[0])
	}
	if maps[1]["type"] != "single" || maps[1]["address"] != "Jalan Bahagia" ||
		strings.Join(maps[1]["units"].([]string), ",") != "1,3,5,5A" {
		t.Fatalf("street = %v", maps[1])
	}
	if view := proposalView(p); !strings.Contains(view, `"maps":2`) || strings.Contains(view, "671") {
		t.Fatalf("the model sees the counts, not the units: %s", view)
	}

	// A territory alone is fine; maps can follow.
	if alone := propose(t, s, proposeTerritoryCreate, `{"code": "T06", "name": "Tampines", "maps": []}`); alone.Data["homes"] != 0 {
		t.Fatalf("territory alone = %+v", alone)
	}
	_, err := proposeTerritoryCreate.Run(s, json.RawMessage(`{"code": "t01", "name": "Again", "maps": []}`))
	wantArgError(t, err, "already exists")
}

func TestProposeMapsCreate(t *testing.T) {
	s := adminScope(t, alphaCong, "testuseralpha01")
	p := propose(t, s, proposeMapsCreate, `{"territory": "t01", "maps": [`+blk412+`]}`)
	if p.Kind != "maps_create" || p.Target.ID != "testterralpha01" || p.Target.Name != "T01" {
		t.Fatalf("proposal = %+v", p)
	}

	for _, c := range []struct{ args, want string }{
		{`{"territory": "T01", "maps": [{"name": "Blk 100A", "kind": "houses", "lowest_floor": null, "top_floor": null,
			"units": [{"from": "1", "to": "2", "only": null, "codes": null}], "skip": [], "address": null}]}`, "already has a map named"},
		{`{"territory": "T01", "maps": [{"name": "Blk 5", "kind": "building", "lowest_floor": null, "top_floor": null,
			"units": [{"from": "1", "to": "2", "only": null, "codes": null}], "skip": [], "address": null}]}`, "never guess"},
		{`{"territory": "T01", "maps": [{"name": "Blk 5", "kind": "building", "lowest_floor": 0, "top_floor": 5,
			"units": [{"from": "1", "to": "2", "only": null, "codes": null}], "skip": [], "address": null}]}`, "no floor 0"},
		{`{"territory": "T01", "maps": [{"name": "Blk 5", "kind": "building", "lowest_floor": 1, "top_floor": 101,
			"units": [{"from": "1", "to": "2", "only": null, "codes": null}], "skip": [], "address": null}]}`, "at most 100 floors"},
		{`{"territory": "T01", "maps": [{"name": "Blk 5", "kind": "building", "lowest_floor": 1, "top_floor": 60,
			"units": [{"from": "1", "to": "90", "only": null, "codes": null}], "skip": [], "address": null}]}`, "at most 5000"},
		{`{"territory": "ZZ9", "maps": []}`, "no territory has the code"},
	} {
		_, err := proposeMapsCreate.Run(s, json.RawMessage(c.args))
		wantArgError(t, err, c.want)
	}
}
