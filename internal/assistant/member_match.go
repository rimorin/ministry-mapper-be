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
// Remembered marks a pick that rests on what the admin chose before and not
// on the name itself, so the card can say so.
type LinkPerson struct {
	Name       string   `json:"name"`
	Picked     string   `json:"picked,omitempty"`
	Remembered bool     `json:"remembered,omitempty"`
	Suggested  []string `json:"suggested,omitempty"`
}

// NamePair says which account the admin confirmed for a name they typed, or
// none when User is empty.
type NamePair struct {
	Name string `json:"name"`
	User string `json:"user"`
}

type member struct {
	id          string
	name, email []string
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

// The ways a typed name can match a member, strictest first. Only the first
// kind that matches anyone is used, so a real Ong is never joined by a Wong
// that is one typo away.
const (
	matchNone = iota
	matchSameWords
	matchWholeWords
	matchJoined
	matchWordStarts
	matchEmail
	matchNearMiss
)

func matchKind(typed []string, m member) int {
	every := func(words []string, ok func(typed, word string) bool) bool {
		return len(words) > 0 && !slices.ContainsFunc(typed, func(t string) bool {
			return !slices.ContainsFunc(words, func(w string) bool { return ok(t, w) })
		})
	}
	equal := func(t, w string) bool { return t == w }
	starts := func(t, w string) bool { return len([]rune(t)) >= 3 && strings.HasPrefix(w, t) }
	switch {
	case len(typed) == len(m.name) && every(m.name, equal):
		return matchSameWords
	case every(m.name, equal):
		return matchWholeWords
	case joinedMatch(typed, m.name) || joinedMatch(m.name, typed):
		return matchJoined
	case every(m.name, starts):
		return matchWordStarts
	case every(m.email, starts):
		return matchEmail
	// wordDistance counts any start as a match, however short; here a start
	// still needs its three letters.
	case every(m.name, func(t, w string) bool { return t == w || starts(t, w) || wordDistance(t, w) > 0 }):
		return matchNearMiss
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
// stand for. An account is picked outright only where a wrong guess is
// unlikely: a pairing the admin confirmed before, or the member's full name of
// two words or more. Anything looser is only suggested, most recently given a
// map first: a first name, or part of a name, often fits one member with an
// account and several people without. A count of real links showed more than
// half of such lone matches had a namesake in the same congregation.
func matchMembers(s Scope, names []string) ([]LinkPerson, error) {
	var rows []struct {
		ID    string `db:"id"`
		Name  string `db:"name"`
		Email string `db:"email"`
	}
	err := s.App.DB().NewQuery(`
		SELECT DISTINCT u.id, u.name, u.email FROM roles r JOIN users u ON u.id = r.user
		WHERE r.congregation = {:congregation} AND u.disabled = FALSE AND TRIM(COALESCE(u.name, '')) != ''
	`).Bind(dbx.Params{"congregation": s.CongregationID}).All(&rows)
	if err != nil {
		return nil, err
	}
	members := make([]member, len(rows))
	for i, r := range rows {
		local, _, _ := strings.Cut(r.Email, "@")
		members[i] = member{id: r.ID, name: nameWords(r.Name), email: nameWords(local)}
	}

	var learned []struct {
		Alias string `db:"alias"`
		User  string `db:"user"`
	}
	err = s.App.DB().NewQuery(`SELECT alias, user FROM assistant_name_links WHERE congregation = {:congregation}`).
		Bind(dbx.Params{"congregation": s.CongregationID}).All(&learned)
	if err != nil {
		return nil, err
	}

	// When each member was last given a map, to offer the likelier ones first.
	var given []struct {
		User string `db:"user"`
		At   string `db:"at"`
	}
	err = s.App.DB().NewQuery(`
		SELECT held.value AS user, MAX(l.created) AS at
		FROM assignments_log l, json_each(CASE WHEN json_valid(l.assignees) THEN l.assignees ELSE '[]' END) held
		WHERE l.congregation = {:congregation} AND l.action = 'assigned'
		GROUP BY held.value
	`).Bind(dbx.Params{"congregation": s.CongregationID}).All(&given)
	if err != nil {
		return nil, err
	}
	lastGiven := make(map[string]string, len(given))
	for _, g := range given {
		lastGiven[g.User] = g.At
	}

	people := make([]LinkPerson, len(names))
	var picked []string
	for i, name := range names {
		people[i].Name = name
		typed := nameWords(name)
		if len(typed) == 0 {
			continue
		}
		remembered := ""
		for _, l := range learned {
			// A pairing only holds while the account is still a member.
			if l.Alias == strings.Join(typed, " ") && slices.ContainsFunc(members, func(m member) bool { return m.id == l.User }) {
				remembered = l.User
			}
		}
		best, hits := matchNone, []string(nil)
		for _, m := range members {
			switch kind := matchKind(typed, m); {
			case kind == matchNone:
			case best == matchNone || kind < best:
				best, hits = kind, []string{m.id}
			case kind == best:
				hits = append(hits, m.id)
			}
		}
		slices.SortStableFunc(hits, func(a, b string) int { return strings.Compare(lastGiven[b], lastGiven[a]) })

		fullName := best == matchSameWords && len(typed) > 1
		pick := ""
		switch {
		// A member whose full name this is outranks an older pairing with
		// someone else: the admin is asked which they mean.
		case remembered != "" && !(fullName && !slices.Contains(hits, remembered)):
			pick = remembered
		case remembered == "" && fullName && len(hits) == 1:
			pick = hits[0]
		}
		if pick != "" && !slices.Contains(picked, pick) {
			people[i].Picked, people[i].Remembered = pick, pick == remembered
			picked = append(picked, pick)
			continue
		}
		if remembered != "" {
			hits = slices.Insert(slices.DeleteFunc(hits, func(id string) bool { return id == remembered }), 0, remembered)
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
