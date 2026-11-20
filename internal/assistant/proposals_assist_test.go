//go:build testdata

package assistant

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestProposeMarkRead(t *testing.T) {
	s := adminScope(t, alphaCong, "testuseralpha01")
	p := propose(t, s, proposeMarkRead, `{"map": "Blk 100A"}`)
	if p.Kind != "mark_read" || len(p.Items) != 1 || p.Items[0].ID != "testmsgalpha01a" || p.Items[0].Label != "Blk 100A" {
		t.Fatalf("proposal = %+v; only the publisher's unread message, not the admin's own", p)
	}
	_, err := proposeMarkRead.Run(s, json.RawMessage(`{"map": "Blk 200A"}`))
	wantArgError(t, err, "no unread messages")
}

func TestProposeUnpinMessage(t *testing.T) {
	s := adminScope(t, alphaCong, "testuseralpha01")
	p := propose(t, s, proposeUnpinMessage, `{"map": "Blk 100A"}`)
	if p.Kind != "unpin_message" || len(p.Items) != 1 || p.Items[0].ID != "testmsgalphapin1" || p.Items[0].Label != "Pinned admin notice" {
		t.Fatalf("proposal = %+v", p)
	}
	_, err := proposeUnpinMessage.Run(s, json.RawMessage(`{"map": "Blk 200A"}`))
	wantArgError(t, err, "no pinned instruction")
}

func TestProposeMapLocation(t *testing.T) {
	s := adminScope(t, alphaCong, "testuseralpha01")
	_, err := proposeMapLocation.Run(s, json.RawMessage(`{"map": "Rich Map"}`))
	if !errors.As(err, &needLocationError{}) {
		t.Fatalf("err = %v, want the admin asked for their location", err)
	}

	s.Origin = &Location{Lat: 1.30, Lng: 103.80}
	p := propose(t, s, proposeMapLocation, `{"map": "Rich Map"}`)
	if p.Kind != "map_location" || p.Target.ID != "testmapalphrich1" || p.Previous == "" {
		t.Fatalf("proposal = %+v", p)
	}
	old, _ := parsePoint(p.Previous)
	far := distanceMeters(old, *s.Origin) > farMeters
	if far != strings.Contains(strings.Join(p.Warnings, ","), "moves_far") {
		t.Fatalf("warnings = %v for a move of %.0f m", p.Warnings, distanceMeters(old, *s.Origin))
	}

	// Standing on the old spot moves it nowhere, so no warning.
	s.Origin = &old
	if near := propose(t, s, proposeMapLocation, `{"map": "Rich Map"}`); len(near.Warnings) != 0 {
		t.Fatalf("warnings = %v", near.Warnings)
	}
	if view := proposalView(p); strings.Contains(view, "103.8") {
		t.Fatalf("the model must not see the location: %s", view)
	}
}

func TestProposeLinkRevoke(t *testing.T) {
	s := adminScope(t, alphaCong, "testuseralpha01")
	p := propose(t, s, proposeLinkRevoke, `{"map": "Blk 100A"}`)
	if p.Kind != "link_revoke" || len(p.Items) != 1 || p.Items[0].ID != "testassignalpha01" ||
		p.Items[0].Label != "Test Publisher Alpha" || strings.Join(p.Warnings, ",") != "stops_now" {
		t.Fatalf("proposal = %+v; the expired link must not be listed", p)
	}
	token := newPseudonyms(alphaCong).token("Test Publisher Alpha")
	if byToken := propose(t, s, proposeLinkRevoke, `{"map": "Blk 100A", "publisher": "`+token+`"}`); len(byToken.Items) != 1 {
		t.Fatalf("by token = %+v", byToken)
	}
	_, err := proposeLinkRevoke.Run(s, json.RawMessage(`{"map": "Blk 100A", "publisher": "Sis Ong"}`))
	wantArgError(t, err, "no active link from that publisher")

	s.Role = "conductor"
	propose(t, s, proposeLinkRevoke, `{"map": "Blk 100A"}`)
}

func TestProposeQuickLink(t *testing.T) {
	app := newTestApp(t)
	s := scopeFor(t, app, alphaCong)
	// Run does not check roles; the policy keeps Quick Link to conductors,
	// as the screen does.
	s.Role, s.UserID = "read_only", "testuseralpha03"
	_, err := proposeQuickLink.Run(s, json.RawMessage(`{"territory": "T01", "publishers": ["Sis Ong"], "near_me": true}`))
	if !errors.As(err, &needLocationError{}) {
		t.Fatalf("err = %v, want the admin asked for their location", err)
	}
	s.Origin = &Location{Lat: 1.30, Lng: 103.80}
	p := propose(t, s, proposeQuickLink, `{"territory": "t01", "publishers": ["Sis Ong"], "near_me": true}`)
	if p.Kind != "quick_link" || p.Target.ID != "testterralpha01" || p.Target.Name != "T01" || p.Value != "Sis Ong" ||
		p.Data["near_me"] != true || p.People != nil {
		t.Fatalf("proposal = %+v", p)
	}

	// Planned ahead, it needs no position, and a group goes on one link with
	// the accounts its names fit.
	s.Origin, s.MemberLinks = nil, true
	group := propose(t, s, proposeQuickLink, `{"territory": "T01", "publishers": ["Alpha Publisher", "Sis Ong"], "near_me": false}`)
	if group.Value != "Alpha Publisher, Sis Ong" || group.Data["near_me"] != false ||
		len(group.People) != 2 || group.People[0].Picked != "testuseralpha04" || group.People[1].Picked != "" {
		t.Fatalf("group = %+v", group)
	}
	if _, has := group.Data["origin"]; has || group.Data["together"] != nil {
		t.Fatalf("a plain card carries a place or a group: %+v", group.Data)
	}

	// Starting from a named place, the card carries where that is and the
	// model's copy does not; kept together is passed on as asked.
	placeMaps(t, app)
	near := propose(t, s, proposeQuickLink,
		`{"territory": "T01", "publishers": ["Sis Ong"], "near_me": false, "near_place": "Blk 100A", "together": true}`)
	origin, _ := near.Data["origin"].(map[string]float64)
	if near.Data["near"] != "Blk 100A" || near.Data["together"] != true || origin["lat"] == 0 || origin["lng"] == 0 {
		t.Fatalf("near = %+v", near.Data)
	}
	if view := proposalView(near); strings.Contains(view, "lat") || strings.Contains(view, "origin") {
		t.Fatalf("the model was shown the place's position: %s", view)
	}
	for _, bad := range []string{
		`{"territory": "T01", "publishers": ["Sis Ong"], "near_me": true, "near_place": "Blk 100A"}`,
		`{"territory": "T01", "publishers": ["Sis Ong"], "near_me": false, "near_place": "Nowhere 9"}`,
	} {
		if _, err := proposeQuickLink.Run(s, json.RawMessage(bad)); !errors.As(err, &argError{}) {
			t.Fatalf("%s: err = %v, want it refused", bad, err)
		}
	}
}

func homeStates(t *testing.T, p Proposal) (before, after homeState) {
	t.Helper()
	if err := json.Unmarshal([]byte(p.Previous), &before); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(p.Value), &after); err != nil {
		t.Fatal(err)
	}
	return before, after
}

func TestProposeHomeUpdate(t *testing.T) {
	s := adminScope(t, alphaCong, "testuseralpha01")
	token := newPseudonyms(alphaCong).token("Test Publisher Alpha")
	p := propose(t, s, proposeHomeUpdate, `{"map": "Blk 100A", "unit": "12", "status": "done",
		"notes_action": "add", "notes": "Ask `+token+` first", "household_type": "language note"}`)
	if p.Kind != "home_update" || len(p.Items) != 1 || p.Items[0].ID != "testalpha01a003" || p.Items[0].Label != "12" ||
		p.From != "not_home" || p.To != "done" {
		t.Fatalf("proposal = %+v", p)
	}
	before, after := homeStates(t, p)
	if strings.Join(before.Types, ",") != "testoptialpha01" || strings.Join(before.TypeNames, ",") != "Not Home" {
		t.Fatalf("before = %+v", before)
	}
	if after.Status != "done" || after.Notes != "Ask Test Publisher Alpha first" ||
		strings.Join(after.Types, ",") != "testoptialpha03" || strings.Join(after.TypeNames, ",") != "Language Note" {
		t.Fatalf("after = %+v; the note must say the name, not the token", after)
	}
	if view := proposalView(p); strings.Contains(view, "Test Publisher") || strings.Contains(view, "Ask") {
		t.Fatalf("the model must not see the note: %s", view)
	}

	dnc := propose(t, s, proposeHomeUpdate, `{"map": "Blk 100A", "unit": "10", "status": "do_not_call"}`)
	if _, after := homeStates(t, dnc); after.DncTime == "" {
		t.Fatalf("a home marked do not call needs its time: %+v", after)
	}
	notHome := propose(t, s, proposeHomeUpdate, `{"map": "Blk 100A", "unit": "10", "status": "not_home"}`)
	if _, after := homeStates(t, notHome); after.Tries != 1 {
		t.Fatalf("tries = %d, want 1 for a first nobody-home visit", after.Tries)
	}
}

func TestProposeHomeUpdate_FindsTheRightHome(t *testing.T) {
	s := adminScope(t, alphaCong, "testuseralpha01")
	_, err := proposeHomeUpdate.Run(s, json.RawMessage(`{"map": "Multi Floor Blk", "unit": "1", "status": "done"}`))
	wantArgError(t, err, "several floors")
	p := propose(t, s, proposeHomeUpdate, `{"map": "Multi Floor Blk", "unit": "#02-1", "status": "done"}`)
	if p.Items[0].ID != "testalphcf01003" || p.Items[0].Label != "#02-01" {
		t.Fatalf("item = %+v", p.Items[0])
	}
	_, err = proposeHomeUpdate.Run(s, json.RawMessage(`{"map": "Blk 100A", "unit": "99", "status": "done"}`))
	wantArgError(t, err, "has no home")
	_, err = proposeHomeUpdate.Run(s, json.RawMessage(`{"map": "Blk 100A", "unit": "14", "status": "done"}`))
	wantArgError(t, err, "already has those details")
	_, err = proposeHomeUpdate.Run(s, json.RawMessage(`{"map": "Blk 100A", "unit": "14"}`))
	wantArgError(t, err, "say what changes")
	_, err = proposeHomeUpdate.Run(s, json.RawMessage(`{"map": "Blk 100A", "unit": "14", "household_type": "Martian"}`))
	wantArgError(t, err, "no one household type")
	_, err = proposeHomeUpdate.Run(s, json.RawMessage(`{"map": "Blk 100A", "unit": "14", "notes_action": "add"}`))
	wantArgError(t, err, "notes is required")
	// Part of a type's name is enough when it fits one, as it is when listing homes.
	typed := propose(t, s, proposeHomeUpdate, `{"map": "Blk 100A", "unit": "14", "household_type": "language"}`)
	if _, after := homeStates(t, typed); len(after.TypeNames) != 1 || after.TypeNames[0] != "Language Note" {
		t.Fatalf("household type by part of its name: %+v", after)
	}
}

// A home is found however it is written, basements included: what one
// function prints, the others accept.
func TestUnits_AreReadTheSameEverywhere(t *testing.T) {
	app := newTestApp(t)
	exec(t, app, `UPDATE addresses SET floor = -1 WHERE id = 'testalphcf01003'`, nil)
	exec(t, app, `UPDATE addresses SET code = '12-A' WHERE id = 'testalpha01a001'`, nil)
	s := scopeFor(t, app, alphaCong)
	s.Role, s.UserID = "administrator", "testuseralpha01"

	basement := propose(t, s, proposeHomeUpdate, `{"map": "Multi Floor Blk", "unit": "#-1-1", "status": "done"}`)
	if basement.Items[0].ID != "testalphcf01003" {
		t.Fatalf("basement home: %+v", basement.Items[0])
	}
	// A street's own hyphenated number is a code, not a floor and a unit.
	street := propose(t, s, proposeHomeUpdate, `{"map": "Blk 100A", "unit": "12-a", "status": "done"}`)
	if street.Items[0].ID != "testalpha01a001" {
		t.Fatalf("hyphenated house number: %+v", street.Items[0])
	}
	if r := run(t, s, homesTool, `{"map": "Multi Floor Blk", "unit": "#-01-01", "group_by": "home"}`); len(r.Rows) != 1 {
		t.Fatalf("homes by a printed unit: %d rows", len(r.Rows))
	}
	// With no floors named, every floor counts, below ground too.
	all := propose(t, s, proposeHomesStatus, `{"map": "Multi Floor Blk", "status": "done"}`)
	if !slices.ContainsFunc(all.Items, func(it Item) bool { return it.ID == "testalphcf01003" }) {
		t.Fatalf("the basement home was left out: %+v", all.Items)
	}
	one := propose(t, s, proposeHomesStatus, `{"map": "Multi Floor Blk", "status": "done", "units": ["#-1-1"]}`)
	if len(one.Items) != 1 || one.Items[0].ID != "testalphcf01003" {
		t.Fatalf("a unit named with its floor: %+v", one.Items)
	}
}

func TestProposeHomeUpdate_WarnsWhenLeavingDoNotCall(t *testing.T) {
	app := newTestApp(t)
	exec(t, app, `UPDATE addresses SET status = 'do_not_call', dnc_time = '2024-01-01 00:00:00.000Z' WHERE id = 'testalpha01a001'`, nil)
	s := scopeFor(t, app, alphaCong)
	s.Role, s.UserID = "administrator", "testuseralpha01"
	p := propose(t, s, proposeHomeUpdate, `{"map": "Blk 100A", "unit": "10", "status": "not_done"}`)
	if strings.Join(p.Warnings, ",") != "leaves_do_not_call" {
		t.Fatalf("warnings = %v", p.Warnings)
	}
	if before, _ := homeStates(t, p); before.DncTime == "" {
		t.Fatalf("the old do-not-call time must be kept for Undo: %+v", before)
	}
}
