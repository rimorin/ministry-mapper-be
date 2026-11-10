package assistant

import (
	"encoding/json"
	"regexp"
	"strconv"

	"ministry-mapper/internal/llm"
)

// answerText turns a model reply, a JSON answer written a few characters at a
// time, into answer text the admin reads as it arrives. Only settled text is
// sent: a figure once it is complete and found in the data, a person token once
// it is whole and turned back into the name. A figure the data does not hold
// withdraws the text and stops the reply's stream, since that answer is asked
// for again or dropped; the final response always carries the answer in full.
type answerText struct {
	send   func(string)
	reset  func()
	people *pseudonyms

	figures llm.Figures
	head    string // the reply's start, until the answer string opens
	opened  bool
	escape  string // an escape sequence still being read
	text    []byte // answer text read but not yet sent
	shown   bool   // text was sent since the last reset
	done    bool
}

// newAnswerText returns nil when nobody listens; its methods then do nothing.
func newAnswerText(send func(string), reset func(), people *pseudonyms) *answerText {
	if send == nil {
		return nil
	}
	return &answerText{send: send, reset: reset, people: people}
}

// begin starts a new model reply, which may quote figures from source. Text
// sent from an earlier reply is withdrawn: that reply called functions, hung,
// or quoted a figure the data does not hold.
func (a *answerText) begin(source string) {
	if a == nil {
		return
	}
	if a.shown {
		a.reset()
	}
	*a = answerText{send: a.send, reset: a.reset, people: a.people, figures: llm.FiguresIn(source)}
}

// answerOpening is how a reply in the answer format starts: the answer field
// comes first, so its text can be read before the rest arrives.
var answerOpening = regexp.MustCompile(`^\s*\{\s*"answer"\s*:\s*"`)

// write reads the next piece of the reply.
func (a *answerText) write(delta string) {
	if a == nil || a.done {
		return
	}
	if !a.opened {
		a.head += delta
		at := answerOpening.FindStringIndex(a.head)
		if at == nil {
			// Not the expected shape: leave the answer to the final response.
			a.done = len(a.head) > 64
			return
		}
		delta, a.head, a.opened = a.head[at[1]:], "", true
	}
	ended := false
	for i := 0; i < len(delta) && !ended; i++ {
		ended = a.read(delta[i])
	}
	a.flush(ended)
}

// read takes one byte of the JSON string, reporting whether the string ended.
func (a *answerText) read(c byte) bool {
	if a.escape == "" {
		switch c {
		case '"':
			return true
		case '\\':
			a.escape = `\`
		default:
			a.text = append(a.text, c)
		}
		return false
	}
	a.escape += string(c)
	switch n := len(a.escape); {
	case n == 2 && c != 'u', n == 6 && !highSurrogate(a.escape), n == 12:
		a.text = append(a.text, unescape(a.escape)...)
		a.escape = ""
	case n == 7 && c != '\\', n == 8 && c != 'u':
		// Half a surrogate pair without the other half: decode it alone and
		// read what followed it again.
		rest := a.escape[6:]
		a.text = append(a.text, unescape(a.escape[:6])...)
		a.escape = ""
		for i := 0; i < len(rest); i++ {
			if a.read(rest[i]) {
				return true
			}
		}
	}
	return false
}

func highSurrogate(escape string) bool {
	v, err := strconv.ParseUint(escape[2:6], 16, 32)
	return err == nil && v >= 0xD800 && v < 0xDC00
}

// unescape decodes one JSON escape sequence as encoding/json does.
func unescape(escape string) string {
	var s string
	if json.Unmarshal([]byte(`"`+escape+`"`), &s) != nil {
		return "�"
	}
	return s
}

// unsettled matches the end of the text that may still grow into a figure or
// a person token, so it waits for what follows.
var unsettled = regexp.MustCompile(`(?:[\d.,]+|\b` + startsOf(tokenWord) + `)$`)

// startsOf matches any start of a token: "P", "Pers", "Person QX" and so on.
func startsOf(word string) string {
	pattern := "(?:[A-Z]{0,4})?"
	for i := len(word) - 1; i > 0; i-- {
		pattern = "(?:" + regexp.QuoteMeta(word[i:i+1]) + pattern + ")?"
	}
	return regexp.QuoteMeta(word[:1]) + pattern
}

// flush sends the settled text, or all of it once the answer has ended.
func (a *answerText) flush(ended bool) {
	cut := len(a.text)
	if !ended {
		if at := unsettled.FindIndex(a.text); at != nil {
			cut = at[0]
		}
	}
	a.done = ended
	if cut == 0 {
		return
	}
	chunk := string(a.text[:cut])
	a.text = a.text[cut:]
	if a.figures.Check(chunk) != nil {
		if a.shown {
			a.reset()
		}
		a.shown, a.done = false, true
		return
	}
	a.send(a.people.restore(chunk))
	a.shown = true
}
