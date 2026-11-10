//go:build testdata

package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/openai/openai-go/v3/responses"
)

// scriptedChat replays model replies in order and records every request. Replies
// are JSON, unmarshalled the way real API responses are, and their text streams
// a few characters at a time, as the API sends it.
type scriptedChat struct {
	t       *testing.T
	replies []string
	calls   []responses.ResponseNewParams
}

func (c *scriptedChat) Respond(_ context.Context, params responses.ResponseNewParams, onText func(string)) (*responses.Response, error) {
	c.calls = append(c.calls, params)
	if len(c.replies) == 0 {
		c.t.Fatalf("unexpected model call %d", len(c.calls))
	}
	reply := c.replies[0]
	c.replies = c.replies[1:]
	var r responses.Response
	if err := json.Unmarshal([]byte(reply), &r); err != nil {
		c.t.Fatalf("bad scripted reply: %v", err)
	}
	if onText != nil {
		for text := []rune(r.OutputText()); len(text) > 0; {
			n := min(3, len(text))
			onText(string(text[:n]))
			text = text[n:]
		}
	}
	return &r, nil
}

// answeredWith is a Responses reply whose message is the given text.
func answeredWith(text string, input, output int) string {
	quoted, _ := json.Marshal(text)
	return fmt.Sprintf(`{"id":"resp_final","object":"response","status":"completed","output":[{"type":"message",
		"id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":%s,"annotations":[]}]}],
		"usage":{"input_tokens":%d,"output_tokens":%d,"input_tokens_details":{"cached_tokens":0}}}`, quoted, input, output)
}

// toolChoiceNone reports whether a call made the model answer instead of
// calling a function.
func toolChoiceNone(params responses.ResponseNewParams) bool {
	return params.ToolChoice.OfToolChoiceMode.Value == responses.ToolChoiceOptionsNone
}

// sent is everything the model was shown across all calls.
func (c *scriptedChat) sent() string {
	raw, _ := json.Marshal(c.calls)
	return string(raw)
}

func toolCalls(calls ...[2]string) string {
	parts := make([]string, len(calls))
	for i, call := range calls {
		args, _ := json.Marshal(call[1])
		parts[i] = fmt.Sprintf(`{"type":"function_call","id":"fc_%d","call_id":"call_%d","name":%q,"arguments":%s,
			"status":"completed"}`, i, i, call[0], args)
	}
	return `{"id":"resp_calls","object":"response","status":"completed","output":[` + strings.Join(parts, ",") +
		`],"usage":{"input_tokens":100,"output_tokens":10,"input_tokens_details":{"cached_tokens":40}}}`
}

func final(answer string, answered bool) string {
	content, _ := json.Marshal(map[string]any{"answer": answer, "answered": answered})
	return answeredWith(string(content), 200, 20)
}

func ask(t *testing.T, chat *scriptedChat, question string) Response {
	t.Helper()
	app := newTestApp(t)
	resp, err := Ask(context.Background(), chat, scopeFor(t, app, alphaCong), Request{Question: question})
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestAsk_CallsFunctionThenAnswers(t *testing.T) {
	chat := &scriptedChat{t: t, replies: []string{
		toolCalls([2]string{"congregation_settings", `{}`}),
		final("Homes count as covered after 3 nobody-home tries.", true),
	}}
	resp := ask(t, chat, "How many tries before a home counts?")

	if !resp.Grounded || !resp.Answered || resp.Answer != "Homes count as covered after 3 nobody-home tries." {
		t.Fatalf("resp = %+v", resp)
	}
	if len(resp.Steps) != 1 || resp.Steps[0].Tool != "congregation_settings" || resp.Steps[0].Result == nil {
		t.Fatalf("steps = %+v", resp.Steps)
	}
	if resp.PromptTokens != 300 || resp.CompletionTokens != 30 || resp.CachedTokens != 40 {
		t.Errorf("tokens = %d/%d (%d cached), want summed over both calls",
			resp.PromptTokens, resp.CompletionTokens, resp.CachedTokens)
	}
	if len(chat.calls[0].Tools) != len(Tools) {
		t.Errorf("the first call must offer every function")
	}
	for i, call := range chat.calls {
		if call.Store.Value || !call.Store.Valid() {
			t.Errorf("call %d: store must be false; nothing is kept on OpenAI's side", i)
		}
		if call.Reasoning.Effort != ReasoningEffort {
			t.Errorf("call %d: reasoning effort = %q, want %q", i, call.Reasoning.Effort, ReasoningEffort)
		}
	}
	// Stateless: the second call carries the function call and its result.
	second, _ := json.Marshal(chat.calls[1].Input)
	if !strings.Contains(string(second), `"type":"function_call"`) ||
		!strings.Contains(string(second), `"call_id":"call_0","type":"function_call_output"`) {
		t.Errorf("second call input = %s", second)
	}
}

// hangingChat leaves its first calls unanswered until they time out, then
// answers like scriptedChat.
type hangingChat struct {
	scriptedChat
	hangs int
}

func (c *hangingChat) Respond(ctx context.Context, params responses.ResponseNewParams, onText func(string)) (*responses.Response, error) {
	if c.hangs > 0 {
		c.hangs--
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return c.scriptedChat.Respond(ctx, params, onText)
}

func TestAsk_RetriesACallThatHangs(t *testing.T) {
	previous := modelCallTimeout
	modelCallTimeout = 50 * time.Millisecond
	t.Cleanup(func() { modelCallTimeout = previous })
	app := newTestApp(t)
	s := scopeFor(t, app, alphaCong)

	once := &hangingChat{scriptedChat: scriptedChat{t: t, replies: []string{final("Nothing to add.", true)}}, hangs: 1}
	resp, err := Ask(context.Background(), once, s, Request{Question: "Anything?"})
	if err != nil || resp.Answer != "Nothing to add." {
		t.Fatalf("a hung call must be tried again: resp %+v, err %v", resp, err)
	}

	twice := &hangingChat{scriptedChat: scriptedChat{t: t}, hangs: 2}
	if _, err := Ask(context.Background(), twice, s, Request{Question: "Anything?"}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("a call that hangs twice must give up, err = %v", err)
	}
}

func TestAsk_TellsTheModelWhenACardIsOverTheLimit(t *testing.T) {
	link := func(i int) [2]string {
		return [2]string{"propose_share_link", fmt.Sprintf(`{"map": "Blk 100B", "publisher": "Publisher %d", "type": "normal"}`, i)}
	}
	var replies []string
	for first := 0; first <= maxProposals; first += maxCallsPerRound {
		var batch [][2]string
		for i := first; i < min(first+maxCallsPerRound, maxProposals+1); i++ {
			batch = append(batch, link(i))
		}
		replies = append(replies, toolCalls(batch...))
	}
	replies = append(replies, final("The links are ready to confirm.", true))
	chat := &scriptedChat{t: t, replies: replies}
	app := newTestApp(t)
	s := scopeFor(t, app, alphaCong)
	s.UserID = "testuseralpha01"

	resp, err := Ask(context.Background(), chat, s, Request{Question: "Give Blk 100B to these publishers"})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Proposals) != maxProposals {
		t.Fatalf("cards = %d, want %d", len(resp.Proposals), maxProposals)
	}
	last := resp.Steps[len(resp.Steps)-1]
	if !strings.Contains(last.Error, "at most") || !strings.Contains(chat.sent(), "ask again for the rest") {
		t.Fatalf("the card over the limit must be refused to the model and shown as not prepared: %+v", last)
	}
}

func TestAsk_DropsTextWithInventedFigures(t *testing.T) {
	chat := &scriptedChat{t: t, replies: []string{
		toolCalls([2]string{"congregation_settings", `{}`}),
		final("Homes count after 7 tries.", true),
		final("Homes count after 9 tries.", true),
	}}
	resp := ask(t, chat, "How many tries before a home counts?")
	if resp.Grounded || resp.Answer != "" || len(resp.Steps) != 1 {
		t.Fatalf("an answer that invents figures twice must be dropped, keeping the data: %+v", resp)
	}
	retry := chat.calls[2]
	if !toolChoiceNone(retry) || !strings.Contains(chat.sent(), "only figures from the function results") {
		t.Errorf("the retry must ask for grounded figures without letting the model call a function")
	}
}

func TestAsk_RetryCanRecoverGrounding(t *testing.T) {
	chat := &scriptedChat{t: t, replies: []string{
		toolCalls([2]string{"congregation_settings", `{}`}),
		final("Homes count after 7 tries.", true),
		final("Homes count after 3 tries.", true),
	}}
	if resp := ask(t, chat, "How many tries?"); !resp.Grounded || resp.Answer != "Homes count after 3 tries." {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestAsk_NamesNeverReachTheModel(t *testing.T) {
	token := newPseudonyms(alphaCong).token("Alpha Admin")
	chat := &scriptedChat{t: t, replies: []string{
		toolCalls([2]string{"people_and_roles", `{"role":"administrator","name":null}`}),
		final(token+" is the only administrator.", true),
	}}
	resp := ask(t, chat, "Who are the admins?")

	if strings.Contains(chat.sent(), "Alpha Admin") {
		t.Fatal("a person's name was sent to the model")
	}
	if resp.Answer != "Alpha Admin is the only administrator." || resp.Context != token+" is the only administrator." {
		t.Fatalf("answer = %q, context = %q", resp.Answer, resp.Context)
	}
	if resp.People[token] != "Alpha Admin" {
		t.Errorf("people = %v", resp.People)
	}
	if resp.Steps[0].Result.Rows[0]["name"] != "Alpha Admin" {
		t.Errorf("the admin's table must keep the real name")
	}
}

func TestAsk_ArgumentErrorGoesBackToTheModel(t *testing.T) {
	chat := &scriptedChat{t: t, replies: []string{
		toolCalls([2]string{"territory_overview", `{"territory":"ZZ9"}`}),
		final("I could not find territory ZZ9.", false),
	}}
	resp := ask(t, chat, "How is ZZ9 doing?")
	if resp.Answered || len(resp.Steps) != 1 || resp.Steps[0].Error == "" {
		t.Fatalf("resp = %+v", resp)
	}
	if !strings.Contains(chat.sent(), "find_places") {
		t.Error("the model must be told how to recover")
	}
}

func TestAsk_RoundsAndCallsAreBounded(t *testing.T) {
	five := make([][2]string, 5)
	for i := range five {
		five[i] = [2]string{"congregation_settings", `{}`}
	}
	replies := []string{toolCalls(five...)}
	for range maxRounds - 2 {
		replies = append(replies, toolCalls([2]string{"congregation_settings", `{}`}))
	}
	replies = append(replies, final("Settings checked.", true))
	chat := &scriptedChat{t: t, replies: replies}
	resp := ask(t, chat, "Check the settings.")

	if len(chat.calls) != maxRounds {
		t.Fatalf("model calls = %d, want %d", len(chat.calls), maxRounds)
	}
	if !toolChoiceNone(chat.calls[maxRounds-1]) || toolChoiceNone(chat.calls[0]) {
		t.Error("only the last call may stop the model calling functions, so it has to answer")
	}
	if want := maxCallsPerRound + maxRounds - 2; len(resp.Steps) != want {
		t.Errorf("steps = %d, want %d (the fifth call in one reply is refused)", len(resp.Steps), want)
	}
}

func TestAsk_HistoryCarriesTokensNotNames(t *testing.T) {
	token := newPseudonyms(alphaCong).token("Bro Tan")
	chat := &scriptedChat{t: t, replies: []string{final("Yes, "+token+" did.", true)}}
	app := newTestApp(t)
	resp, err := Ask(context.Background(), chat, scopeFor(t, app, alphaCong), Request{
		Question: "Did they come out on Saturday too?",
		History:  []Turn{{Question: "Who came out today?", Answer: token + " came out today."}},
		People:   map[string]string{token: "Bro Tan"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(chat.sent(), "Bro Tan") {
		t.Fatal("a name from the history was sent to the model")
	}
	if resp.Answer != "Yes, Bro Tan did." {
		t.Errorf("answer = %q", resp.Answer)
	}
}

func TestAsk_SendsOnlyTheLatestTurns(t *testing.T) {
	history := make([]Turn, MaxHistory+2)
	for i := range history {
		history[i] = Turn{Question: fmt.Sprintf("Question %c?", 'A'+i), Answer: "Noted."}
	}
	chat := &scriptedChat{t: t, replies: []string{final("Done.", true)}}
	app := newTestApp(t)
	if _, err := Ask(context.Background(), chat, scopeFor(t, app, alphaCong), Request{Question: "And now?", History: history}); err != nil {
		t.Fatal(err)
	}
	sent := chat.sent()
	if strings.Contains(sent, "Question A?") || strings.Contains(sent, "Question B?") ||
		!strings.Contains(sent, "Question C?") || !strings.Contains(sent, fmt.Sprintf("Question %c?", 'A'+MaxHistory+1)) {
		t.Fatalf("want only the last %d turns: %s", MaxHistory, sent)
	}
}

func finalWithSuggestions(answer string, suggestions ...string) string {
	content, _ := json.Marshal(map[string]any{"answer": answer, "answered": true, "suggestions": suggestions})
	return answeredWith(string(content), 10, 5)
}

func TestAsk_ReturnsUsableFollowUps(t *testing.T) {
	token := newPseudonyms(alphaCong).token("Bro Tan")
	chat := &scriptedChat{t: t, replies: []string{finalWithSuggestions(
		"Here is an overview.",
		"  Which territories are not started?  ",
		"What did "+token+" do today?",
		strings.Repeat("a", maxSuggestionLength+1),
		"Which maps are free to assign?",
		"Who came out today?",
		"One too many?",
	)}}
	resp := ask(t, chat, "Tell me about my territories")

	want := []string{"Which territories are not started?", "Which maps are free to assign?", "Who came out today?"}
	if strings.Join(resp.Suggestions, "|") != strings.Join(want, "|") {
		t.Fatalf("suggestions = %q, want %q", resp.Suggestions, want)
	}
}

func TestAsk_MarksAQuestionBack(t *testing.T) {
	chat := &scriptedChat{t: t, replies: []string{answeredWith(
		`{"answer":"Which map should I give out?","answered":true,"needs_detail":true,"suggestions":[]}`, 10, 5)}}
	if resp := ask(t, chat, "Let's assign a map"); !resp.NeedsDetail || !resp.Answered {
		t.Fatalf("asking for a detail must be marked, and not as a failure: %+v", resp)
	}
}

func TestAsk_SuggestionsAreNeverNull(t *testing.T) {
	chat := &scriptedChat{t: t, replies: []string{final("Nothing to add.", true)}}
	if resp := ask(t, chat, "Anything?"); resp.Suggestions == nil {
		t.Fatal("suggestions must be an empty list, not null, so the client can map over it")
	}
}

// A question naming a person must reach the model as that person's token, so
// it matches the tokens in the results; the name itself never leaves.
func TestAsk_MasksNamesInTheQuestion(t *testing.T) {
	app := newTestApp(t)
	logAssigned(t, app, "testasgmask0001", "testmapalpha01a", "Bro Tan", "normal", testNow.Add(-time.Hour))
	token := newPseudonyms(alphaCong).token("Bro Tan")
	// No figures in the reply: no function runs here, so any number would be
	// rejected by the grounding check.
	chat := &scriptedChat{t: t, replies: []string{final(token+" got a link today.", true)}}

	resp, err := Ask(context.Background(), chat, scopeFor(t, app, alphaCong), Request{
		Question: "Which map did bro tan get a link for today?",
		History:  []Turn{{Question: "Did Bro Tan come out?", Answer: "Yes."}},
	})
	if err != nil {
		t.Fatal(err)
	}
	sent := strings.ToLower(chat.sent())
	if strings.Contains(sent, "bro tan") {
		t.Fatal("a name the admin typed was sent to the model")
	}
	if !strings.Contains(chat.sent(), "Which map did "+token+" get a link for today?") {
		t.Errorf("the question must carry the person's token: %s", chat.sent())
	}
	if resp.Answer != "Bro Tan got a link today." {
		t.Errorf("answer = %q", resp.Answer)
	}
}

func TestParseAnswer_KeepsTheFirstOfTwo(t *testing.T) {
	a, err := parseAnswer(`{"answer":"T01 needs the administrator role.","answered":false}{"answer":"again","answered":false}`)
	if err != nil || a.Answer != "T01 needs the administrator role." {
		t.Fatalf("answer = %+v, err = %v", a, err)
	}
	if _, err := parseAnswer(`not json`); err == nil {
		t.Fatal("an answer that is not JSON must still fail")
	}
}
