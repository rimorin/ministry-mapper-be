//go:build testdata

package assistant

import (
	"strings"
	"testing"

	"ganigeorgiev.com/pocketbase/dbx"
)

func TestMapDetails(t *testing.T) {
	app := newTestApp(t)
	s := scopeFor(t, app, alphaCong)
	exec(t, app, `UPDATE addresses SET not_home_tries = 3 WHERE id = 'testalpha01a003'`, nil)
	exec(t, app, `UPDATE addresses SET notes = 'Dog' WHERE id = 'testalpha01a001'`, nil)

	r := run(t, s, mapDetails, `{"map": "Blk 100A"}`)
	row := r.Rows[0]
	// Blk 100A: 10 and 11 not done, 12 and 13 nobody home (12 at the limit
	// of 3), 14 done. Only 12 and 13 have a countable household type.
	if row["homes"] != 5 || row["not_counted"] != 3 || row["not_home"] != 2 || row["at_try_limit"] != 1 {
		t.Fatalf("homes = %v", row)
	}
	if row["progress"] != 50 || row["units_per_floor"] != 5 || row["homes_with_notes"] != 1 {
		t.Fatalf("progress and units = %v", row)
	}
	if row["territory"] != "T01" || row["active_links"] != 1 || row["pinned_instructions"] != 1 {
		t.Fatalf("territory, links, pinned = %v", row)
	}
	if pinned := row["pinned"].([]string); len(pinned) != 1 || pinned[0] != "Pinned admin notice" {
		t.Fatalf("pinned = %v", row["pinned"])
	}
	links, ok := row["links"].([]struct {
		Publisher string `db:"publisher" json:"publisher"`
		Expires   string `db:"expires" json:"expires"`
	})
	if !ok || len(links) != 1 || links[0].Publisher != "Test Publisher Alpha" {
		t.Fatalf("administrators see who holds the map: %v", row["links"])
	}

	// A read-only helper sees how many links, not who holds them.
	s.Role = roleReadOnly
	if row := run(t, s, mapDetails, `{"map": "Blk 100A"}`).Rows[0]; row["links"] != nil || row["active_links"] != 1 {
		t.Fatalf("read-only row = %v", row)
	}

	// The model sees the figures, never the instruction text or the names.
	_, view, err := runTool(s, newPseudonyms(alphaCong), "map_details", `{"map": "Blk 100A"}`)
	if err != nil || strings.Contains(view, "Pinned admin notice") || strings.Contains(view, "Test Publisher") {
		t.Fatalf("view = %s, err = %v", view, err)
	}
}

func TestMapDetails_FloorsOfABuilding(t *testing.T) {
	app := newTestApp(t)
	s := scopeFor(t, app, alphaCong)
	// Rich Map is a building; give it a second floor.
	exec(t, app, `INSERT INTO addresses (id, map, territory, congregation, code, floor, sequence, status)
		SELECT 'testrichfloor2', map, territory, congregation, 'R01', 2, 1, 'not_done' FROM addresses
		WHERE id = 'testalpharich01'`, dbx.Params{})
	row := run(t, s, mapDetails, `{"map": "Rich Map"}`).Rows[0]
	if row["type"] != "multi" || row["floors"] != "1 to 2" {
		t.Fatalf("building = %v", row)
	}
	if row["has_location"] != true || row["lat"] == nil {
		t.Fatalf("location = %v", row)
	}
}
