package handlers

import (
	"encoding/json"
	"slices"
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
	// assistantHistoryPage is how many conversations one list request returns.
	assistantHistoryPage = 30
	// maxConversationTitle fits the title field of assistant_conversations.
	maxConversationTitle = 120
)

type assistantHistoryRequest struct {
	Congregation string `json:"congregation"`
	// Action is list, open, rename or delete.
	Action string `json:"action"`
	// Conversation is the one to open, rename or delete.
	Conversation string `json:"conversation"`
	// Title is the new name; empty goes back to the first question.
	Title string `json:"title"`
	// Offset skips conversations already listed, for "Show more".
	Offset int `json:"offset"`
}

type conversationSummary struct {
	ID            string `json:"id" db:"id"`
	Title         string `json:"title" db:"title"`
	FirstQuestion string `json:"firstQuestion" db:"first_question"`
	Questions     int    `json:"questions" db:"questions"`
	LastQuestion  string `json:"lastQuestion" db:"last_question"`
}

type historyTurn struct {
	ID       string `json:"id"`
	Question string `json:"question"`
	// Answer has people's names put back, to show; Context keeps the tokens,
	// to send back as history if the asker continues.
	Answer  string `json:"answer"`
	Context string `json:"context"`
	Asked   string `json:"asked"`
	// Cards are the changes the answer prepared and what became of them.
	Cards []assistant.Card `json:"cards"`
	// Steps are the lookups and the rows they found, as the asker saw them.
	Steps json.RawMessage `json:"steps"`
	// Status is "" when answered, or failed or stopped.
	Status string `json:"status"`
	// View is the place open when asking, or null.
	View json.RawMessage `json:"view"`
	// Rating is the asker's thumbs up (1), down (-1) or none (0), and Reason why.
	Rating int    `json:"rating"`
	Reason string `json:"reason"`
}

// HandleAssistantHistory lets the asker list, open, rename and delete their own
// assistant conversations in a congregation they still belong to. Anyone
// else's conversation is reported as not found.
func HandleAssistantHistory(c *core.RequestEvent, app core.App) error {
	var req assistantHistoryRequest
	if err := c.BindBody(&req); err != nil {
		return apis.NewBadRequestError("Invalid request body", nil)
	}
	if req.Congregation == "" {
		return apis.NewBadRequestError("congregation is required", nil)
	}
	// Past answers hold the congregation's data, so leaving it ends access to them.
	role, err := assistantRole(app, c.Auth.Id, req.Congregation)
	if err != nil {
		return newServerError(err)
	}
	if role == "" {
		return apis.NewForbiddenError("Congregation access required", nil)
	}

	if req.Action == "list" {
		return listConversations(c, app, req)
	}
	if req.Action != "open" && req.Action != "rename" && req.Action != "delete" {
		return apis.NewBadRequestError("action must be list, open, rename or delete", nil)
	}
	conversation, _, err := askerConversation(app, req.Conversation, c.Auth.Id, req.Congregation)
	if err != nil {
		return newServerError(err)
	}
	if conversation == nil {
		return apis.NewNotFoundError("Conversation not found", nil)
	}
	switch req.Action {
	case "open":
		return openConversation(c, app, conversation)
	case "rename":
		title := strings.TrimSpace(req.Title)
		if utf8.RuneCountInString(title) > maxConversationTitle {
			return apis.NewBadRequestError("title is too long", nil)
		}
		conversation.Set("title", title)
		if err := app.Save(conversation); err != nil {
			return newServerError(err)
		}
		return c.NoContent(204)
	default:
		// Its questions go with it (cascade delete on assistant_questions.conversation).
		if err := app.Delete(conversation); err != nil {
			return newServerError(err)
		}
		return c.NoContent(204)
	}
}

// listConversations returns the asker's conversations, latest question first.
func listConversations(c *core.RequestEvent, app core.App, req assistantHistoryRequest) error {
	var rows []conversationSummary
	err := app.DB().NewQuery(`
		SELECT c.id, COALESCE(c.title, '') AS title, COALESCE(c.last_question, '') AS last_question,
		       COALESCE((SELECT q.question FROM assistant_questions q WHERE q.conversation = c.id
		                 ORDER BY q.created, q.id LIMIT 1), '') AS first_question,
		       (SELECT COUNT(*) FROM assistant_questions q WHERE q.conversation = c.id) AS questions
		FROM assistant_conversations c
		WHERE c.congregation = {:congregation} AND c.user = {:user}
		ORDER BY c.last_question DESC, c.id DESC
		LIMIT {:limit} OFFSET {:offset}`).
		Bind(dbx.Params{
			"congregation": req.Congregation, "user": c.Auth.Id,
			"limit": assistantHistoryPage + 1, "offset": max(req.Offset, 0),
		}).All(&rows)
	if err != nil {
		return newServerError(err)
	}
	more := len(rows) > assistantHistoryPage
	if more {
		rows = rows[:assistantHistoryPage]
	}
	for i := range rows {
		rows[i].LastQuestion = isoTime(rows[i].LastQuestion)
	}
	if rows == nil {
		rows = []conversationSummary{}
	}
	return c.JSON(200, map[string]any{"conversations": rows, "more": more})
}

// openConversation returns a conversation's questions and answers, oldest
// first, with the names its answers mention put back for the asker.
func openConversation(c *core.RequestEvent, app core.App, conversation *core.Record) error {
	var rows []struct {
		ID       string `db:"id"`
		Question string `db:"question"`
		Answer   string `db:"answer"`
		Created  string `db:"created"`
		Cards    string `db:"cards"`
		Steps    string `db:"steps"`
		Status   string `db:"status"`
		View     string `db:"view"`
		Rating   int    `db:"rating"`
		Reason   string `db:"reason"`
	}
	err := app.DB().NewQuery(`
		SELECT id, question, COALESCE(answer, '') AS answer, created,
		       COALESCE(cards, '') AS cards, COALESCE(steps, '') AS steps, COALESCE(status, '') AS status,
		       COALESCE(view, '') AS view, COALESCE(rating, 0) AS rating, COALESCE(reason, '') AS reason
		FROM assistant_questions WHERE conversation = {:conversation}
		ORDER BY created, id`).
		Bind(dbx.Params{"conversation": conversation.Id}).All(&rows)
	if err != nil {
		return newServerError(err)
	}
	answers := make([]string, len(rows))
	for i, r := range rows {
		answers[i] = r.Answer
	}
	congregationID := conversation.GetString("congregation")
	restored, people, err := assistant.RestoreNames(assistant.Scope{App: app, CongregationID: congregationID}, answers)
	if err != nil {
		return newServerError(err)
	}
	turns := make([]historyTurn, len(rows))
	for i, r := range rows {
		cards := []assistant.Card{}
		if r.Cards != "" {
			_ = json.Unmarshal([]byte(r.Cards), &cards) // a question without cards keeps []
		}
		turns[i] = historyTurn{
			ID: r.ID, Question: r.Question, Answer: restored[i], Context: r.Answer,
			Asked: isoTime(r.Created), Cards: cards,
			Steps: jsonOr(r.Steps, "[]"), Status: r.Status, View: jsonOr(r.View, "null"),
			Rating: r.Rating, Reason: r.Reason,
		}
	}
	if err := keepLiveLinks(app, congregationID, turns); err != nil {
		return newServerError(err)
	}
	return c.JSON(200, map[string]any{
		"conversation": map[string]any{"id": conversation.Id},
		"turns":        turns,
		"people":       people,
	})
}

// keepLiveLinks keeps the link each card created only while it still works in
// this congregation, so history offers to share only those.
func keepLiveLinks(app core.App, congregationID string, turns []historyTurn) error {
	var ids []any
	for _, turn := range turns {
		for _, card := range turn.Cards {
			if card.Record != "" && card.Outcome == "done" {
				ids = append(ids, card.Record)
			}
		}
	}
	if len(ids) == 0 {
		return nil
	}
	var live []string
	err := app.DB().Select("id").From("assignments").
		Where(dbx.In("id", ids...)).
		AndWhere(dbx.HashExp{"congregation": congregationID}).
		AndWhere(dbx.NewExp("expiry_date > {:now}", dbx.Params{"now": types.NowDateTime().String()})).
		Column(&live)
	if err != nil {
		return err
	}
	for _, turn := range turns {
		for i := range turn.Cards {
			if card := &turn.Cards[i]; card.Outcome == "done" && !slices.Contains(live, card.Record) {
				card.Record = ""
			}
		}
	}
	return nil
}

// jsonOr passes a stored JSON value through as it is, or the default when empty.
func jsonOr(stored, empty string) json.RawMessage {
	if strings.TrimSpace(stored) == "" || stored == "null" {
		return json.RawMessage(empty)
	}
	return json.RawMessage(stored)
}

// isoTime writes a stored datetime as RFC 3339, which every browser parses.
func isoTime(stored string) string {
	dt, err := types.ParseDateTime(stored)
	if err != nil || dt.IsZero() {
		return ""
	}
	return dt.Time().UTC().Format(time.RFC3339)
}
