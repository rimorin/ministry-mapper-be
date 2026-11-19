//go:build testdata

package assistant

import (
	"slices"
	"strings"
	"testing"

	"ganigeorgiev.com/pocketbase/dbx"
)

func TestMatchKind(t *testing.T) {
	ong := member{id: "u1", name: nameWords("Ong Mei Ling"), email: nameWords("meiling.ong88")}
	for _, c := range []struct {
		typed string
		want  int
	}{
		{"Mei Ling Ong", matchSameWords},
		{"Sis Ong", matchWholeWords},
		{"mei ling", matchWholeWords},
		{"Mei L", matchNone}, // a start needs three letters
		{"Mei Lin", matchWordStarts},
		{"meiling", matchJoined},
		{"Meiling Ong", matchJoined},
		{"Meiling Tan", matchNone},
		{"ong88", matchEmail},
		{"Ong Mei Lang", matchNearMiss},
		{"Wong", matchNearMiss},
		{"Tan", matchNone},
		{"Ong Tan", matchNone},
	} {
		if got := matchKind(nameWords(c.typed), ong); got != c.want {
			t.Errorf("matchKind(%q) = %d, want %d", c.typed, got, c.want)
		}
	}
}

func TestMatchMembers(t *testing.T) {
	app := newTestApp(t)
	s := scopeFor(t, app, alphaCong)
	const publisher = "testuseralpha04"

	match := func(names ...string) []LinkPerson {
		t.Helper()
		people, err := matchMembers(s, names)
		if err != nil {
			t.Fatal(err)
		}
		return people
	}

	// A full name that fits one member is picked; one word is only suggested,
	// however few members it fits.
	people := match("Alpha Publisher", "Sis Publisher", "Alpha", "Alpha Publsher", "Sis Ong", "Alpha Read")
	if people[0].Picked != publisher {
		t.Errorf("full name: %+v", people[0])
	}
	if people[1].Picked != "" || !slices.Equal(people[1].Suggested, []string{publisher}) {
		t.Errorf("one word: %+v", people[1])
	}
	if people[2].Picked != "" || len(people[2].Suggested) != 4 {
		t.Errorf("a word four members share: %+v", people[2])
	}
	if people[3].Picked != "" || !slices.Equal(people[3].Suggested, []string{publisher}) {
		t.Errorf("a typo is never picked outright: %+v", people[3])
	}
	if people[4].Picked != "" || people[4].Suggested != nil {
		t.Errorf("nobody's name: %+v", people[4])
	}
	if people[5].Picked != "" || !slices.Equal(people[5].Suggested, []string{"testuseralpha03"}) {
		t.Errorf("part of a name is never picked outright: %+v", people[5])
	}

	// Of several members a name fits, the one last given a map comes first.
	exec(t, app, `INSERT INTO assignments_log (id, congregation, action, assignees, created)
		VALUES ('testgivenlog001', {:congregation}, 'assigned', '["testuseralpha03"]', '2026-11-01 00:00:00.000Z')`,
		dbx.Params{"congregation": alphaCong})
	if first := match("Alpha")[0].Suggested[0]; first != "testuseralpha03" {
		t.Errorf("first suggestion = %q, want the member last given a map", first)
	}

	// One account is picked for one name only.
	twice := match("Alpha Publisher", "Publisher Alpha")
	if twice[0].Picked != publisher || twice[1].Picked != "" || !slices.Equal(twice[1].Suggested, []string{publisher}) {
		t.Errorf("same account twice: %+v", twice)
	}

	// Another congregation's members are never offered.
	if other := match("Beta Admin")[0]; other.Picked != "" || other.Suggested != nil {
		t.Errorf("another congregation's member: %+v", other)
	}
}

func TestSaveNamePairs_RemembersWhatTheAdminConfirmed(t *testing.T) {
	app := newTestApp(t)
	s := scopeFor(t, app, alphaCong)
	const publisher = "testuseralpha04"
	card := []LinkPerson{{Name: "Sis Ong"}, {Name: "John"}}
	picked := func(name string) string {
		t.Helper()
		people, err := matchMembers(s, []string{name})
		if err != nil {
			t.Fatal(err)
		}
		return people[0].Picked
	}
	save := func(pairs ...NamePair) {
		t.Helper()
		if err := SaveNamePairs(app, alphaCong, card, pairs); err != nil {
			t.Fatal(err)
		}
	}

	save(NamePair{"Sis Ong", publisher},
		NamePair{"John", "testuserbeta001"}, // not a member here
		NamePair{"Someone Else", publisher}) // not a name on the card
	if got := picked("sis ong"); got != publisher {
		t.Errorf("confirmed pairing picked %q", got)
	}
	// The card is told the pick rests on an earlier choice, not on the name.
	if people, _ := matchMembers(s, []string{"Sis Ong", "Alpha Publisher"}); !people[0].Remembered || people[1].Remembered {
		t.Errorf("remembered = %v, %v; want only the first", people[0].Remembered, people[1].Remembered)
	}

	// Undoing the link forgets what its card saved, unless the name has since
	// been paired with someone else.
	forget := func(pairs ...NamePair) {
		t.Helper()
		if err := ForgetNamePairs(app, alphaCong, card, pairs); err != nil {
			t.Fatal(err)
		}
	}
	forget(NamePair{"Sis Ong", "testuseralpha03"})
	if got := picked("Sis Ong"); got != publisher {
		t.Errorf("another card's undo dropped the pairing: %q", got)
	}
	forget(NamePair{"Sis Ong", publisher})
	if got := picked("Sis Ong"); got != "" {
		t.Errorf("an undone pairing still picked %q", got)
	}
	save(NamePair{"Sis Ong", publisher})
	if picked("John") != "" || picked("Someone Else") != "" {
		t.Error("a pairing was kept for a non-member or a name the card did not carry")
	}

	// Confirming again with another account replaces it; none forgets it.
	save(NamePair{"Sis Ong", "testuseralpha03"})
	if got := picked("Sis Ong"); got != "testuseralpha03" {
		t.Errorf("replaced pairing picked %q", got)
	}
	save(NamePair{"Sis Ong", ""})
	if got := picked("Sis Ong"); got != "" {
		t.Errorf("forgotten pairing still picked %q", got)
	}

	// A member whose full name was typed outranks a pairing with someone
	// else: neither is picked, and the remembered one is offered first.
	card = append(card, LinkPerson{Name: "Alpha Publisher"})
	save(NamePair{"Alpha Publisher", "testuseralpha03"})
	if people, _ := matchMembers(s, []string{"Alpha Publisher"}); people[0].Picked != "" ||
		!slices.Equal(people[0].Suggested, []string{"testuseralpha03", publisher}) {
		t.Errorf("full name against an older pairing: %+v", people[0])
	}
	save(NamePair{"Alpha Publisher", ""})

	// A pairing stops counting once the account leaves the congregation.
	save(NamePair{"Sis Ong", publisher})
	exec(t, app, `DELETE FROM roles WHERE user = {:user}`, dbx.Params{"user": publisher})
	if got := picked("Sis Ong"); got != "" {
		t.Errorf("a former member was picked: %q", got)
	}
}

func TestProposeShareLink_MatchesAccountsOnlyWhereSwitchedOn(t *testing.T) {
	app := newTestApp(t)
	s := adminScope(t, alphaCong, "testuseralpha01")
	args := `{"map": "Blk 100B", "publishers": ["Alpha Publisher", "Sis Ong"], "type": "normal"}`

	if p := propose(t, s, proposeShareLink, args); p.People != nil || p.Value != "Alpha Publisher, Sis Ong" {
		t.Fatalf("switched off: %+v", p)
	}
	s.MemberLinks = true
	p := propose(t, s, proposeShareLink, args)
	if len(p.People) != 2 || p.People[0].Picked != "testuseralpha04" || p.People[1].Picked != "" {
		t.Fatalf("switched on: %+v", p.People)
	}
	// The model learns how the names fared, never whose account is whose.
	view := proposalView(p)
	for _, want := range []string{`"people":2`, `"account_matched":1`, `"account_to_check_on_card":0`} {
		if !strings.Contains(view, want) {
			t.Errorf("model view %s lacks %s", view, want)
		}
	}
	if strings.Contains(view, "testuseralpha04") || strings.Contains(view, "Alpha Publisher") {
		t.Errorf("model view names an account: %s", view)
	}
	_ = app
}
