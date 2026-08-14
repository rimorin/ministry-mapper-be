//go:build testdata

package setup

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// buildTestMux wires the app's routes the same way ApiScenario does, but hands
// back the mux so a test can drive several requests at once.
func buildTestMux(t *testing.T, testApp *tests.TestApp) http.Handler {
	t.Helper()

	baseRouter, err := apis.NewRouter(testApp)
	if err != nil {
		t.Fatal(err)
	}

	serveEvent := new(core.ServeEvent)
	serveEvent.App = testApp
	serveEvent.Router = baseRouter

	var mux http.Handler
	err = testApp.OnServe().Trigger(serveEvent, func(e *core.ServeEvent) error {
		built, err := e.Router.BuildMux()
		if err != nil {
			return err
		}
		mux = built
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	return mux
}

func codesSharingASequence(t *testing.T, testApp *tests.TestApp, mapId string) []string {
	t.Helper()

	rows := []struct {
		Sequence int    `db:"sequence"`
		Codes    string `db:"codes"`
	}{}
	err := testApp.DB().NewQuery(`
		SELECT sequence, GROUP_CONCAT(DISTINCT code) AS codes
		FROM addresses
		WHERE map = {:map}
		GROUP BY sequence
		HAVING COUNT(DISTINCT code) > 1
	`).Bind(dbx.Params{"map": mapId}).All(&rows)
	if err != nil {
		t.Fatal(err)
	}

	clashes := make([]string, 0, len(rows))
	for _, row := range rows {
		clashes = append(clashes, fmt.Sprintf("sequence %d: %s", row.Sequence, row.Codes))
	}

	return clashes
}

// /map/code/add derives the new sequence from MAX(sequence). Reading that
// outside the transaction let simultaneous additions to one map observe the
// same high-water mark and hand the same sequence to different codes, which
// puts their cells under each other's column header and drops one of them from
// the Excel report.
func TestHandleMapAddConcurrentAdditionsGetDistinctSequences(t *testing.T) {
	adminToken, err := generateToken("admin@alpha.test")
	if err != nil {
		t.Fatal(err)
	}

	testApp := setupTestApp(t)
	defer testApp.Cleanup()

	mux := buildTestMux(t, testApp)

	const additions = 8
	var wg sync.WaitGroup
	statuses := make([]int, additions)

	for i := range additions {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			body := fmt.Sprintf(`{"map":"testmapalpha01b","codes":["X%d"]}`, i)
			req := httptest.NewRequest(http.MethodPost, "/map/code/add", strings.NewReader(body))
			req.Header.Set("content-type", "application/json")
			req.Header.Set("Authorization", adminToken)

			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, req)
			statuses[i] = recorder.Code
		}(i)
	}
	wg.Wait()

	// Individual requests may lose a write lock and fail; that is acceptable.
	// What must never happen is two codes landing on one sequence.
	if clashes := codesSharingASequence(t, testApp, "testmapalpha01b"); len(clashes) > 0 {
		t.Errorf("concurrent additions produced %d sequence collision(s):\n  %s",
			len(clashes), strings.Join(clashes, "\n  "))
	}

	var succeeded int
	for _, status := range statuses {
		if status == http.StatusOK {
			succeeded++
		}
	}
	if succeeded == 0 {
		t.Fatalf("no addition succeeded, statuses=%v — the test proved nothing", statuses)
	}
	t.Logf("%d/%d concurrent additions committed", succeeded, additions)
}

// The sequential path must also keep sequences distinct, and must never reuse a
// sequence already present in the map.
func TestHandleMapAddSequentialAdditionsGetDistinctSequences(t *testing.T) {
	adminToken, err := generateToken("admin@alpha.test")
	if err != nil {
		t.Fatal(err)
	}

	testApp := setupTestApp(t)
	defer testApp.Cleanup()

	mux := buildTestMux(t, testApp)

	for _, codes := range []string{`["Y1","Y2"]`, `["Y3"]`, `["Y4","Y5","Y6"]`} {
		body := fmt.Sprintf(`{"map":"testmapalpha01b","codes":%s}`, codes)
		req := httptest.NewRequest(http.MethodPost, "/map/code/add", strings.NewReader(body))
		req.Header.Set("content-type", "application/json")
		req.Header.Set("Authorization", adminToken)

		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusOK {
			t.Fatalf("adding %s returned %d: %s", codes, recorder.Code, recorder.Body)
		}
	}

	if clashes := codesSharingASequence(t, testApp, "testmapalpha01b"); len(clashes) > 0 {
		t.Errorf("sequential additions produced collision(s):\n  %s", strings.Join(clashes, "\n  "))
	}

	// Every code in the map should now hold exactly one sequence, and the whole
	// set should be distinct.
	var counts struct {
		Codes     int `db:"codes"`
		Sequences int `db:"sequences"`
	}
	err = testApp.DB().NewQuery(`
		SELECT COUNT(DISTINCT code) AS codes, COUNT(DISTINCT sequence) AS sequences
		FROM addresses WHERE map = {:map}
	`).Bind(dbx.Params{"map": "testmapalpha01b"}).One(&counts)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Codes != counts.Sequences {
		t.Errorf("%d codes share only %d distinct sequences", counts.Codes, counts.Sequences)
	}
}
