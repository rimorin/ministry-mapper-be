//go:build testdata

package jobs

import (
	"testing"
	"time"

	"ganigeorgiev.com/pocketbase/dbx"
	"ganigeorgiev.com/pocketbase/pocketbase/tests"
	"ganigeorgiev.com/pocketbase/pocketbase/tools/types"
)

func TestCleanUpAssistantQuestions_DeletesOnlyExpired(t *testing.T) {
	app, err := tests.NewTestApp("../../test_pb_data")
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	insert := func(id string, age time.Duration) {
		t.Helper()
		created, _ := types.ParseDateTime(now.Add(-age))
		_, err := app.DB().NewQuery(`INSERT INTO assistant_questions (id, congregation, question, created)
			VALUES ({:id}, 'testcongalpha01', 'q', {:created})`).
			Bind(dbx.Params{"id": id, "created": created.String()}).Execute()
		if err != nil {
			t.Fatal(err)
		}
	}
	insert("assistqexpired", assistantQuestionRetention+time.Hour)
	insert("assistqkeptnow", assistantQuestionRetention-time.Hour)

	if err := cleanUpAssistantQuestions(app, now); err != nil {
		t.Fatal(err)
	}

	var ids []struct {
		ID string `db:"id"`
	}
	if err := app.DB().NewQuery("SELECT id FROM assistant_questions").All(&ids); err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0].ID != "assistqkeptnow" {
		t.Fatalf("remaining = %v, want only assistqkeptnow", ids)
	}
}

func TestCleanUpAssistantQuestions_KeepsAConversationUntilItsLastQuestionExpires(t *testing.T) {
	app, err := tests.NewTestApp("../../test_pb_data")
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	at := func(age time.Duration) string {
		dt, _ := types.ParseDateTime(now.Add(-age))
		return dt.String()
	}
	exec := func(query string, params dbx.Params) {
		t.Helper()
		if _, err := app.DB().NewQuery(query).Bind(params).Execute(); err != nil {
			t.Fatal(err)
		}
	}
	conversation := func(id string, last time.Duration) {
		exec(`INSERT INTO assistant_conversations (id, congregation, user, last_question, created)
			VALUES ({:id}, 'testcongalpha01', 'testuseralpha01', {:last}, {:last})`,
			dbx.Params{"id": id, "last": at(last)})
	}
	question := func(id, conversation string, age time.Duration) {
		exec(`INSERT INTO assistant_questions (id, congregation, conversation, question, created)
			VALUES ({:id}, 'testcongalpha01', {:conversation}, 'q', {:created})`,
			dbx.Params{"id": id, "conversation": conversation, "created": at(age)})
	}
	conversation("assistcstale001", assistantQuestionRetention+time.Hour)
	question("assistqstale001", "assistcstale001", assistantQuestionRetention+2*time.Hour)
	conversation("assistcactive01", time.Hour)
	question("assistqoldbutin", "assistcactive01", assistantQuestionRetention+time.Hour)
	question("assistqrecentin", "assistcactive01", time.Hour)

	if err := cleanUpAssistantQuestions(app, now); err != nil {
		t.Fatal(err)
	}

	count := func(table string) int {
		var n struct {
			N int `db:"n"`
		}
		if err := app.DB().NewQuery("SELECT COUNT(*) AS n FROM " + table).One(&n); err != nil {
			t.Fatal(err)
		}
		return n.N
	}
	if got := count("assistant_conversations"); got != 1 {
		t.Errorf("conversations = %d, want the active one only", got)
	}
	if got := count("assistant_questions"); got != 2 {
		t.Errorf("questions = %d, want both of the active conversation's", got)
	}
}
