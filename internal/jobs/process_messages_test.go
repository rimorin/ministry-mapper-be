package jobs

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

const testDataDir = "../../test_pb_data"

// setupMessagesTestApp creates a test app and chdirs to the repo root, since
// processMessage/processMessage load templates via a path relative to the
// server's working directory (repo root at runtime), not the package directory
// `go test` uses by default.
func setupMessagesTestApp(t testing.TB) *tests.TestApp {
	t.Helper()
	app, err := tests.NewTestApp(testDataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir("../.."); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	return app
}

// stubSend installs a fake sendHTMLEmail for the duration of the test and
// returns a pointer to the recorded calls.
type sentEmail struct {
	Recipients []Recipient
	Subject    string
	Body       string
}

func stubSend(t testing.TB, result error) *[]sentEmail {
	t.Helper()
	sent := []sentEmail{}
	original := sendHTMLEmail
	sendHTMLEmail = func(recipients []Recipient, subject, htmlBody string) error {
		sent = append(sent, sentEmail{Recipients: recipients, Subject: subject, Body: htmlBody})
		return result
	}
	t.Cleanup(func() { sendHTMLEmail = original })
	return &sent
}

func addMessage(t testing.TB, app core.App, mapID, message, createdBy, msgType string, read bool) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("messages")
	if err != nil {
		t.Fatalf("failed to find messages collection: %v", err)
	}
	rec := core.NewRecord(col)
	rec.Set("congregation", "testcongalpha01")
	rec.Set("map", mapID)
	rec.Set("message", message)
	rec.Set("created_by", createdBy)
	rec.Set("type", msgType)
	rec.Set("read", read)
	if err := app.SaveNoValidate(rec); err != nil {
		t.Fatalf("failed to save message: %v", err)
	}
	return rec
}

func TestProcessMessages_MarksReadAfterSuccessfulSend(t *testing.T) {
	app := setupMessagesTestApp(t)
	sent := stubSend(t, nil)

	if err := processMessages(app, 60); err != nil {
		t.Fatalf("processMessages returned error: %v", err)
	}

	if len(*sent) != 1 {
		t.Fatalf("expected exactly 1 email sent, got %d", len(*sent))
	}

	msg, err := app.FindRecordById("messages", "testmsgalpha01a")
	if err != nil {
		t.Fatalf("failed to reload message: %v", err)
	}
	if !msg.GetBool("read") {
		t.Error("expected seeded unread publisher message to be marked read after a successful digest send")
	}
}

func TestProcessMessages_ConsolidatesMultipleMapsIntoOneEmailPerCongregation(t *testing.T) {
	app := setupMessagesTestApp(t)
	second := addMessage(t, app, "testmapalpha01b", "Second map message", "Test Publisher 2", "publisher", false)
	sent := stubSend(t, nil)

	if err := processMessages(app, 60); err != nil {
		t.Fatalf("processMessages returned error: %v", err)
	}

	if len(*sent) != 1 {
		t.Fatalf("expected exactly 1 consolidated email for the congregation, got %d", len(*sent))
	}

	body := (*sent)[0].Body
	if !strings.Contains(body, "Blk 100A") || !strings.Contains(body, "Blk 100B") {
		t.Errorf("expected consolidated email to reference both map names, got body: %s", body)
	}

	reloaded, err := app.FindRecordById("messages", second.Id)
	if err != nil {
		t.Fatalf("failed to reload second message: %v", err)
	}
	if !reloaded.GetBool("read") {
		t.Error("expected second map's message to be marked read too")
	}
}

func TestProcessMessages_FailedSendLeavesMessagesUnread(t *testing.T) {
	app := setupMessagesTestApp(t)
	stubSend(t, assertError)

	if err := processMessages(app, 60); err != nil {
		t.Fatalf("processMessages should not itself return an error when a single congregation's send fails: %v", err)
	}

	msg, err := app.FindRecordById("messages", "testmsgalpha01a")
	if err != nil {
		t.Fatalf("failed to reload message: %v", err)
	}
	if msg.GetBool("read") {
		t.Error("message should remain unread when the digest email failed to send")
	}
}

func TestProcessMessages_ExcludesAdministratorTypeMessages(t *testing.T) {
	app := setupMessagesTestApp(t)
	sent := stubSend(t, nil)

	if err := processMessages(app, 60); err != nil {
		t.Fatalf("processMessages returned error: %v", err)
	}

	body := (*sent)[0].Body
	if strings.Contains(body, "Pinned admin notice") || strings.Contains(body, "Unpinned admin notice") {
		t.Error("digest should not include administrator-type messages")
	}

	pinned, err := app.FindRecordById("messages", "testmsgalphapin1")
	if err != nil {
		t.Fatalf("failed to reload pinned message: %v", err)
	}
	if pinned.GetBool("read") {
		t.Error("administrator-type message should not be marked read by the publisher/conductor digest")
	}
}

var assertError = errors.New("stubbed send failure")

// TestProcessMessages_IncludesOldBacklogMessages guards against a regression
// where an added recency filter on the per-congregation fetch made any
// message older than the discovery window permanently unreachable, even
// though the whole point of the digest is to clear the unread backlog.
func TestProcessMessages_IncludesOldBacklogMessages(t *testing.T) {
	app := setupMessagesTestApp(t)

	old := addMessage(t, app, "testmapalpha01b", "Old backlog message", "Test Publisher 3", "publisher", false)
	old.Set("created", time.Now().Add(-500*24*time.Hour))
	if err := app.SaveNoValidate(old); err != nil {
		t.Fatalf("failed to backdate message: %v", err)
	}

	sent := stubSend(t, nil)

	// The seeded testmsgalpha01a (created "now") triggers discovery for
	// testcongalpha01; the 500-day-old message should be swept up too.
	if err := processMessages(app, 60); err != nil {
		t.Fatalf("processMessages returned error: %v", err)
	}

	body := (*sent)[0].Body
	if !strings.Contains(body, "Old backlog message") {
		t.Errorf("expected 500-day-old backlog message to be included once its congregation is triggered, got body: %s", body)
	}

	reloaded, err := app.FindRecordById("messages", old.Id)
	if err != nil {
		t.Fatalf("failed to reload old message: %v", err)
	}
	if !reloaded.GetBool("read") {
		t.Error("expected old backlog message to be marked read")
	}
}

// TestProcessMessages_HandlesMessageWithNoMap guards against a regression
// where a message with an empty/dangling map relation caused a nil-pointer
// panic instead of degrading gracefully.
func TestProcessMessages_HandlesMessageWithNoMap(t *testing.T) {
	app := setupMessagesTestApp(t)

	col, err := app.FindCollectionByNameOrId("messages")
	if err != nil {
		t.Fatalf("failed to find messages collection: %v", err)
	}
	rec := core.NewRecord(col)
	rec.Set("congregation", "testcongalpha01")
	rec.Set("message", "Message with no map")
	rec.Set("created_by", "Test Publisher 4")
	rec.Set("type", "publisher")
	rec.Set("read", false)
	if err := app.SaveNoValidate(rec); err != nil {
		t.Fatalf("failed to save mapless message: %v", err)
	}

	sent := stubSend(t, nil)

	if err := processMessages(app, 60); err != nil {
		t.Fatalf("processMessages returned error: %v", err)
	}

	body := (*sent)[0].Body
	if !strings.Contains(body, "(unknown map)") {
		t.Errorf("expected fallback map name for a message with no map, got body: %s", body)
	}

	reloaded, err := app.FindRecordById("messages", rec.Id)
	if err != nil {
		t.Fatalf("failed to reload mapless message: %v", err)
	}
	if !reloaded.GetBool("read") {
		t.Error("expected mapless message to still be marked read")
	}
}
