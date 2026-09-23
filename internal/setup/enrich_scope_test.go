//go:build testdata

package setup

import (
	"net/http"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/subscriptions"
)

// seedCrossAssignments gives the Beta administrator one assignment on an Alpha
// map and one on a Beta map, so an Alpha conductor listing that user's history
// gets one row in scope and one out of it in the same response.
func seedCrossAssignments(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("assignments")
	if err != nil {
		t.Fatal(err)
	}
	expiry := time.Now().UTC().Add(24 * time.Hour).Format("2006-01-02 15:04:05.000Z")
	for _, a := range []struct{ id, mapId, congId string }{
		{"crossassignalph", "testmapalpha01a", "testcongalpha01"},
		{"crossassignbeta", "testmapbeta001a", "testcongbeta001"},
	} {
		rec := core.NewRecord(col)
		rec.Id = a.id
		rec.Set("map", a.mapId)
		rec.Set("congregation", a.congId)
		rec.Set("user", "testuserbeta001")
		rec.Set("type", "normal")
		rec.Set("publisher", "Cross Probe")
		rec.Set("expiry_date", expiry)
		if err := app.SaveNoValidate(rec); err != nil {
			t.Fatal(err)
		}
	}
}

func TestEnrichScope_Expand(t *testing.T) {
	adminToken, err := generateToken("admin@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	conductorToken, err := generateToken("conductor@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	superuserToken, err := generateSuperuserToken("testing_account@ministry-mapper.com")
	if err != nil {
		t.Fatal(err)
	}

	listURL := func(expand, fields string) string {
		return "/api/collections/assignments/records?expand=" + expand + "&fields=" + url.QueryEscape(fields) +
			"&filter=" + url.QueryEscape(`user="testuserbeta001"`)
	}

	scenarios := []tests.ApiScenario{
		{
			Name:               "map expand shows the caller's own map and hides the other congregation's",
			Method:             http.MethodGet,
			URL:                listURL("map", "id,expand.map.description"),
			Headers:            map[string]string{"Authorization": conductorToken},
			TestAppFactory:     setupTestApp,
			BeforeTestFunc:     seedCrossAssignments,
			ExpectedStatus:     200,
			ExpectedContent:    []string{`"Blk 100A"`},
			NotExpectedContent: []string{`"Blk 300A"`},
		},
		{
			Name:               "congregation expand shows the caller's own congregation and hides the other",
			Method:             http.MethodGet,
			URL:                listURL("congregation", "id,expand.congregation.name"),
			Headers:            map[string]string{"Authorization": conductorToken},
			TestAppFactory:     setupTestApp,
			BeforeTestFunc:     seedCrossAssignments,
			ExpectedStatus:     200,
			ExpectedContent:    []string{`"Alpha Congregation"`},
			NotExpectedContent: []string{`"Beta Congregation"`},
		},
		{
			Name:               "nested territory expand hides the other congregation's territory",
			Method:             http.MethodGet,
			URL:                listURL("map.territory", "id,expand.map.expand.territory.description"),
			Headers:            map[string]string{"Authorization": conductorToken},
			TestAppFactory:     setupTestApp,
			BeforeTestFunc:     seedCrossAssignments,
			ExpectedStatus:     200,
			ExpectedContent:    []string{`"Alpha Territory 01"`},
			NotExpectedContent: []string{`"Beta Territory 01"`},
		},
		{
			Name:               "user expand hides another congregation's user from a conductor",
			Method:             http.MethodGet,
			URL:                listURL("user", "id,expand.user.email"),
			Headers:            map[string]string{"Authorization": conductorToken},
			TestAppFactory:     setupTestApp,
			BeforeTestFunc:     seedCrossAssignments,
			ExpectedStatus:     200,
			ExpectedContent:    []string{`"crossassignalph"`},
			NotExpectedContent: []string{`"admin@beta.test"`},
		},
		{
			Name:            "an administrator may see users across congregations",
			Method:          http.MethodGet,
			URL:             listURL("user", "id,expand.user.email"),
			Headers:         map[string]string{"Authorization": adminToken},
			TestAppFactory:  setupTestApp,
			BeforeTestFunc:  seedCrossAssignments,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"admin@beta.test"`},
		},
		{
			Name:   "the roster still shows co-members to a conductor",
			Method: http.MethodGet,
			URL: "/api/collections/roles/records?expand=user&fields=" + url.QueryEscape("id,expand.user.email") +
				"&filter=" + url.QueryEscape(`congregation="testcongalpha01"`),
			Headers:         map[string]string{"Authorization": conductorToken},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"admin@alpha.test"`, `"readonly@alpha.test"`},
		},
		{
			Name:   "a publisher's link still expands its own map",
			Method: http.MethodGet,
			URL: "/api/collections/addresses/records?expand=map&fields=" + url.QueryEscape("id,expand.map.description") +
				"&filter=" + url.QueryEscape(`map="testmapalpha01a"`),
			Headers:         map[string]string{"link-id": "testassignalpha01"},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"Blk 100A"`},
		},
		{
			Name:            "superuser expand is not scoped",
			Method:          http.MethodGet,
			URL:             listURL("map", "id,expand.map.description"),
			Headers:         map[string]string{"Authorization": superuserToken},
			TestAppFactory:  setupTestApp,
			BeforeTestFunc:  seedCrossAssignments,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"Blk 100A"`, `"Blk 300A"`},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

// TestEnrichScope_Broadcast saves a record and counts what PocketBase's realtime
// broadcaster actually delivers to each subscriber.
//
// The broadcast sends on unbuffered channels and waits for every send before
// Save returns, so each subscriber needs a reader, and once Save returns every
// message that was going to be delivered already has been.
func TestEnrichScope_Broadcast(t *testing.T) {
	app := setupTestApp(t)
	defer app.Cleanup()

	if _, err := apis.NewRouter(app); err != nil { // binds the realtime broadcast hooks
		t.Fatal(err)
	}

	user := func(collection, email string) *core.Record {
		t.Helper()
		rec, err := app.FindAuthRecordByEmail(collection, email)
		if err != nil {
			t.Fatalf("find %s: %v", email, err)
		}
		return rec
	}
	member := user("users", "conductor@alpha.test")
	alphaReadonly := user("users", "readonly@alpha.test")
	foreign := user("users", "admin@beta.test")
	foreignConductor := user("users", "xcong@beta.test")
	superuser := user(core.CollectionNameSuperusers, "testing_account@ministry-mapper.com")

	type subscriber struct {
		name   string
		auth   *core.Record
		linkId string
		want   bool
	}

	for _, c := range []struct {
		collection, id, field string
		subscribers           []subscriber
	}{
		{"maps", "testmapalpha01a", "description", []subscriber{
			{"member", member, "", true},
			{"other congregation", foreign, "", false},
			{"valid link", nil, "testassignalpha01", true},
			{"expired link", nil, "testassignexprd01", false},
			{"link for another map", nil, "testassignbeta001", false},
			{"superuser", superuser, "", true},
		}},
		{"territories", "testterralpha01", "description", []subscriber{
			{"member", member, "", true},
			{"other congregation", foreign, "", false},
		}},
		{"congregations", "testcongalpha01", "name", []subscriber{
			{"member", member, "", true},
			{"other congregation", foreign, "", false},
		}},
		{"users", "testuseralpha03", "name", []subscriber{
			{"the user", alphaReadonly, "", true},
			{"co-member", member, "", true},
			{"conductor elsewhere", foreignConductor, "", false},
			{"administrator elsewhere", foreign, "", true},
			// A link takes precedence over the token it is sent with.
			{"link for the user's congregation", foreignConductor, "testassignalpha01", true},
			{"link for another congregation", member, "testassignbeta001", false},
		}},
	} {
		t.Run(c.collection, func(t *testing.T) {
			stop := make(chan struct{})
			received := make([]atomic.Int32, len(c.subscribers))

			for i, sub := range c.subscribers {
				client := subscriptions.NewDefaultClient()
				if sub.auth != nil {
					client.Set(apis.RealtimeClientAuthKey, sub.auth)
				}
				topic := c.collection + "/" + c.id
				if sub.linkId != "" {
					topic += "?options=" + url.QueryEscape(`{"headers":{"link-id":"`+sub.linkId+`"}}`)
				}
				client.Subscribe(topic)
				app.SubscriptionsBroker().Register(client)
				defer app.SubscriptionsBroker().Unregister(client.Id())

				go func(ch <-chan subscriptions.Message, n *atomic.Int32) {
					for {
						select {
						case _, ok := <-ch:
							if !ok {
								return
							}
							n.Add(1)
						case <-stop:
							return
						}
					}
				}(client.Channel(), &received[i])
			}
			defer close(stop)

			record, err := app.FindRecordById(c.collection, c.id)
			if err != nil {
				t.Fatal(err)
			}
			record.Set(c.field, "broadcast probe")
			if err := app.Save(record); err != nil {
				t.Fatal(err)
			}

			// Give the readers a moment to count what they took.
			time.Sleep(50 * time.Millisecond)

			for i, sub := range c.subscribers {
				got := received[i].Load()
				if sub.want && got != 1 {
					t.Errorf("%s: expected the event, received %d", sub.name, got)
				}
				if !sub.want && got != 0 {
					t.Errorf("%s: expected no event, received %d", sub.name, got)
				}
			}
		})
	}
}
