package jobs

import (
	"log"
	"time"

	"ganigeorgiev.com/pocketbase/dbx"
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"ganigeorgiev.com/pocketbase/pocketbase/tools/types"
)

// assistantQuestionRetention is how long the admin assistant keeps a
// conversation after its last question. The questions show which query
// functions to build next and let the asker look back; older ones add nothing
// and only hold what admins typed.
const assistantQuestionRetention = 90 * 24 * time.Hour

// cleanUpAssistantQuestions deletes conversations whose last question is older
// than the retention period, with their questions, and questions logged before
// conversations existed once they are that old. Raw DELETEs are enough: both
// collections are superuser-only, so nothing subscribes to them. Questions go
// first, as a raw DELETE does not cascade.
func cleanUpAssistantQuestions(app core.App, now time.Time) error {
	cutoff, _ := types.ParseDateTime(now.Add(-assistantQuestionRetention))
	params := dbx.Params{"cutoff": cutoff.String()}
	result, err := app.DB().NewQuery(`DELETE FROM assistant_questions
		WHERE conversation IN (SELECT id FROM assistant_conversations WHERE last_question < {:cutoff})
		   OR (COALESCE(conversation, '') = '' AND created < {:cutoff})`).Bind(params).Execute()
	if err != nil {
		return err
	}
	questions, _ := result.RowsAffected()
	result, err = app.DB().NewQuery(`DELETE FROM assistant_conversations WHERE last_question < {:cutoff}`).
		Bind(params).Execute()
	if err != nil {
		return err
	}
	if conversations, _ := result.RowsAffected(); questions > 0 || conversations > 0 {
		log.Printf("cleanUpAssistantQuestions: deleted %d conversations and %d questions older than %s",
			conversations, questions, cutoff)
	}
	return nil
}
