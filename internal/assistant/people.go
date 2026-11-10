package assistant

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"ganigeorgiev.com/pocketbase/dbx"
)

// knownPeople lists every name the congregation's records hold for a person:
// users with a role, publishers on share links, and the names saved with
// visits, notes and messages. The model sees these only as person tokens.
func knownPeople(s Scope) ([]string, error) {
	var names []string
	err := s.App.DB().NewQuery(`
		SELECT DISTINCT TRIM(name) AS name FROM (
			SELECT u.name AS name FROM roles r JOIN users u ON u.id = r.user WHERE r.congregation = {:congregation}
			UNION SELECT publisher FROM assignments WHERE congregation = {:congregation}
			UNION SELECT publisher FROM assignments_log WHERE congregation = {:congregation}
			UNION SELECT changed_by FROM addresses_log WHERE congregation = {:congregation}
			UNION SELECT last_notes_updated_by FROM addresses WHERE congregation = {:congregation}
			UNION SELECT created_by FROM messages WHERE congregation = {:congregation}
		) WHERE TRIM(COALESCE(name, '')) != ''
	`).Bind(dbx.Params{"congregation": s.CongregationID}).Column(&names)
	return names, err
}

// nameMask finds known names in text the admin typed and replaces each with
// its person token, so a question like "what did Bro Tan do?" reaches the
// model as "what did Person QXWB do?" and matches the tokens in the results.
// It is built once per question and applied to the question and its history.
//
// A name of several words is matched in any case, since admins type "bro tan".
// A single-word name is matched only as written, so a publisher called "Will"
// or "May" does not swallow those ordinary words. Longer names go first, so
// "Bro Tan Ah Kow" is not cut short by "Bro Tan". A match inside a longer name
// the admin typed is left alone: "Test" in "Bro QA Test" is someone new, and
// masking it would hand the model half a name.
type nameMask []namePattern

type namePattern struct {
	name string
	re   *regexp.Regexp
}

func newNameMask(names []string) nameMask {
	sorted := append([]string(nil), names...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return utf8.RuneCountInString(sorted[i]) > utf8.RuneCountInString(sorted[j])
	})
	var mask nameMask
	for _, name := range sorted {
		if utf8.RuneCountInString(name) < 3 || tokenPattern.MatchString(name) {
			continue
		}
		// \b only understands ASCII, so a boundary is required only where the
		// name starts or ends with an ASCII letter or digit; a Chinese or Tamil
		// name is matched as written.
		pattern := strings.Join(strings.Fields(regexp.QuoteMeta(name)), `\s+`)
		if isASCIIWord(name[0]) {
			pattern = `\b` + pattern
		}
		if isASCIIWord(name[len(name)-1]) {
			pattern += `\b`
		}
		if len(strings.Fields(name)) > 1 {
			pattern = `(?i)` + pattern
		}
		mask = append(mask, namePattern{name: name, re: regexp.MustCompile(pattern)})
	}
	return mask
}

func (mask nameMask) apply(text string, people *pseudonyms) string {
	for _, n := range mask {
		var out strings.Builder
		last := 0
		for _, m := range n.re.FindAllStringIndex(text, -1) {
			if insideLongerName(text, m[0], m[1]) {
				continue
			}
			out.WriteString(text[last:m[0]])
			out.WriteString(people.token(n.name))
			last = m[1]
		}
		out.WriteString(text[last:])
		text = out.String()
	}
	return text
}

// honorifics may sit before a name without being part of it.
var honorifics = map[string]bool{
	"bro": true, "brother": true, "sis": true, "sister": true, "mr": true, "mrs": true,
	"ms": true, "mdm": true, "dr": true, "sdr": true, "sdri": true, "i": true,
}

// insideLongerName reports whether text[start:end] is directly joined to
// another capitalised word, as in "Bro QA Test" or "Tan Ah Kow". A capitalised
// word that only starts the sentence ("Did Lim come?") does not count.
func insideLongerName(text string, start, end int) bool {
	if after := strings.Fields(text[end:]); len(after) > 0 && end < len(text) && unicode.IsSpace(rune(text[end])) {
		word := strings.TrimRight(after[0], ".,;:!?'\")")
		if capitalised(word) && !honorifics[strings.ToLower(word)] {
			return true
		}
	}
	before := strings.Fields(text[:start])
	if len(before) == 0 || !unicode.IsSpace(rune(text[start-1])) {
		return false
	}
	word := before[len(before)-1]
	if !capitalised(word) || honorifics[strings.ToLower(word)] {
		return false
	}
	return len(before) > 1 && !strings.ContainsAny(before[len(before)-2], ".!?")
}

// capitalised reports whether word is letters only and starts with a capital.
func capitalised(word string) bool {
	first, _ := utf8.DecodeRuneInString(word)
	if !unicode.IsUpper(first) {
		return false
	}
	for _, r := range word {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return true
}

func isASCIIWord(b byte) bool {
	return b == '_' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}
