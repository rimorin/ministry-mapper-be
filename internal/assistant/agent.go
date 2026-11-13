package assistant

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"ministry-mapper/internal/llm"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

// Model answers admin questions. Its functions need the Responses API.
const Model = "gpt-6.1-terra"

// modelCallTimeout bounds one model call. A call takes a few seconds; one that
// hangs is tried once more rather than using up the whole question. A variable
// only so tests can shorten it.
var modelCallTimeout = 30 * time.Second

// ReasoningEffort is how much the model thinks before each reply. A variable
// only so the live eval can compare settings.
var ReasoningEffort = shared.ReasoningEffortLow

const (
	// maxRounds bounds model calls per question: tool rounds plus the final answer.
	maxRounds = 6
	// maxCallsPerRound bounds the functions one model reply may request.
	maxCallsPerRound = 4
	// MaxHistory is how many earlier turns are sent as context. Most of each
	// request is read from OpenAI's cache, so a longer memory costs little.
	MaxHistory = 8
)

// Tools is every query function the model may call.
var Tools = []Tool{
	findPlaces, territoryOverview, mapOverview, mapDetails, homesTool, mapsNear, activity, whoWorked, territoryRounds,
	assignments, feedback, dataQuality,
	changeHistory, peopleAndRoles, congregationSettings, calculate,
	proposeRoleChange, proposeRoleGrant,
	proposeRename, proposeShareLink, proposePinMessage, proposeReportEmail,
	proposeMarkRead, proposeUnpinMessage, proposeMapLocation, proposeLinkRevoke, proposeQuickLink, proposeHomeUpdate,
	proposeReset, proposeTriesReset, proposeHomesStatus, proposeDncReturn, proposeSettings,
	proposeMapMove, proposeFloorAdd, proposeUnitAdd, proposeTerritoryCreate, proposeMapsCreate,
}

// Responder is one model call, streamed: onText, when set, receives the reply's
// text as it is written. *llm.Client satisfies it; tests substitute a script.
type Responder interface {
	Respond(ctx context.Context, params responses.ResponseNewParams, onText func(string)) (*responses.Response, error)
}

// NewResponder returns the model client, or nil when OPENAI_API_KEY is unset.
// Tests replace it.
var NewResponder = func() Responder {
	if c := llm.New(Model); c != nil {
		return c
	}
	return nil
}

// Turn is an earlier question and its answer. Answer is the masked text the
// previous response returned as Context, so it carries person tokens, not names.
type Turn struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

type Request struct {
	Question string
	History  []Turn
	// People maps person tokens from earlier answers back to names, so tokens
	// repeated from the history can be restored too.
	People map[string]string
	// OnStep, when set, receives each function call as soon as it finishes, so
	// the admin sees progress while the model is still working.
	OnStep func(Step)
	// OnText, when set, receives the answer as it is written, names restored,
	// and OnReset withdraws what OnText sent; Response.Answer stays the answer.
	OnText  func(string)
	OnReset func()
}

// Step is one function call, shown to the admin as a line of "what I looked at"
// with its rows. Result keeps real names and record ids; only the model's copy is
// masked.
type Step struct {
	Tool   string  `json:"tool"`
	Result *Result `json:"result,omitempty"`
	Error  string  `json:"error,omitempty"`
	// Chart is how the answer asked for this result to be drawn, if at all.
	Chart *Chart `json:"chart,omitempty"`

	proposal *Proposal
}

// maxProposals bounds the cards one answer can carry: enough to give out maps to
// a group in one go.
const maxProposals = 10

type Response struct {
	Answer string `json:"answer"`
	// Answered is false when the functions could not answer the question; the
	// question is then logged as something to build next.
	Answered bool `json:"answered"`
	// NeedsDetail is set when the reply asks the admin for something it needs,
	// such as which map: a step on the way, not something to build. Logged only.
	NeedsDetail bool `json:"-"`
	// Grounded is false when the model's text quoted figures absent from the data
	// and was dropped; the steps still show the data.
	Grounded bool   `json:"grounded"`
	Steps    []Step `json:"steps"`
	// Context is the answer as the model wrote it, with person tokens. The client
	// sends it back as history so names never reach the model.
	Context string            `json:"context"`
	People  map[string]string `json:"people"`
	// Proposals are changes prepared for the admin to confirm on a card.
	Proposals []Proposal `json:"proposals"`
	// NeedsLocation is set when the admin asked about their own position
	// without sharing it, so the panel can offer to share it and ask again.
	NeedsLocation bool `json:"needsLocation"`
	// QuestionID is the logged question, set by the route once it is saved, so
	// the asker can rate the answer and report what happened to its cards.
	QuestionID string `json:"questionId,omitempty"`
	// ConversationID is the conversation the question was logged in, set by the
	// route; the panel sends it with the next question to stay in it.
	ConversationID string `json:"conversationId,omitempty"`
	// Suggestions are follow-up questions the admin can send with one tap,
	// including the choices when the model needs the admin to pick one.
	Suggestions []string `json:"suggestions"`

	PromptTokens     int64 `json:"-"`
	CompletionTokens int64 `json:"-"`
	// CachedTokens are the prompt tokens read from OpenAI's cache, billed lower.
	CachedTokens int64 `json:"-"`
}

var answerFormat = responses.ResponseFormatTextJSONSchemaConfigParam{
	Name:   "assistant_answer",
	Strict: openai.Bool(true),
	Schema: object(map[string]any{
		"answer":   map[string]any{"type": "string", "description": "The answer for the admin, or why it cannot be answered."},
		"answered": map[string]any{"type": "boolean", "description": "False only when the functions cannot answer the question."},
		"needs_detail": map[string]any{"type": "boolean",
			"description": "True when you ask the admin for something you need to go on, such as which map or how many days."},
		"suggestions": map[string]any{
			"type":        "array",
			"items":       map[string]any{"type": "string"},
			"description": "Up to 3 short follow-up questions the admin may ask next, written as the admin would ask them.",
		},
		"chart": chartParam,
	}),
}

// toolParams is the functions offered to the model: only those the asker's
// role may use (see policy.go).
func toolParams(role string) []responses.ToolUnionParam {
	tools := toolsFor(role)
	params := make([]responses.ToolUnionParam, len(tools))
	for i, t := range tools {
		params[i] = responses.ToolUnionParam{OfFunction: &responses.FunctionToolParam{
			Name:        t.Name,
			Description: openai.String(t.Description),
			Strict:      openai.Bool(true),
			Parameters:  t.Params,
		}}
	}
	return params
}

// respondOnce makes one model call, trying a second time if the first runs past
// modelCallTimeout. The SDK retries failed requests itself, but not one that
// simply never answers. Stopping the question still stops at once. The reply's
// answer streams to text, which may quote figures from source.
func respondOnce(ctx context.Context, chat Responder, params responses.ResponseNewParams, text *answerText, source string) (*responses.Response, error) {
	for attempt := 1; ; attempt++ {
		text.begin(source)
		callCtx, cancel := context.WithTimeout(ctx, modelCallTimeout)
		r, err := chat.Respond(callCtx, params, text.write)
		hung := errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil
		cancel()
		if !hung || attempt == 2 {
			return r, err
		}
	}
}

func findTool(name string) (Tool, bool) {
	for _, t := range Tools {
		if t.Name == name {
			return t, true
		}
	}
	return Tool{}, false
}

// Ask answers one question. Database failures are returned as errors; mistakes
// in the model's arguments go back to the model to correct.
func Ask(ctx context.Context, chat Responder, s Scope, req Request) (Response, error) {
	people := newPseudonyms(s.CongregationID)
	for token, name := range req.People {
		people.remember(token, name)
	}
	// Names the admin types are masked too, so a question about a person
	// matches that person's token in the results and the name stays here.
	names, err := knownPeople(s)
	if err != nil {
		return Response{}, fmt.Errorf("assistant known people: %w", err)
	}
	s.knownTokens = tokensFor(s.CongregationID, names)
	if len(req.History) > MaxHistory {
		req.History = req.History[len(req.History)-MaxHistory:]
	}
	mask := newNameMask(names)
	req.Question = mask.apply(req.Question, people)
	for i := range req.History {
		req.History[i].Question = mask.apply(req.History[i].Question, people)
	}

	var source strings.Builder // everything the model may quote figures from
	system := systemPrompt(s)
	source.WriteString(system)
	input := responses.ResponseInputParam{}
	for _, turn := range req.History {
		input = append(input,
			responses.ResponseInputItemParamOfMessage(clip(turn.Question), responses.EasyInputMessageRoleUser),
			responses.ResponseInputItemParamOfMessage(clip(turn.Answer), responses.EasyInputMessageRoleAssistant))
		source.WriteString(turn.Question + "\n" + turn.Answer + "\n")
	}
	input = append(input, responses.ResponseInputItemParamOfMessage(req.Question, responses.EasyInputMessageRoleUser))
	source.WriteString(req.Question + "\n")

	resp := Response{Steps: []Step{}, Suggestions: []string{}, Proposals: []Proposal{}}
	text := newAnswerText(req.OnText, req.OnReset, people)
	tools := toolParams(s.Role)
	call := func(withTools bool) (*responses.Response, error) {
		params := responses.ResponseNewParams{
			Instructions: openai.String(system),
			Input:        responses.ResponseNewParamsInputUnion{OfInputItemList: input},
			// The functions stay listed on every call, so the cached start of the
			// request stays the same; tool_choice none makes the model answer.
			Tools:     tools,
			Reasoning: shared.ReasoningParam{Effort: ReasoningEffort},
			Text: responses.ResponseTextConfigParam{
				Format: responses.ResponseFormatTextConfigUnionParam{OfJSONSchema: &answerFormat},
			},
			// Nothing is kept on OpenAI's side: the history arrives with each
			// question, and the reasoning comes back encrypted to pass on.
			Store:   openai.Bool(false),
			Include: []responses.ResponseIncludable{responses.ResponseIncludableReasoningEncryptedContent},
		}
		if !withTools {
			params.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{
				OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptionsNone),
			}
		}
		r, err := respondOnce(ctx, chat, params, text, source.String())
		if err != nil {
			return nil, fmt.Errorf("assistant model call: %w", err)
		}
		resp.PromptTokens += r.Usage.InputTokens
		resp.CompletionTokens += r.Usage.OutputTokens
		resp.CachedTokens += r.Usage.InputTokensDetails.CachedTokens
		return r, nil
	}
	// echo hands a reply's items back as input: the request is stateless, so
	// the model sees its own function calls and reasoning again next round.
	echo := func(r *responses.Response) {
		for _, item := range r.Output {
			input = append(input, param.Override[responses.ResponseInputItemUnionParam](json.RawMessage(item.RawJSON())))
		}
	}

	var final *responses.Response
	for round := 0; ; round++ {
		r, err := call(round < maxRounds-1)
		if err != nil {
			return resp, err
		}
		var calls []responses.ResponseFunctionToolCall
		for _, item := range r.Output {
			if item.Type == "function_call" {
				calls = append(calls, item.AsFunctionCall())
			}
		}
		if len(calls) == 0 {
			final = r
			break
		}
		echo(r)
		for i, tc := range calls {
			output := fmt.Sprintf(`{"error":"too many functions in one reply; call at most %d at a time"}`, maxCallsPerRound)
			if i < maxCallsPerRound {
				s.seen = source.String()
				step, out, err := runTool(s, people, tc.Name, tc.Arguments)
				if err != nil {
					return resp, err
				}
				if step.proposal != nil {
					if len(resp.Proposals) < maxProposals {
						resp.Proposals = append(resp.Proposals, *step.proposal)
					} else {
						// The card will not be shown, so the model must not say it is ready.
						step.Error = fmt.Sprintf("not prepared: at most %d cards per answer", maxProposals)
						refusal, _ := json.Marshal(map[string]string{"error": step.Error + "; tell the admin to ask again for the rest"})
						out = string(refusal)
					}
				}
				resp.Steps = append(resp.Steps, step)
				if step.Error == (needLocationError{}).Error() {
					resp.NeedsLocation = true
				}
				if req.OnStep != nil {
					req.OnStep(step)
				}
				output = out
			}
			result := responses.ResponseInputItemParamOfFunctionCallOutput(output)
			result.OfFunctionCallOutput.CallID = param.NewOpt(tc.CallID)
			input = append(input, result)
			source.WriteString(output + "\n")
		}
	}

	answer, err := parseAnswer(final.OutputText())
	if err != nil {
		return resp, err
	}
	resp.Answered = answer.Answered
	groundErr := llm.GroundedNumbers(source.String(), answer.Answer)
	if groundErr != nil {
		echo(final)
		input = append(input, responses.ResponseInputItemParamOfMessage(
			"Your answer "+groundErr.Error()+". Answer again using only figures from the function results.",
			responses.EasyInputMessageRoleDeveloper))
		retry, err := call(false)
		if err != nil {
			return resp, err
		}
		if answer, err = parseAnswer(retry.OutputText()); err != nil {
			return resp, err
		}
		resp.Answered = answer.Answered
		groundErr = llm.GroundedNumbers(source.String(), answer.Answer)
	}
	attachChart(resp.Steps, answer.Chart)
	if groundErr != nil {
		return resp, nil // Grounded stays false: the admin sees the data without the text.
	}
	resp.Grounded = true
	resp.NeedsDetail = answer.NeedsDetail
	resp.Context = answer.Answer
	resp.Answer = people.restore(answer.Answer)
	resp.People = people.used(answer.Answer)
	resp.Suggestions = followUps(answer.Suggestions)
	return resp, nil
}

type modelAnswer struct {
	Answer      string        `json:"answer"`
	Answered    bool          `json:"answered"`
	NeedsDetail bool          `json:"needs_detail"`
	Suggestions []string      `json:"suggestions"`
	Chart       *chartRequest `json:"chart"`
}

const (
	maxSuggestions      = 3
	maxSuggestionLength = 120
)

// followUps keeps the usable suggestions. One naming a person is dropped: a
// tapped suggestion becomes the next question, which would send the name to
// the model.
func followUps(suggestions []string) []string {
	out := []string{}
	for _, s := range suggestions {
		s = strings.TrimSpace(s)
		if s == "" || len([]rune(s)) > maxSuggestionLength || tokenPattern.MatchString(s) {
			continue
		}
		out = append(out, s)
		if len(out) == maxSuggestions {
			break
		}
	}
	return out
}

// parseAnswer reads the model's answer. The model has been seen to send its
// answer twice in one reply; the first complete answer is kept.
func parseAnswer(content string) (modelAnswer, error) {
	var a modelAnswer
	if err := json.NewDecoder(strings.NewReader(content)).Decode(&a); err != nil {
		return a, fmt.Errorf("parse assistant answer: %w", err)
	}
	return a, nil
}

// runTool runs one function and returns its step for the admin and its output
// for the model. Only a database failure is an error.
func runTool(s Scope, people *pseudonyms, name, rawArgs string) (Step, string, error) {
	step := Step{Tool: name}
	tool, ok := findTool(name)
	if !ok {
		step.Error = "unknown function"
		return step, `{"error":"unknown function"}`, nil
	}
	// The model is offered only what the role allows, but a call is checked
	// again: a function's name is no secret, and one argument can need more.
	var result Result
	err := allowed(s.Role, name, json.RawMessage(rawArgs))
	if err == nil {
		result, err = tool.Run(s, json.RawMessage(rawArgs))
	}
	// The model's own mistakes go back to it as the output, so it can recover.
	if errors.As(err, new(needLocationError)) || errors.As(err, new(argError)) {
		step.Error = err.Error()
		out, _ := json.Marshal(map[string]string{"error": step.Error})
		return step, string(out), nil
	}
	if err != nil {
		return step, "", fmt.Errorf("assistant function %s: %w", name, err)
	}
	if p := result.Proposal; p != nil {
		fillProposal(p, name)
		step.proposal = p
		return step, proposalView(*p), nil
	}
	localizeRows(&result, s.Locale)
	if result.Total == 0 {
		// Only listings truncate; a single-record result such as map_details
		// would otherwise read as "nothing found".
		result.Total = len(result.Rows)
	}
	step.Result = &result
	return step, modelView(result, tool.PersonColumns, people), nil
}

// fillProposal sets what every card needs, here rather than in each function:
// an id, the kind (the function's name without "propose_" unless it chose
// one), and empty lists, since the card reads both.
func fillProposal(p *Proposal, toolName string) {
	p.ID = proposalID()
	if p.Kind == "" {
		p.Kind = strings.TrimPrefix(toolName, "propose_")
	}
	if p.Warnings == nil {
		p.Warnings = []string{}
	}
	if p.Candidates == nil {
		p.Candidates = []Candidate{}
	}
}

// modelView is the model's copy of a result: the listed columns only (so no
// record ids), rows as arrays, and people as tokens.
func modelView(r Result, personColumns []string, people *pseudonyms) string {
	isPerson := map[string]bool{}
	for _, c := range personColumns {
		isPerson[c] = true
	}
	rows := make([][]any, len(r.Rows))
	for i, row := range r.Rows {
		values := make([]any, len(r.Columns))
		for j, c := range r.Columns {
			v := row[c]
			if name, ok := v.(string); ok && isPerson[c] && name != "" {
				v = people.token(name)
			}
			values[j] = v
		}
		rows[i] = values
	}
	out, _ := json.Marshal(map[string]any{
		"columns": r.Columns, "rows": rows, "total_rows": r.Total, "rows_shown": len(rows), "note": r.Note,
	})
	return string(out)
}

func clip(s string) string {
	const max = 1500
	if len(s) <= max {
		return s
	}
	// Cut on a character boundary, so Tamil or Chinese text stays valid.
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

// pseudonyms replace people's names with tokens such as "Person QXWBKDMR". A token
// depends only on the congregation and the name, so the same person keeps the
// same token across follow-up questions. Letters only, so a token never looks
// like a figure to the grounding check.
type pseudonyms struct {
	salt  string
	names map[string]string // token → name
}

// tokenWord starts every person token, such as "Person QXWBKDMR".
const tokenWord = "Person "

// tokenLetters is how many letters follow tokenWord. A congregation's records
// can hold well over a thousand names, and two names sharing a token would
// put one person's name on the other's work: four letters gave such a pair in
// about one large congregation in four, eight in about one in a hundred
// thousand.
const tokenLetters = 8

var tokenPattern = regexp.MustCompile(fmt.Sprintf(`\b%s[A-Z]{%d}\b`, tokenWord, tokenLetters))

func newPseudonyms(salt string) *pseudonyms {
	return &pseudonyms{salt: salt, names: map[string]string{}}
}

func (p *pseudonyms) token(name string) string {
	sum := sha256.Sum256([]byte(p.salt + "\x00" + strings.ToLower(strings.TrimSpace(name))))
	letters := make([]byte, tokenLetters)
	for i := range letters {
		letters[i] = 'A' + sum[i]%26
	}
	token := tokenWord + string(letters)
	p.remember(token, name)
	return token
}

func (p *pseudonyms) remember(token, name string) {
	if _, seen := p.names[token]; !seen && tokenPattern.MatchString(token) {
		p.names[token] = name
	}
}

func (p *pseudonyms) restore(text string) string {
	return replaceTokens(text, p.names, nil)
}

// replaceTokens puts back the names that person tokens stand for, leaving
// tokens it does not know. used, when given, collects the tokens it replaced.
func replaceTokens(text string, names, used map[string]string) string {
	return tokenPattern.ReplaceAllStringFunc(text, func(token string) string {
		name, ok := names[token]
		if !ok {
			return token
		}
		if used != nil {
			used[token] = name
		}
		return name
	})
}

// used returns the tokens that text mentions, for the client to send back.
func (p *pseudonyms) used(text string) map[string]string {
	out := map[string]string{}
	for _, token := range tokenPattern.FindAllString(text, -1) {
		if name, ok := p.names[token]; ok {
			out[token] = name
		}
	}
	return out
}
