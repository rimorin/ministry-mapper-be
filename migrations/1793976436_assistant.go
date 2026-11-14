package migrations

import (
	"slices"

	"ganigeorgiev.com/pocketbase/pocketbase/core"
	m "ganigeorgiev.com/pocketbase/pocketbase/migrations"
	"ganigeorgiev.com/pocketbase/pocketbase/tools/types"
)

// The admin assistant's rate limits. Each question costs model calls, so a burst
// from one client is held well below the general /api/ limit; ratings and card
// outcomes get their own allowance, as an exact label wins over the prefix. They
// take effect wherever PB_ENABLE_RATE_LIMITING turned rate limiting on.
var assistantRateLimits = []core.RateLimitRule{
	{Label: "/assistant/", MaxRequests: 10, Duration: 60},
	{Label: "/assistant/feedback", MaxRequests: 60, Duration: 60},
}

// assistantQuestionFields are the question log's fields after its relations:
//   - question, tools, answered and the token counts: what was asked and how
//   - needs_detail: the reply asked the admin for something, such as which map;
//     a step on the way, so the unanswered questions worth reviewing are
//     answered = false and needs_detail = false
//   - answer: as the model wrote it, with people as tokens, never names
//   - rating: 1 for thumbs up, -1 for thumbs down, 0 for none; reason says why
//     for a thumbs down, such as wrong_figures
//   - actions: each card's kind and its outcome, such as {"share_link": "undone"}
func assistantQuestionFields() []core.Field {
	return []core.Field{
		&core.TextField{Name: "question", Required: true, Max: 1000},
		&core.JSONField{Name: "tools"},
		&core.BoolField{Name: "answered"},
		&core.BoolField{Name: "needs_detail"},
		&core.NumberField{Name: "prompt_tokens", OnlyInt: true},
		&core.NumberField{Name: "completion_tokens", OnlyInt: true},
		&core.TextField{Name: "answer", Max: 5000},
		&core.NumberField{Name: "rating", OnlyInt: true, Min: types.Pointer(-1.0), Max: types.Pointer(1.0)},
		&core.TextField{Name: "reason", Max: 32},
		&core.JSONField{Name: "actions", MaxSize: 2000},
		&core.AutodateField{Name: "created", OnCreate: true},
	}
}

// Adds the admin assistant's question log, assistant_questions, and its rate
// limits. Rules stay nil (superuser only); the assistant routes write the log
// server-side. A database that already has part of it, from before these steps
// were one migration, gains only what it lacks.
func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("assistant_questions")
		if err != nil {
			congregations, err := app.FindCollectionByNameOrId("congregations")
			if err != nil {
				return err
			}
			users, err := app.FindCollectionByNameOrId("users")
			if err != nil {
				return err
			}
			collection = core.NewBaseCollection("assistant_questions")
			collection.Fields.Add(
				&core.RelationField{Name: "congregation", CollectionId: congregations.Id, MaxSelect: 1, Required: true, CascadeDelete: true},
				&core.RelationField{Name: "user", CollectionId: users.Id, MaxSelect: 1},
			)
			collection.AddIndex("idx_assistant_questions_congregation_created", false, "`congregation`, `created`", "")
		}
		for _, field := range assistantQuestionFields() {
			if collection.Fields.GetByName(field.GetName()) == nil {
				collection.Fields.Add(field)
			}
		}
		if err := app.Save(collection); err != nil {
			return err
		}

		settings := app.Settings()
		for _, rule := range assistantRateLimits {
			if !slices.ContainsFunc(settings.RateLimits.Rules, func(r core.RateLimitRule) bool { return r.Label == rule.Label }) {
				settings.RateLimits.Rules = append(settings.RateLimits.Rules, rule)
			}
		}
		return app.Save(settings)
	}, func(app core.App) error {
		settings := app.Settings()
		settings.RateLimits.Rules = slices.DeleteFunc(settings.RateLimits.Rules, func(r core.RateLimitRule) bool {
			return slices.ContainsFunc(assistantRateLimits, func(a core.RateLimitRule) bool { return a.Label == r.Label })
		})
		if err := app.Save(settings); err != nil {
			return err
		}
		collection, err := app.FindCollectionByNameOrId("assistant_questions")
		if err != nil {
			return nil
		}
		return app.Delete(collection)
	})
}
