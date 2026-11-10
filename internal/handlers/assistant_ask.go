package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"ministry-mapper/internal/assistant"

	"ganigeorgiev.com/pocketbase/dbx"
	"ganigeorgiev.com/pocketbase/pocketbase/apis"
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"ganigeorgiev.com/pocketbase/pocketbase/tools/types"
)

const (
	maxAssistantQuestionLength = 500
	// assistantDailyCap bounds model spend per congregation per rolling day.
	assistantDailyCap = 300
	assistantTimeout  = 90 * time.Second
	// maxLoggedAnswerRunes fits the answer field of assistant_questions.
	maxLoggedAnswerRunes = 5000
)

type assistantAskRequest struct {
	Congregation string            `json:"congregation"`
	Question     string            `json:"question"`
	History      []assistant.Turn  `json:"history"`
	Locale       string            `json:"locale"`
	People       map[string]string `json:"people"`
	// Conversation is the id the panel gave this conversation when it started,
	// so even a question that fails or is stopped lands in it. One that is
	// someone else's, or not an id, starts a new conversation instead.
	Conversation string `json:"conversation"`
	// Location is the admin's position, sent only after they chose to share it.
	Location *struct {
		Lat float64 `json:"lat"`
		Lng float64 `json:"lng"`
	} `json:"location"`
	// View is the territory and map the admin has open, by id, when they let
	// the assistant use it. Checked against the congregation before use.
	View *struct {
		Territory string `json:"territory"`
		Map       string `json:"map"`
	} `json:"view"`
}

// assistantEvent is one line of the streamed answer. Type comes first so each
// line reads clearly in a network inspector.
type assistantEvent struct {
	Type     string              `json:"type"`
	Step     *assistant.Step     `json:"step,omitempty"`
	Response *assistant.Response `json:"response,omitempty"`
	Message  string              `json:"message,omitempty"`
	Text     string              `json:"text,omitempty"`
}

// HandleAssistantAsk answers a question about the asker's congregation. Every
// role may ask; the asker's role, read here and never from the request,
// decides which functions the assistant may use (assistant/policy.go). Changes
// are only prepared: the asker confirms them through the screens' own calls.
func HandleAssistantAsk(c *core.RequestEvent, app core.App, enabled func(congregationID string) bool) error {
	var req assistantAskRequest
	if err := c.BindBody(&req); err != nil {
		return apis.NewBadRequestError("Invalid request body", nil)
	}
	req.Question = strings.TrimSpace(req.Question)
	if req.Congregation == "" {
		return apis.NewBadRequestError("congregation is required", nil)
	}
	if req.Question == "" {
		return apis.NewBadRequestError("question is required", nil)
	}
	if utf8.RuneCountInString(req.Question) > maxAssistantQuestionLength {
		return apis.NewBadRequestError("question is too long", nil)
	}

	var origin *assistant.Location
	if req.Location != nil {
		if req.Location.Lat < -90 || req.Location.Lat > 90 || req.Location.Lng < -180 || req.Location.Lng > 180 {
			return apis.NewBadRequestError("location is out of range", nil)
		}
		origin = &assistant.Location{Lat: req.Location.Lat, Lng: req.Location.Lng}
	}

	role, err := assistantRole(app, c.Auth.Id, req.Congregation)
	if err != nil {
		return newServerError(err)
	}
	if role == "" {
		return apis.NewForbiddenError("Congregation access required", nil)
	}
	if !enabled(req.Congregation) {
		return apis.NewForbiddenError("The assistant is not enabled for this congregation", nil)
	}
	chat := assistant.NewResponder()
	if chat == nil {
		return apis.NewApiError(http.StatusServiceUnavailable, "The assistant is not configured", nil)
	}

	var asked int
	since, _ := types.ParseDateTime(time.Now().Add(-24 * time.Hour))
	err = app.DB().NewQuery(`SELECT COUNT(*) FROM assistant_questions WHERE congregation = {:congregation} AND created >= {:since}`).
		Bind(dbx.Params{"congregation": req.Congregation, "since": since.String()}).Row(&asked)
	if err != nil {
		return newServerError(err)
	}
	if asked >= assistantDailyCap {
		return apis.NewTooManyRequestsError("Your congregation has reached today's question limit", nil)
	}

	congregation, err := app.FindRecordById("congregations", req.Congregation)
	if err != nil {
		return newServerError(err)
	}
	location, err := time.LoadLocation(congregation.GetString("timezone"))
	if err != nil {
		location = time.UTC
	}

	// The answer streams as newline-delimited JSON: a "step" event as each query
	// function finishes, "delta" events as the answer is written ("reset"
	// withdraws them), then one "done" event with the full response, which
	// replaces the streamed text. Every refusal above is a plain JSON error, sent
	// before the stream starts.
	c.Response.Header().Set("Content-Type", "application/x-ndjson")
	c.Response.Header().Set("Cache-Control", "no-cache")
	c.Response.Header().Set("X-Accel-Buffering", "no") // keep proxies from holding events back
	c.Response.WriteHeader(http.StatusOK)
	send := func(event assistantEvent) {
		line, err := json.Marshal(event)
		if err != nil {
			log.Printf("assistant: encode event: %v", err)
			return
		}
		_, _ = c.Response.Write(append(line, '\n'))
		_ = c.Flush()
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), assistantTimeout)
	defer cancel()
	scope := assistant.Scope{App: app, CongregationID: req.Congregation, Location: location, Now: time.Now(),
		Locale: req.Locale, Origin: origin, UserID: c.Auth.Id, Role: role}
	if req.View != nil {
		scope.View = assistant.ResolveView(scope, req.View.Territory, req.View.Map)
	}
	resp, err := assistant.Ask(ctx, chat, scope,
		assistant.Request{
			Question: req.Question, History: req.History, People: req.People,
			OnStep:  func(step assistant.Step) { send(assistantEvent{Type: "step", Step: &step}) },
			OnText:  func(text string) { send(assistantEvent{Type: "delta", Text: text}) },
			OnReset: func() { send(assistantEvent{Type: "reset"}) },
		})
	entry := loggedQuestion{
		congregation: req.Congregation, user: c.Auth.Id, conversation: req.Conversation,
		question: req.Question, view: loggedViewOf(req, scope.View),
	}
	if errors.Is(err, context.Canceled) {
		// The admin pressed Stop or closed the page: nobody is left to answer,
		// but the question stays in the conversation. A timeout is
		// context.DeadlineExceeded and still reported.
		entry.status = "stopped"
		logAssistantQuestion(app, entry, resp)
		return nil
	}
	if err != nil {
		entry.status = "failed"
		logAssistantQuestion(app, entry, resp)
		send(assistantEvent{Type: "error", Message: "The assistant could not answer. Please try again."})
		// Returned for Sentry; PocketBase writes nothing once the stream has started.
		return newServerError(err)
	}

	resp.ConversationID, resp.QuestionID = logAssistantQuestion(app, entry, resp)
	send(assistantEvent{Type: "done", Response: &resp})
	return nil
}

// loggedView is the place the admin had open, as history shows it again and as
// a retry from history sends it back: the ids and what they are called.
type loggedView struct {
	TerritoryID   string `json:"territoryId"`
	TerritoryCode string `json:"territoryCode"`
	TerritoryName string `json:"territoryName,omitempty"`
	MapID         string `json:"mapId,omitempty"`
	MapName       string `json:"mapName,omitempty"`
}

// loggedViewOf is the place the question was asked about, or nil when none was
// open or it did not check out.
func loggedViewOf(req assistantAskRequest, view *assistant.View) *loggedView {
	if view == nil {
		return nil
	}
	logged := &loggedView{TerritoryID: req.View.Territory, TerritoryCode: view.TerritoryCode, TerritoryName: view.TerritoryName}
	if view.MapName != "" {
		logged.MapID, logged.MapName = req.View.Map, view.MapName
	}
	return logged
}

// loggedQuestion is what is known of a question before its answer.
type loggedQuestion struct {
	congregation, user, conversation, question string
	// status is "" when answered, or failed or stopped.
	status string
	view   *loggedView
}

// assistantRole is the user's role in the congregation, or "" without one.
func assistantRole(app core.App, userID, congregationID string) (string, error) {
	var role string
	err := app.DB().NewQuery(`SELECT role FROM roles WHERE user = {:user} AND congregation = {:congregation}
		AND role IN ('administrator', 'conductor', 'read_only') LIMIT 1`).
		Bind(dbx.Params{"user": userID, "congregation": congregationID}).Row(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return role, err
}

// logAssistantQuestion records the question with everything its answer showed,
// so history can bring it back whole: the answer as the model wrote it (people
// as tokens), the lookups and the rows they found, the change cards, the place
// open and whether it failed or was stopped. It goes in the asker's
// conversation: the one with the panel's id, created if new, or a new one when
// that id is someone else's. It returns the conversation's and the question's
// ids. A failure is logged, not returned: the admin already has their answer.
func logAssistantQuestion(app core.App, asked loggedQuestion, resp assistant.Response) (string, string) {
	collection, err := app.FindCachedCollectionByNameOrId("assistant_questions")
	if err != nil {
		log.Printf("assistant: find assistant_questions: %v", err)
		return "", ""
	}
	conversation, taken, err := askerConversation(app, asked.conversation, asked.user, asked.congregation)
	if err != nil {
		log.Printf("assistant: find conversation: %v", err)
		return "", ""
	}
	if conversation == nil {
		conversations, err := app.FindCachedCollectionByNameOrId("assistant_conversations")
		if err != nil {
			log.Printf("assistant: find assistant_conversations: %v", err)
			return "", ""
		}
		conversation = core.NewRecord(conversations)
		// The panel's id when it is new, so the panel and history agree.
		if !taken && conversationIDPattern.MatchString(asked.conversation) {
			conversation.Id = asked.conversation
		}
		conversation.Set("congregation", asked.congregation)
		conversation.Set("user", asked.user)
	}
	conversation.Set("last_question", types.NowDateTime())
	if err := app.Save(conversation); err != nil {
		log.Printf("assistant: save conversation: %v", err)
		return "", ""
	}
	tools := make([]string, 0, len(resp.Steps))
	for _, s := range resp.Steps {
		tools = append(tools, s.Tool)
	}
	record := core.NewRecord(collection)
	record.Set("congregation", asked.congregation)
	record.Set("user", asked.user)
	record.Set("conversation", conversation.Id)
	record.Set("question", asked.question)
	record.Set("tools", tools)
	record.Set("answered", resp.Answered)
	record.Set("needs_detail", resp.NeedsDetail)
	record.Set("answer", clipRunes(resp.Context, maxLoggedAnswerRunes))
	record.Set("cards", assistant.Cards(resp.Proposals))
	record.Set("steps", stepsForLog(resp.Steps))
	record.Set("status", asked.status)
	if asked.view != nil {
		record.Set("view", asked.view)
	}
	record.Set("prompt_tokens", resp.PromptTokens)
	record.Set("completion_tokens", resp.CompletionTokens)
	if err := app.Save(record); err != nil {
		log.Printf("assistant: log question: %v", err)
		return conversation.Id, ""
	}
	return conversation.Id, record.Id
}

// conversationIDPattern matches the ids the panel gives new conversations,
// which are PocketBase's own form.
var conversationIDPattern = regexp.MustCompile(`^[a-z0-9]{15}$`)

// maxLoggedSteps bounds the lookups kept with a question, well under the steps
// field's 2 MB limit, as a conversation sends them all whenever it is opened;
// past it, each lookup keeps its first rows only.
const (
	maxLoggedSteps  = 200_000
	trimmedStepRows = 25
)

// stepsForLog is the lookups as the admin saw them, rows and names included,
// trimmed to their first rows when they would be too large to keep.
func stepsForLog(steps []assistant.Step) []assistant.Step {
	if raw, _ := json.Marshal(steps); len(raw) <= maxLoggedSteps {
		return steps
	}
	trimmed := make([]assistant.Step, len(steps))
	for i, step := range steps {
		trimmed[i] = step
		if step.Result != nil && len(step.Result.Rows) > trimmedStepRows {
			result := *step.Result
			result.Rows = result.Rows[:trimmedStepRows]
			trimmed[i].Result = &result
		}
	}
	return trimmed
}

// askerConversation is the user's conversation with this id in the
// congregation, or nil when there is none: no id, deleted, or someone else's.
// taken says the id belongs to someone else's conversation.
func askerConversation(app core.App, id, userID, congregationID string) (conversation *core.Record, taken bool, err error) {
	if id == "" {
		return nil, false, nil
	}
	conversation, err = app.FindRecordById("assistant_conversations", id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if conversation.GetString("user") != userID || conversation.GetString("congregation") != congregationID {
		return nil, true, nil
	}
	return conversation, false, nil
}

// clipRunes cuts text to at most max characters.
func clipRunes(text string, max int) string {
	if utf8.RuneCountInString(text) <= max {
		return text
	}
	return string([]rune(text)[:max])
}
