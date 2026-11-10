package migrations

import (
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	m "ganigeorgiev.com/pocketbase/pocketbase/migrations"
)

// Keeps everything an answer showed, so reopening it from history, after a
// reload or on another day, brings back the same answer, not only its text:
//   - cards: each change card and what became of it, replacing actions, which
//     kept one outcome per kind of card and so lost all but one share link
//   - steps: each lookup and the rows it found, names included
//   - view: the territory and map the admin had open when asking
//   - status: "" when answered, or failed or stopped, so those questions stay too
func init() {
	m.Register(func(app core.App) error {
		questions, err := app.FindCollectionByNameOrId("assistant_questions")
		if err != nil {
			return err
		}
		for _, field := range []core.Field{
			&core.JSONField{Name: "cards", MaxSize: 200_000},
			&core.JSONField{Name: "steps", MaxSize: 2_000_000},
			&core.JSONField{Name: "view", MaxSize: 2000},
			&core.TextField{Name: "status", Max: 16},
		} {
			if questions.Fields.GetByName(field.GetName()) == nil {
				questions.Fields.Add(field)
			}
		}
		questions.Fields.RemoveByName("actions")
		return app.Save(questions)
	}, func(app core.App) error {
		questions, err := app.FindCollectionByNameOrId("assistant_questions")
		if err != nil {
			return nil
		}
		for _, name := range []string{"cards", "steps", "view", "status"} {
			questions.Fields.RemoveByName(name)
		}
		if questions.Fields.GetByName("actions") == nil {
			questions.Fields.Add(&core.JSONField{Name: "actions", MaxSize: 2000})
		}
		return app.Save(questions)
	})
}
