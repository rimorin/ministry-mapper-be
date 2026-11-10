package assistant

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// placeAbbreviations write the short and long forms of address words one way,
// so "5 Marsiling Dr" and "5, Marsiling Drive" read the same.
var placeAbbreviations = map[string]string{
	"blk": "block", "dr": "drive", "rd": "road", "ave": "avenue", "av": "avenue", "st": "street",
	"cres": "crescent", "jln": "jalan", "lor": "lorong", "upp": "upper", "nth": "north", "sth": "south",
	"ctrl": "central", "pl": "place", "cl": "close", "hts": "heights", "pk": "park", "gdns": "gardens",
}

// placeWords splits a place name into comparable words: lower case, without
// punctuation, with abbreviations spelled out.
func placeWords(name string) []string {
	words := strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	for i, w := range words {
		if long, ok := placeAbbreviations[w]; ok {
			words[i] = long
		}
	}
	return words
}

// placeMatch reports whether every word of the query matches a word of the
// place, and how many typos that took. A word matches when it is equal to a
// place word or starts one ("100" finds "100a"); failing that, a word of four
// letters or more may be one edit away, of eight or more two. Numbers never
// match loosely: 5 is not 6. extra are words the query may also use, such as a
// map's territory code, as long as one query word matches the place's own words.
func placeMatch(query, own, extra []string) (typos int, ok bool) {
	matchedOwn := false
	for _, q := range query {
		best, inOwn := -1, false
		for i, w := range append(own[:len(own):len(own)], extra...) {
			d := wordDistance(q, w)
			if d >= 0 && (best < 0 || d < best || (d == best && i < len(own))) {
				best, inOwn = d, i < len(own)
			}
		}
		if best < 0 {
			return 0, false
		}
		typos += best
		matchedOwn = matchedOwn || inOwn
	}
	return typos, matchedOwn
}

// wordDistance is 0 for a match, the number of edits for a close word, or -1.
func wordDistance(q, w string) int {
	if strings.HasPrefix(w, q) {
		return 0
	}
	if strings.ContainsFunc(q, unicode.IsDigit) || strings.ContainsFunc(w, unicode.IsDigit) {
		return -1
	}
	allowed := 0
	switch n := utf8.RuneCountInString(q); {
	case n >= 8:
		allowed = 2
	case n >= 4:
		allowed = 1
	}
	if d := editDistance(q, w); d <= allowed {
		return d
	}
	return -1
}

// editDistance counts the insertions, deletions, substitutions and swaps of
// neighbouring letters that turn a into b.
func editDistance(a, b string) int {
	x, y := []rune(a), []rune(b)
	d := make([][]int, len(x)+1)
	for i := range d {
		d[i] = make([]int, len(y)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}
	for i := 1; i <= len(x); i++ {
		for j := 1; j <= len(y); j++ {
			cost := 1
			if x[i-1] == y[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
			if i > 1 && j > 1 && x[i-1] == y[j-2] && x[i-2] == y[j-1] {
				d[i][j] = min(d[i][j], d[i-2][j-2]+1)
			}
		}
	}
	return d[len(x)][len(y)]
}
