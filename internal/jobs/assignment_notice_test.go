//go:build testdata

package jobs

import (
	"strings"
	"testing"
	"time"

	"ganigeorgiev.com/pocketbase/pocketbase/core"
)

func newAssignment(t testing.TB, app core.App, linkType string, assignees ...string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("assignments")
	if err != nil {
		t.Fatal(err)
	}
	rec := core.NewRecord(col)
	rec.Set("map", "testmapalpha01a")
	rec.Set("congregation", "testcongalpha01")
	rec.Set("user", "testuseralpha02")
	rec.Set("assignees", assignees)
	rec.Set("type", linkType)
	rec.Set("publisher", "Alpha Publisher")
	rec.Set("expiry_date", time.Now().Add(4*time.Hour).UTC().Format("2006-01-02 15:04:05.000Z"))
	if err := app.SaveNoValidate(rec); err != nil {
		t.Fatal(err)
	}
	return rec
}

func TestNotifyAssignees_EmailsEachAssigneeButNotTheCreator(t *testing.T) {
	app := setupMessagesTestApp(t)
	sent := stubSend(t, nil)
	t.Setenv("PB_APP_URL", "https://app.example/")

	link := newAssignment(t, app, "personal", "testuseralpha04", "testuseralpha02", "testuseralpha03")
	sendAssigneeEmails(assigneeEmails(app, link, nil), link.Id)

	if len(*sent) != 2 {
		t.Fatalf("sent %d emails, want 2", len(*sent))
	}
	first := (*sent)[0]
	if got := first.Recipients[0].Email; got != "publisher@alpha.test" {
		t.Errorf("first recipient = %q, want the publisher", got)
	}
	if got := (*sent)[1].Recipients[0].Email; got != "readonly@alpha.test" {
		t.Errorf("second recipient = %q, want the read-only user", got)
	}
	for _, want := range []string{
		"https://app.example/map/" + link.Id,
		"Alpha Conductor gave you",
		"as a personal territory",
		"Hello Alpha Publisher",
	} {
		if !strings.Contains(first.Body, want) {
			t.Errorf("email body is missing %q", want)
		}
	}
}

func TestNotifyAssignees_SkipsLinksWithoutAssigneesAndDisabledAccounts(t *testing.T) {
	app := setupMessagesTestApp(t)
	sent := stubSend(t, nil)

	sendAssigneeEmails(assigneeEmails(app, newAssignment(t, app, "normal"), nil), "")

	publisher, err := app.FindRecordById("users", "testuseralpha04")
	if err != nil {
		t.Fatal(err)
	}
	publisher.Set("disabled", true)
	if err := app.SaveNoValidate(publisher); err != nil {
		t.Fatal(err)
	}
	sendAssigneeEmails(assigneeEmails(app, newAssignment(t, app, "normal", "testuseralpha04"), nil), "")

	if len(*sent) != 0 {
		t.Fatalf("sent %d emails, want none", len(*sent))
	}
}

func TestNotifyAssignees_RespectsTheOptOutAndTheOnlyList(t *testing.T) {
	app := setupMessagesTestApp(t)
	sent := stubSend(t, nil)

	link := newAssignment(t, app, "normal", "testuseralpha04", "testuseralpha03")
	sendAssigneeEmails(assigneeEmails(app, link, []string{"testuseralpha03"}), link.Id)
	if len(*sent) != 1 || (*sent)[0].Recipients[0].Email != "readonly@alpha.test" {
		t.Fatalf("only the added account should be emailed, got %+v", *sent)
	}

	publisher, err := app.FindRecordById("users", "testuseralpha04")
	if err != nil {
		t.Fatal(err)
	}
	publisher.Set("assignment_emails_off", true)
	if err := app.SaveNoValidate(publisher); err != nil {
		t.Fatal(err)
	}
	*sent = nil
	sendAssigneeEmails(assigneeEmails(app, link, []string{"testuseralpha04"}), link.Id)
	if len(*sent) != 0 {
		t.Fatalf("an account that turned the emails off was emailed")
	}
}

func TestEnglishName(t *testing.T) {
	for stored, want := range map[string]string{
		"Blk 100A":                       "Blk 100A",
		`{"en":"Sunset Road","zh":"日落"}`: "Sunset Road",
		`{"zh":"日落"}`:                    "日落",
		"713":                            "713",
	} {
		if got := englishName(stored); got != want {
			t.Errorf("englishName(%q) = %q, want %q", stored, got, want)
		}
	}
}
