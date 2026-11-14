package handlers

import (
	"database/sql"
	"errors"
	"regexp"
	"slices"
	"unicode/utf8"

	"ministry-mapper/internal/assistant"

	"ganigeorgiev.com/pocketbase/pocketbase/apis"
	"ganigeorgiev.com/pocketbase/pocketbase/core"
)

// The outcomes the panel reports for a card, as its status names them.
var cardOutcomes = []string{"done", "cancelled", "undone", "failed", "stale"}

// The reasons the panel offers after a thumbs down.
var downReasons = []string{"wrong_figures", "misunderstood", "incomplete", "wrong_change", "other"}

// proposalID matches the ids the assistant gives its cards.
var proposalID = regexp.MustCompile(`^[0-9a-f]{16}$`)

// recordID matches the record a link card created. Share link ids are long and
// mixed-case, as they double as the link's token.
var recordID = regexp.MustCompile(`^[A-Za-z0-9]{1,64}$`)

// maxCardPerson bounds the name a role card reports it applied to, and
// maxCardLabel the map a quick link reports it picked.
const (
	maxCardPerson = 100
	maxCardLabel  = 200
)

type assistantFeedbackRequest struct {
	Question string `json:"question"`
	// Rating is 1 for thumbs up, -1 for thumbs down and 0 to take it back.
	Rating *int `json:"rating"`
	// Reason says why, with a thumbs down only.
	Reason string `json:"reason"`
	// Proposal, Outcome and Person report what happened to one of the answer's
	// cards, and for a role change, whom it applied to.
	Proposal string `json:"proposal"`
	Outcome  string `json:"outcome"`
	Person   string `json:"person"`
	// Record and Label are the link a link card created and the map it picked.
	Record string `json:"record"`
	Label  string `json:"label"`
}

// HandleAssistantFeedback records the asker's rating of an answer, or what
// happened to one of its cards, on the logged question. Only the person who
// asked may; anyone else is told the question does not exist.
func HandleAssistantFeedback(c *core.RequestEvent, app core.App) error {
	var req assistantFeedbackRequest
	if err := c.BindBody(&req); err != nil {
		return apis.NewBadRequestError("Invalid request body", nil)
	}
	if req.Rating == nil && req.Proposal == "" {
		return apis.NewBadRequestError("rating or proposal is required", nil)
	}
	if req.Rating != nil && (*req.Rating < -1 || *req.Rating > 1) {
		return apis.NewBadRequestError("rating must be -1, 0 or 1", nil)
	}
	if req.Reason != "" && (req.Rating == nil || *req.Rating != -1 || !slices.Contains(downReasons, req.Reason)) {
		return apis.NewBadRequestError("a reason goes with a thumbs down, and must be a known one", nil)
	}
	if req.Proposal != "" && (!proposalID.MatchString(req.Proposal) || !slices.Contains(cardOutcomes, req.Outcome) ||
		utf8.RuneCountInString(req.Person) > maxCardPerson || utf8.RuneCountInString(req.Label) > maxCardLabel ||
		(req.Record != "" && !recordID.MatchString(req.Record))) {
		return apis.NewBadRequestError("proposal, outcome, person, record or label is not valid", nil)
	}

	// Read and saved in one transaction: an answer's cards share one field, and
	// two reports at once would otherwise each save over the other's.
	err := app.RunInTransaction(func(tx core.App) error {
		record, err := tx.FindRecordById("assistant_questions", req.Question)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && record.GetString("user") != c.Auth.Id) {
			return apis.NewNotFoundError("Question not found", nil)
		}
		if err != nil {
			return err
		}
		if req.Rating != nil {
			record.Set("rating", *req.Rating)
			// A new rating replaces the old one's reason.
			record.Set("reason", req.Reason)
		}
		if req.Proposal != "" {
			var cards []assistant.Card
			_ = record.UnmarshalJSONField("cards", &cards)
			i := slices.IndexFunc(cards, func(card assistant.Card) bool { return card.Proposal.ID == req.Proposal })
			if i < 0 {
				return apis.NewNotFoundError("Card not found", nil)
			}
			cards[i].Outcome = req.Outcome
			if req.Person != "" {
				cards[i].Person = req.Person
			}
			if req.Record != "" {
				cards[i].Record, cards[i].Label = req.Record, req.Label
			}
			record.Set("cards", cards)
		}
		return tx.Save(record)
	})
	if err != nil {
		return wrapTransactionError(err)
	}
	return c.NoContent(204)
}
