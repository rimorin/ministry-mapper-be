package assistant

import (
	"slices"
	"strings"
	"unicode"

	"ganigeorgiev.com/pocketbase/dbx"
	"ganigeorgiev.com/pocketbase/pocketbase/core"
)

const (
	// maxLinkPeople is how many people one share link may name; the link's own
	// limit on accounts matches.
	maxLinkPeople = 20
	maxSuggested  = 5
)

// LinkPerson is one person a share link names, with the member accounts the
// name may stand for. Picked is an account chosen without asking; Suggested
// are offered on the card for the admin to add. Both are user ids: the card
// looks the people up itself, so no name or email of theirs travels here.
type LinkPerson struct {
	Name      string   `json:"name"`
	Picked    string   `json:"picked,omitempty"`
	Suggested []string `json:"suggested,omitempty"`
}

// NamePair says which account the admin confirmed for a name they typed, or
// none when User is empty.
type NamePair struct {
	Name string `json:"name"`
	User string `json:"user"`
}

// nameWords reduces a person's name to comparable words: lower case, without
// punctuation or a title such as Bro or Sis.
func nameWords(name string) []string {
	words := strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	return slices.DeleteFunc(words, func(w string) bool { return honorifics[w] })
}

func aliasOf(name string) string { return strings.Join(nameWords(name), " ") }

// The ways a typed name can match a member's, strictest first. Only the
// strictest kind that matches anyone is used, so a member called Ann is not
// joined by an Anna, nor a real Ong by a Wong one typo away.
const (
	matchNone  = iota
	matchExact // the same words, in any order
	matchPart  // some of the member's words: a first name, or "Meiling" for "Mei Ling"
	matchStart // the start of their words: "Chris" for "Christopher"
	matchNear  // a likely typo
)

func matchKind(typed, name []string) int {
	every := func(ok func(typed, word string) bool) bool {
		return !slices.ContainsFunc(typed, func(t string) bool {
			return !slices.ContainsFunc(name, func(w string) bool { return ok(t, w) })
		})
	}
	equal := func(t, w string) bool { return t == w }
	starts := func(t, w string) bool { return len([]rune(t)) >= 3 && strings.HasPrefix(w, t) }
	switch {
	case len(typed) == len(name) && every(equal):
		return matchExact
	case every(equal) || joinedMatch(typed, name) || joinedMatch(name, typed):
		return matchPart
	case every(starts):
		return matchStart
	// wordDistance counts any start as a match, however short; here a start
	// still needs its three letters.
	case every(func(t, w string) bool { return t == w || starts(t, w) || wordDistance(t, w) > 0 }):
		return matchNear
	}
	return matchNone
}

// joinedMatch reports whether the words are all among the others once a run
// of the others is written as one word: "Meiling" for "Mei Ling", a name
// people write both ways. The joined word has to be long enough to mean it.
func joinedMatch(words, others []string) bool {
	for i := range others {
		for j := i + 2; j <= len(others); j++ {
			joined := strings.Join(others[i:j], "")
			if len([]rune(joined)) < 5 || !slices.Contains(words, joined) {
				continue
			}
			rest := slices.Concat(others[:i], []string{joined}, others[j:])
			if !slices.ContainsFunc(words, func(w string) bool { return !slices.Contains(rest, w) }) {
				return true
			}
		}
	}
	return false
}

// matchMembers finds, for each name a link is for, the member accounts it may
// stand for. One account is picked when the admin confirmed it for that name
// before, or when it is the only member the name fits. Several members, or a
// name that fits only as a typo, are suggested for the admin to choose from.
// The card shows every pick before anything is made, which is what lets a
// first name be picked at all.
func matchMembers(s Scope, names []string) ([]LinkPerson, error) {
	var members []struct {
		ID   string `db:"id"`
		Name string `db:"name"`
	}
	err := s.App.DB().NewQuery(`
		SELECT DISTINCT u.id, u.name FROM roles r JOIN users u ON u.id = r.user
		WHERE r.congregation = {:congregation} AND u.disabled = FALSE AND TRIM(COALESCE(u.name, '')) != ''
		ORDER BY u.name, u.id
	`).Bind(dbx.Params{"congregation": s.CongregationID}).All(&members)
	if err != nil {
		return nil, err
	}

	// A pairing only holds while the account is still a member.
	var learned []struct {
		Alias string `db:"alias"`
		User  string `db:"user"`
	}
	err = s.App.DB().NewQuery(`
		SELECT l.alias, l.user FROM assistant_name_links l
		WHERE l.congregation = {:congregation}
		  AND EXISTS (SELECT 1 FROM roles r WHERE r.user = l.user AND r.congregation = l.congregation)
	`).Bind(dbx.Params{"congregation": s.CongregationID}).All(&learned)
	if err != nil {
		return nil, err
	}
	remembered := make(map[string]string, len(learned))
	for _, l := range learned {
		remembered[l.Alias] = l.User
	}

	people := make([]LinkPerson, len(names))
	var picked []string
	for i, name := range names {
		people[i].Name = name
		typed := nameWords(name)
		if len(typed) == 0 {
			continue
		}
		best, hits := matchNone, []string(nil)
		for _, m := range members {
			switch kind := matchKind(typed, nameWords(m.Name)); {
			case kind == matchNone:
			case best == matchNone || kind < best:
				best, hits = kind, []string{m.ID}
			case kind == best:
				hits = append(hits, m.ID)
			}
		}
		pick := remembered[strings.Join(typed, " ")]
		if pick == "" && len(hits) == 1 && best != matchNear {
			pick = hits[0]
		}
		// One account is picked for one name only.
		if pick != "" && !slices.Contains(picked, pick) {
			people[i].Picked = pick
			picked = append(picked, pick)
			continue
		}
		people[i].Suggested = hits[:min(len(hits), maxSuggested)]
	}
	return people, nil
}

// SaveNamePairs remembers which account the admin confirmed for each name on
// a link card, and forgets a name they left without one. Only names the card
// carried are kept, and only for accounts that are members.
func SaveNamePairs(app core.App, congregation string, people []LinkPerson, pairs []NamePair) error {
	collection, err := app.FindCollectionByNameOrId("assistant_name_links")
	if err != nil {
		return err
	}
	for _, pair := range pairs {
		alias := aliasOf(pair.Name)
		if alias == "" || !slices.ContainsFunc(people, func(p LinkPerson) bool { return p.Name == pair.Name }) {
			continue
		}
		params := dbx.Params{"congregation": congregation, "alias": alias, "user": pair.User}
		record, err := app.FindFirstRecordByFilter(collection, "congregation = {:congregation} && alias = {:alias}", params)
		if err != nil {
			record = nil
		}
		if pair.User == "" {
			if record != nil {
				if err := app.Delete(record); err != nil {
					return err
				}
			}
			continue
		}
		var isMember bool
		if err := app.DB().NewQuery(`SELECT EXISTS (SELECT 1 FROM roles WHERE user = {:user} AND congregation = {:congregation})`).
			Bind(params).Row(&isMember); err != nil {
			return err
		}
		if !isMember {
			continue
		}
		if record == nil {
			record = core.NewRecord(collection)
			record.Set("congregation", congregation)
			record.Set("alias", alias)
		}
		record.Set("user", pair.User)
		if err := app.Save(record); err != nil {
			return err
		}
	}
	return nil
}

// ForgetNamePairs drops the pairings a link card saved when its link is
// undone: a link taken back may have gone to the wrong account. A name that
// has since been paired with another account is left alone.
func ForgetNamePairs(app core.App, congregation string, people []LinkPerson, pairs []NamePair) error {
	for _, pair := range pairs {
		alias := aliasOf(pair.Name)
		if alias == "" || pair.User == "" || !slices.ContainsFunc(people, func(p LinkPerson) bool { return p.Name == pair.Name }) {
			continue
		}
		record, err := app.FindFirstRecordByFilter("assistant_name_links",
			"congregation = {:congregation} && alias = {:alias} && user = {:user}",
			dbx.Params{"congregation": congregation, "alias": alias, "user": pair.User})
		if err != nil {
			continue
		}
		if err := app.Delete(record); err != nil {
			return err
		}
	}
	return nil
}
