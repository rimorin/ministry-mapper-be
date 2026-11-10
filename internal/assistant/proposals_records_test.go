//go:build testdata

package assistant

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProposeMapRename(t *testing.T) {
	s := adminScope(t, alphaCong, "testuseralpha01")
	p := propose(t, s, proposeMapRename, `{"map": "blk 100a", "territory": "T01", "new_name": "  Blk 100 North "}`)
	if p.Kind != "map_rename" || p.From != "Blk 100A" || p.To != "Blk 100 North" ||
		p.Value != "Blk 100 North" || p.Previous != "Blk 100A" {
		t.Fatalf("proposal = %+v", p)
	}
	if p.Target == nil || p.Target.ID != "testmapalpha01a" || p.Target.TerritoryID != "testterralpha01" {
		t.Fatalf("target = %+v", p.Target)
	}

	_, err := proposeMapRename.Run(s, json.RawMessage(`{"map": "Blk 100A", "new_name": "Blk 100A"}`))
	wantArgError(t, err, "already named")
	_, err = proposeMapRename.Run(s, json.RawMessage(`{"map": "Blk 300A", "new_name": "Mine now"}`))
	wantArgError(t, err, "no map is named")
	_, err = proposeMapRename.Run(s, json.RawMessage(`{"map": "", "new_name": "Mine now"}`))
	wantArgError(t, err, "map is required")
	_, err = proposeMapRename.Run(s, json.RawMessage(`{"map": "Blk 100A", "new_name": "   "}`))
	wantArgError(t, err, "new_name must be")
}

func TestProposeMapRename_KeepsOtherLanguages(t *testing.T) {
	s := adminScope(t, alphaCong, "testuseralpha01")
	s.Locale = "zh"
	p := propose(t, s, proposeMapRename, `{"map": "富地图", "new_name": "新地图"}`)
	if p.From != "富地图" || p.Previous != `{"en":"Rich Map","zh":"富地图"}` {
		t.Fatalf("proposal = %+v", p)
	}
	var stored map[string]string
	if err := json.Unmarshal([]byte(p.Value), &stored); err != nil || stored["zh"] != "新地图" || stored["en"] != "Rich Map" {
		t.Fatalf("value = %s, want only the Chinese name changed", p.Value)
	}

	// A language the map has no name in changes the English one.
	if got := renamed(`{"en":"Rich Map","zh":"富地图"}`, "Big Map", "ta"); got != `{"en":"Big Map","zh":"富地图"}` {
		t.Errorf("renamed = %s", got)
	}
	if got := renamed("Blk 1", "Blk 2", "zh"); got != "Blk 2" {
		t.Errorf("renamed = %s", got)
	}
}

func TestProposeTerritoryRename(t *testing.T) {
	s := adminScope(t, alphaCong, "testuseralpha01")
	p := propose(t, s, proposeTerritoryRename, `{"territory": "t01", "new_name": "North Side"}`)
	if p.Kind != "territory_rename" || p.From != "Alpha Territory 01" || p.To != "North Side" ||
		p.Target == nil || p.Target.ID != "testterralpha01" || p.Target.Name != "T01" {
		t.Fatalf("proposal = %+v", p)
	}
	_, err := proposeTerritoryRename.Run(s, json.RawMessage(`{"territory": "ZZ9", "new_name": "North"}`))
	wantArgError(t, err, "no territory has the code")
	_, err = proposeTerritoryRename.Run(s, json.RawMessage(`{"territory": "T01", "new_name": "Alpha Territory 01"}`))
	wantArgError(t, err, "already named")
}

func TestProposeShareLink(t *testing.T) {
	s := adminScope(t, alphaCong, "testuseralpha01")

	normal := propose(t, s, proposeShareLink, `{"map": "Blk 100B", "publisher": "Sis Ong", "type": "normal"}`)
	if normal.Kind != "share_link" || normal.LinkType != "normal" || normal.Hours != 24 ||
		normal.Value != "Sis Ong" || len(normal.Warnings) != 0 || normal.Target.ID != "testmapalpha01b" {
		t.Fatalf("normal link = %+v", normal)
	}

	// The model refers to a known publisher by token; the card shows the name.
	token := newPseudonyms(alphaCong).token("Test Publisher Alpha")
	personal := propose(t, s, proposeShareLink, `{"map": "Blk 100A", "publisher": "`+token+`", "type": "personal", "days": 14}`)
	if personal.Value != "Test Publisher Alpha" || personal.Hours != 14*24 ||
		strings.Join(personal.Warnings, ",") != "already_assigned" {
		t.Fatalf("personal link = %+v", personal)
	}

	_, err := proposeShareLink.Run(s, json.RawMessage(`{"map": "Blk 100A", "publisher": "Sis Ong", "type": "personal"}`))
	wantArgError(t, err, "needs days")
	_, err = proposeShareLink.Run(s, json.RawMessage(`{"map": "Blk 100A", "publisher": "Sis Ong", "type": "personal", "days": 400}`))
	wantArgError(t, err, "needs days")
	// Another congregation's publisher is not known here.
	other := newPseudonyms(alphaCong).token("Test Publisher Beta")
	_, err = proposeShareLink.Run(s, json.RawMessage(`{"map": "Blk 100A", "publisher": "`+other+`", "type": "normal"}`))
	wantArgError(t, err, "write the publisher's name")
}

func TestProposePinMessage(t *testing.T) {
	s := adminScope(t, alphaCong, "testuseralpha01")
	p := propose(t, s, proposePinMessage, `{"map": "Blk 100A", "message": " Knock softly after 9pm "}`)
	if p.Kind != "pin_message" || p.Value != "Knock softly after 9pm" ||
		strings.Join(p.Warnings, ",") != "emails_everyone" || p.Target.ID != "testmapalpha01a" {
		t.Fatalf("proposal = %+v", p)
	}
	_, err := proposePinMessage.Run(s, json.RawMessage(`{"map": "Blk 100A", "message": ""}`))
	wantArgError(t, err, "message must be")
}

func TestProposeReportEmail(t *testing.T) {
	p := propose(t, adminScope(t, alphaCong, "testuseralpha01"), proposeReportEmail, `{}`)
	if p.Kind != "report_email" || p.Target != nil {
		t.Fatalf("proposal = %+v", p)
	}
}

func TestProposalView_HidesWhatIsStored(t *testing.T) {
	s := adminScope(t, alphaCong, "testuseralpha01")
	p := propose(t, s, proposeShareLink, `{"map": "Blk 100B", "publisher": "Sis Ong", "type": "normal"}`)
	if view := proposalView(p); strings.Contains(view, "Sis Ong") || strings.Contains(view, "testmapalpha01b") {
		t.Fatalf("the model's view must not hold the publisher or ids: %s", view)
	}
}
