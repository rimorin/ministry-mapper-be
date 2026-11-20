//go:build testdata

package assistant

import (
	"slices"
	"strings"
	"testing"

	"ganigeorgiev.com/pocketbase/dbx"
)

func TestMatchKind(t *testing.T) {
	ong := nameWords("Ong Mei Ling")
	for _, c := range []struct {
		typed string
		want  int
	}{
		{"Mei Ling Ong", matchExact},
		{"Sis Ong", matchPart},
		{"mei ling", matchPart},
		{"meiling", matchPart},
		{"Meiling Ong", matchPart},
		{"Meiling Tan", matchNone},
		{"Mei L", matchNone}, // a start needs three letters
		{"Mei Lin", matchStart},
		{"Ong Mei Lang", matchNear},
		{"Wong", matchNear},
		{"Tan", matchNone},
		{"Ong Tan", matchNone},
	} {
		if got := matchKind(nameWords(c.typed), ong); got != c.want {
			t.Errorf("matchKind(%q) = %d, want %d", c.typed, got, c.want)
		}
	}
}

func TestMatchMembers(t *testing.T) {
	s := scopeFor(t, newTestApp(t), alphaCong)
	const publisher = "testuseralpha04"

	match := func(names ...string) []LinkPerson {
		t.Helper()
		people, err := matchMembers(s, names)
		if err != nil {
			t.Fatal(err)
		}
		return people
	}

	// The only member a name fits is picked, whether it is their full name,
	// one word of it or the start of it.
	for name, want := range map[string]string{
		"Alpha Publisher": publisher,
		"Sis Publisher":   publisher,
		"Alpha Read":      "testuseralpha03",
	} {
		if got := match(name)[0]; got.Picked != want || got.Suggested != nil {
			t.Errorf("%q: %+v, want %s picked", name, got, want)
		}
	}
	// Several members, or a likely typo, are left to the admin.
	if shared := match("Alpha")[0]; shared.Picked != "" || len(shared.Suggested) != 4 {
		t.Errorf("a word four members share: %+v", shared)
	}
	if typo := match("Alpha Publsher")[0]; typo.Picked != "" || !slices.Equal(typo.Suggested, []string{publisher}) {
		t.Errorf("a typo is never picked outright: %+v", typo)
	}
	if nobody := match("Sis Ong")[0]; nobody.Picked != "" || nobody.Suggested != nil {
		t.Errorf("nobody's name: %+v", nobody)
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

	// What the admin confirmed outranks what the name fits.
	card = append(card, LinkPerson{Name: "Alpha Publisher"})
	save(NamePair{"Alpha Publisher", "testuseralpha03"})
	if got := picked("Alpha Publisher"); got != "testuseralpha03" {
		t.Errorf("a confirmed pairing lost to the name: %q", got)
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
	// The model learns how each name fared, never whose account is whose.
	view := proposalView(p)
	if want := `"accounts":["matched","none"]`; !strings.Contains(view, want) {
		t.Errorf("model view %s lacks %s", view, want)
	}
	if strings.Contains(view, "testuseralpha04") || strings.Contains(view, "Alpha Publisher") {
		t.Errorf("model view names an account: %s", view)
	}
	_ = app
}
