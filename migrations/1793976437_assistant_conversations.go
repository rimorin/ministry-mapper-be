package migrations

import (
	"slices"

	"ganigeorgiev.com/pocketbase/pocketbase/core"
	m "ganigeorgiev.com/pocketbase/pocketbase/migrations"
)

// Browsing history gets its own allowance, so opening past conversations never
// uses up the questions' (an exact label wins over the /assistant/ prefix).
var assistantHistoryRateLimit = core.RateLimitRule{Label: "/assistant/history", MaxRequests: 60, Duration: 60}

// Groups the assistant's questions into conversations, so the asker can find,
// rename, continue and delete them later:
//   - assistant_conversations: whose it is and where, a title the asker may set
//     (empty shows the first question), and last_question, which orders the list
//     and ages the conversation out; renaming does not count as activity
//   - assistant_questions.conversation: the conversation a question belongs to;
//     deleting a conversation deletes its questions
//
// Rules stay nil (superuser only); the assistant routes read and write both.
func init() {
	m.Register(func(app core.App) error {
		questions, err := app.FindCollectionByNameOrId("assistant_questions")
		if err != nil {
			return err
		}
		conversations, err := app.FindCollectionByNameOrId("assistant_conversations")
		if err != nil {
			congregationsCol, err := app.FindCollectionByNameOrId("congregations")
			if err != nil {
				return err
			}
			users, err := app.FindCollectionByNameOrId("users")
			if err != nil {
				return err
			}
			conversations = core.NewBaseCollection("assistant_conversations")
			conversations.Fields.Add(
				&core.RelationField{Name: "congregation", CollectionId: congregationsCol.Id, MaxSelect: 1, Required: true, CascadeDelete: true},
				&core.RelationField{Name: "user", CollectionId: users.Id, MaxSelect: 1, Required: true, CascadeDelete: true},
				&core.TextField{Name: "title", Max: 120},
				&core.DateField{Name: "last_question"},
				&core.AutodateField{Name: "created", OnCreate: true},
			)
			conversations.AddIndex("idx_assistant_conversations_owner_last", false, "`congregation`, `user`, `last_question`", "")
			if err := app.Save(conversations); err != nil {
				return err
			}
		}
		if questions.Fields.GetByName("conversation") == nil {
			questions.Fields.Add(&core.RelationField{
				Name: "conversation", CollectionId: conversations.Id, MaxSelect: 1, CascadeDelete: true,
			})
			questions.AddIndex("idx_assistant_questions_conversation", false, "`conversation`, `created`", "")
			if err := app.Save(questions); err != nil {
				return err
			}
		}

		settings := app.Settings()
		if !slices.ContainsFunc(settings.RateLimits.Rules, func(r core.RateLimitRule) bool {
			return r.Label == assistantHistoryRateLimit.Label
		}) {
			settings.RateLimits.Rules = append(settings.RateLimits.Rules, assistantHistoryRateLimit)
		}
		return app.Save(settings)
	}, func(app core.App) error {
		settings := app.Settings()
		settings.RateLimits.Rules = slices.DeleteFunc(settings.RateLimits.Rules, func(r core.RateLimitRule) bool {
			return r.Label == assistantHistoryRateLimit.Label
		})
		if err := app.Save(settings); err != nil {
			return err
		}
		if questions, err := app.FindCollectionByNameOrId("assistant_questions"); err == nil {
			questions.RemoveIndex("idx_assistant_questions_conversation")
			questions.Fields.RemoveByName("conversation")
			if err := app.Save(questions); err != nil {
				return err
			}
		}
		conversations, err := app.FindCollectionByNameOrId("assistant_conversations")
		if err != nil {
			return nil
		}
		return app.Delete(conversations)
	})
}
