//go:build testdata

package assistant

import (
	"context"
	"strings"
	"testing"
)

// shown follows the events a reader receives: the text on screen, every piece
// sent, and how often the text was withdrawn.
type shown struct {
	text   string
	pieces []string
	resets int
}

func (s *shown) send(text string) {
	s.text += text
	s.pieces = append(s.pieces, text)
}

func (s *shown) reset() {
	s.text = ""
	s.resets++
}

func (s *shown) stream(people *pseudonyms) *answerText {
	return newAnswerText(s.send, s.reset, people)
}

// feed streams raw to a in the given pieces.
func feed(a *answerText, pieces ...string) {
	for _, p := range pieces {
		a.write(p)
	}
}

func TestAnswerText_DecodesEscapesSplitAcrossPieces(t *testing.T) {
	var s shown
	a := s.stream(newPseudonyms("c"))
	a.begin("")
	feed(a, `{"ans`, `wer": "Say \`, `"hi\"\n caf\u00`, `e9 \ud83d`, `\ude00 \ud83d x\`, `\ end", "answered": true}`)

	if want := "Say \"hi\"\n café 😀 � x\\ end"; s.text != want {
		t.Fatalf("text = %q, want %q", s.text, want)
	}
}

func TestAnswerText_SendsWholeNamesNeverTokens(t *testing.T) {
	people := newPseudonyms("c")
	token := people.token("Alpha Admin")
	var s shown
	a := s.stream(people)
	a.begin("")
	raw := `{"answer":"` + token + ` worked it.","answered":true}`
	for i := 0; i < len(raw); i += 2 {
		a.write(raw[i:min(i+2, len(raw))])
	}

	if s.text != "Alpha Admin worked it." {
		t.Fatalf("text = %q", s.text)
	}
	for _, p := range s.pieces {
		if strings.Contains(p, "Person") {
			t.Errorf("a piece carried a token: %q", p)
		}
	}
}

func TestAnswerText_HoldsAFigureUntilItIsWhole(t *testing.T) {
	var s shown
	a := s.stream(newPseudonyms("c"))
	a.begin(`{"homes": 1305}`)
	feed(a, `{"answer":"There are 1`, `,3`)
	if s.text != "There are " {
		t.Fatalf("a partial figure was sent: %q", s.text)
	}
	feed(a, `05 homes."}`)
	if s.text != "There are 1,305 homes." || s.resets != 0 {
		t.Fatalf("text = %q, resets = %d", s.text, s.resets)
	}
}

func TestAnswerText_WithdrawsAnAnswerQuotingAnInventedFigure(t *testing.T) {
	var s shown
	a := s.stream(newPseudonyms("c"))
	a.begin(`{"tries": 3}`)
	feed(a, `{"answer":"Homes count `, `after 7 tries. Then `, `more."}`)

	if s.text != "" || s.resets != 1 {
		t.Fatalf("text = %q, resets = %d", s.text, s.resets)
	}
	for _, p := range s.pieces {
		if strings.Contains(p, "7") {
			t.Errorf("the invented figure was sent: %q", p)
		}
	}
}

func TestAnswerText_NewReplyWithdrawsTheLast(t *testing.T) {
	var s shown
	a := s.stream(newPseudonyms("c"))
	a.begin("")
	feed(a, `{"answer":"Let me check`)
	a.begin("")
	if s.text != "" || s.resets != 1 {
		t.Fatalf("text = %q, resets = %d", s.text, s.resets)
	}
	a.begin("")
	if s.resets != 1 {
		t.Fatal("nothing was shown, so nothing is withdrawn")
	}
}

func TestAnswerText_IgnoresOtherShapes(t *testing.T) {
	var s shown
	a := s.stream(newPseudonyms("c"))
	a.begin("")
	feed(a, `{"answered":true,"answer":"Late"}`, strings.Repeat(" ", 80))
	if len(s.pieces) != 0 {
		t.Fatalf("pieces = %q", s.pieces)
	}
	var none *answerText
	none.begin("")
	none.write(`{"answer":"x"}`) // nobody listens: nothing to do, nothing to panic over
}

func TestAsk_StreamsTheAnswerAsTheResponseHasIt(t *testing.T) {
	token := newPseudonyms(alphaCong).token("Alpha Admin")
	chat := &scriptedChat{t: t, replies: []string{
		toolCalls([2]string{"people_and_roles", `{"role":"administrator","name":null}`}),
		final(token+" is the only administrator.", true),
	}}
	var s shown
	req := Request{Question: "Who are the admins?", OnText: s.send, OnReset: s.reset}
	resp, err := Ask(context.Background(), chat, scopeFor(t, newTestApp(t), alphaCong), req)
	if err != nil {
		t.Fatal(err)
	}
	if s.text != resp.Answer || s.text != "Alpha Admin is the only administrator." || len(s.pieces) < 2 {
		t.Fatalf("streamed %q in %d pieces, answer %q", s.text, len(s.pieces), resp.Answer)
	}
}

func TestAsk_StreamsTheRetryAfterWithdrawingAnInventedFigure(t *testing.T) {
	chat := &scriptedChat{t: t, replies: []string{
		toolCalls([2]string{"congregation_settings", `{}`}),
		final("Homes count after 7 tries.", true),
		final("Homes count after 3 tries.", true),
	}}
	var s shown
	req := Request{Question: "How many tries?", OnText: s.send, OnReset: s.reset}
	resp, err := Ask(context.Background(), chat, scopeFor(t, newTestApp(t), alphaCong), req)
	if err != nil {
		t.Fatal(err)
	}
	if s.text != resp.Answer || s.resets != 1 || strings.Contains(strings.Join(s.pieces, ""), "7") {
		t.Fatalf("streamed %q (pieces %q, %d resets), answer %q", s.text, s.pieces, s.resets, resp.Answer)
	}
}
