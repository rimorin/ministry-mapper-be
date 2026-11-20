//go:build testdata

package setup

import (
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ganigeorgiev.com/pocketbase/dbx"
	"ganigeorgiev.com/pocketbase/pocketbase/apis"
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"ganigeorgiev.com/pocketbase/pocketbase/tests"
	"ganigeorgiev.com/pocketbase/pocketbase/tools/subscriptions"
)

const (
	publisherUserID  = "testuseralpha04"
	publisherEmail   = "publisher@alpha.test"
	publisherLinkID  = "publinkactive01"
	publisherExpired = "publinkexpired1"
	otherAssigneeID  = "publinkother001"
	sharedLinkID     = "publinkshared01"
)

// seedAssignment saves a link on an Alpha map, made by the Alpha conductor.
func seedAssignment(t testing.TB, app core.App, id string, expiry time.Time, assignees ...string) {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("assignments")
	if err != nil {
		t.Fatal(err)
	}
	rec := core.NewRecord(col)
	rec.Id = id
	rec.Set("map", "testmapalpha01a")
	rec.Set("congregation", "testcongalpha01")
	rec.Set("user", "testuseralpha02")
	rec.Set("assignees", assignees)
	rec.Set("type", "normal")
	rec.Set("publisher", "Alpha Publisher")
	rec.Set("expiry_date", expiry.UTC().Format("2006-01-02 15:04:05.000Z"))
	if err := app.SaveNoValidate(rec); err != nil {
		t.Fatal(err)
	}
}

// seedPublisherLinks gives the publisher one open link and one expired, the
// read-only user one open link, and the two of them one open link together.
func seedPublisherLinks(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
	t.Helper()
	seedAssignment(t, app, publisherLinkID, time.Now().Add(6*time.Hour), publisherUserID)
	seedAssignment(t, app, publisherExpired, time.Now().Add(-time.Hour), publisherUserID)
	seedAssignment(t, app, otherAssigneeID, time.Now().Add(6*time.Hour), "testuseralpha03")
	seedAssignment(t, app, sharedLinkID, time.Now().Add(6*time.Hour), publisherUserID, "testuseralpha03")
}

// A publisher holds a role row, and the checks that once read "has any role"
// now mean staff. Each of these was open to any role before the publisher role
// existed, so each is a place a publisher account would otherwise read or
// change a whole congregation.
func TestPublisher_StaffSurfacesAreRefused(t *testing.T) {
	token, err := generateToken(publisherEmail)
	if err != nil {
		t.Fatal(err)
	}

	list := func(collection, filter string) string {
		return "/api/collections/" + collection + "/records?fields=id&filter=" + url.QueryEscape(filter)
	}
	byCongregation := `congregation="testcongalpha01"`
	byMap := `map="testmapalpha01a"`

	for _, c := range []struct {
		name, method, url, body string
		status                  int
	}{
		{"territories list", http.MethodGet, list("territories", byCongregation), "", 403},
		{"territory view", http.MethodGet, "/api/collections/territories/records/testterralpha01", "", 403},
		{"maps list", http.MethodGet, list("maps", byCongregation), "", 403},
		{"map view", http.MethodGet, "/api/collections/maps/records/testmapalpha01a", "", 403},
		{"congregation view", http.MethodGet, "/api/collections/congregations/records/testcongalpha01", "", 403},
		{"options list", http.MethodGet, list("options", byCongregation), "", 403},
		{"roster", http.MethodGet, list("roles", byCongregation), "", 403},
		{"addresses list", http.MethodGet, list("addresses", byMap), "", 403},
		{"address options list", http.MethodGet, list("address_options", byMap), "", 403},
		{"messages list", http.MethodGet, list("messages", byMap), "", 403},
		{"a map's links", http.MethodGet, list("assignments", byMap), "", 403},
		{"another member's links", http.MethodGet, list("assignments", `user="testuseralpha02"`), "", 403},
		{"a link by id, without holding it", http.MethodGet, "/api/collections/assignments/records/testassignalpha01", "", 403},
		{"another user", http.MethodGet, "/api/collections/users/records/testuseralpha01", "", 403},
		{"map addresses route", http.MethodPost, "/map/addresses", `{"map_id":"testmapalpha01a"}`, 401},
		{"address update route", http.MethodPost, "/address/update",
			`{"address_id":"x","map_id":"testmapalpha01a","status":"done"}`, 403},
		{"quicklink", http.MethodPost, "/territory/link",
			`{"territory":"testterralpha01","coordinates":{"lat":1.3,"lng":103.8},"publisher":"Alpha Publisher"}`, 403},
		{"assistant", http.MethodPost, "/assistant/ask",
			`{"congregation":"testcongalpha01","question":"Who has a map out?"}`, 403},
		{"message create", http.MethodPost, "/api/collections/messages/records",
			`{"map":"testmapalpha01a","congregation":"testcongalpha01","message":"hi","type":"publisher"}`, 403},
		{"link create", http.MethodPost, "/api/collections/assignments/records",
			`{"map":"testmapalpha01a","congregation":"testcongalpha01","user":"testuseralpha04","type":"normal","publisher":"Me","expiry_date":"2099-01-01 00:00:00.000Z"}`, 403},
		{"link members change", http.MethodPost, "/assignment/assignees",
			`{"assignment":"testassignalpha01","assignees":["testuseralpha04"]}`, 403},
		{"link delete", http.MethodDelete, "/api/collections/assignments/records/testassignalpha01", "", 403},
		{"role create", http.MethodPost, "/api/collections/roles/records",
			`{"user":"testuseralpha04","congregation":"testcongalpha01","role":"administrator"}`, 403},
		{"own role update", http.MethodPatch, "/api/collections/roles/records/testrolexcng01d", `{"role":"administrator"}`, 403},
	} {
		scenario := tests.ApiScenario{
			Name:           c.name,
			Method:         c.method,
			URL:            c.url,
			Headers:        map[string]string{"Authorization": token, "Content-Type": "application/json"},
			TestAppFactory: setupTestApp,
			ExpectedStatus: c.status,
			// ApiScenario wants some expected content; a refusal is JSON without records.
			ExpectedContent:    []string{`"`},
			NotExpectedContent: []string{`"collectionId"`, `"items"`},
		}
		if c.body != "" {
			scenario.Body = strings.NewReader(c.body)
		}
		scenario.Test(t)
	}
}

func TestPublisher_SeesOwnRoleAndWorksAHeldLink(t *testing.T) {
	token, err := generateToken(publisherEmail)
	if err != nil {
		t.Fatal(err)
	}

	scenarios := []tests.ApiScenario{
		{
			Name:   "own role row is listed, the rest of the roster is not",
			Method: http.MethodGet,
			URL: "/api/collections/roles/records?fields=id,role&filter=" +
				url.QueryEscape(`user="`+publisherUserID+`"`),
			Headers:            map[string]string{"Authorization": token},
			TestAppFactory:     setupTestApp,
			ExpectedStatus:     200,
			ExpectedContent:    []string{`"testrolexcng01d"`, `"publisher"`},
			NotExpectedContent: []string{`"testrolexcng01a"`},
		},
		{
			// The link, not the account, is what opens the map.
			Name:            "a held link opens its map while signed in",
			Method:          http.MethodPost,
			URL:             "/link/map",
			Headers:         map[string]string{"Authorization": token, "link-id": publisherLinkID},
			TestAppFactory:  setupTestApp,
			BeforeTestFunc:  seedPublisherLinks,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"Blk 100A"`},
		},
		{
			Name:   "a held link lists its map's addresses while signed in",
			Method: http.MethodGet,
			URL: "/api/collections/addresses/records?fields=id&filter=" +
				url.QueryEscape(`map="testmapalpha01a"`),
			Headers:         map[string]string{"Authorization": token, "link-id": publisherLinkID},
			TestAppFactory:  setupTestApp,
			BeforeTestFunc:  seedPublisherLinks,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"items"`},
		},
	}
	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestMyAssignments(t *testing.T) {
	publisherToken, err := generateToken(publisherEmail)
	if err != nil {
		t.Fatal(err)
	}
	readonlyToken, err := generateToken("readonly@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	adminToken, err := generateToken("admin@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	conductorToken, err := generateToken("conductor@alpha.test")
	if err != nil {
		t.Fatal(err)
	}

	scenarios := []tests.ApiScenario{
		{
			Name:           "a link its maker put themself on is marked as their own",
			Method:         http.MethodPost,
			URL:            "/assignments/mine",
			Headers:        map[string]string{"Authorization": conductorToken},
			TestAppFactory: setupTestApp,
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, _ *core.ServeEvent) {
				seedAssignment(t, app, "publinkownself1", time.Now().Add(6*time.Hour), "testuseralpha02")
			},
			ExpectedStatus:  200,
			ExpectedContent: []string{`"id":"publinkownself1"`, `"own":true`},
		},
		{
			Name:           "unauthenticated request is rejected",
			Method:         http.MethodPost,
			URL:            "/assignments/mine",
			TestAppFactory: setupTestApp,
			BeforeTestFunc: seedPublisherLinks,
			ExpectedStatus: 401,
			ExpectedContent: []string{
				`"status":401`,
			},
		},
		{
			Name:           "a publisher gets their open links with the names to show",
			Method:         http.MethodPost,
			URL:            "/assignments/mine",
			Headers:        map[string]string{"Authorization": publisherToken},
			TestAppFactory: setupTestApp,
			BeforeTestFunc: seedPublisherLinks,
			ExpectedStatus: 200,
			ExpectedContent: []string{
				`"id":"` + publisherLinkID + `"`,
				`"id":"` + sharedLinkID + `"`,
				`"map_description":"Blk 100A"`,
				`"congregation_name":"Alpha Congregation"`,
				`"assigner":"Alpha Conductor"`,
				// The shared link names its other holder; the one held alone
				// names nobody, and never the caller.
				`"partners":["Alpha ReadOnly"]`,
				`"partners":[]`,
				`"own":false`,
			},
			NotExpectedContent: []string{publisherExpired, otherAssigneeID, `"Alpha Publisher"`, "readonly@alpha.test"},
		},
		{
			Name:               "a staff member gets the links given to them, not the ones they can manage",
			Method:             http.MethodPost,
			URL:                "/assignments/mine",
			Headers:            map[string]string{"Authorization": readonlyToken},
			TestAppFactory:     setupTestApp,
			BeforeTestFunc:     seedPublisherLinks,
			ExpectedStatus:     200,
			ExpectedContent:    []string{`"id":"` + otherAssigneeID + `"`, `"id":"` + sharedLinkID + `"`},
			NotExpectedContent: []string{publisherLinkID, "testassignalpha01"},
		},
		{
			Name:            "nothing assigned is an empty list",
			Method:          http.MethodPost,
			URL:             "/assignments/mine",
			Headers:         map[string]string{"Authorization": adminToken},
			TestAppFactory:  setupTestApp,
			BeforeTestFunc:  seedPublisherLinks,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"assignments":[]`},
		},
	}
	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

// The link form counts each member's open links by listing the congregation's
// links. The rows stay scoped to the maps the caller is staff for.
func TestAssignmentsListByCongregation(t *testing.T) {
	token := func(email string) string {
		t.Helper()
		tok, err := generateToken(email)
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	listURL := "/api/collections/assignments/records?fields=id,assignees&filter=" +
		url.QueryEscape(`congregation="testcongalpha01" && expiry_date > @now`)

	scenarios := []tests.ApiScenario{
		{
			Name:               "a conductor gets the congregation's open links with their assignees",
			Method:             http.MethodGet,
			URL:                listURL,
			Headers:            map[string]string{"Authorization": token("conductor@alpha.test")},
			TestAppFactory:     setupTestApp,
			BeforeTestFunc:     seedPublisherLinks,
			ExpectedStatus:     200,
			ExpectedContent:    []string{sharedLinkID, `"assignees":["` + publisherUserID + `","testuseralpha03"]`},
			NotExpectedContent: []string{publisherExpired},
		},
		{
			Name:               "an administrator elsewhere gets none of them",
			Method:             http.MethodGet,
			URL:                listURL,
			Headers:            map[string]string{"Authorization": token("admin@beta.test")},
			TestAppFactory:     setupTestApp,
			BeforeTestFunc:     seedPublisherLinks,
			ExpectedStatus:     200,
			ExpectedContent:    []string{`"items":[]`},
			NotExpectedContent: []string{sharedLinkID, publisherUserID},
		},
		{
			Name:            "a publisher cannot list them",
			Method:          http.MethodGet,
			URL:             listURL,
			Headers:         map[string]string{"Authorization": token(publisherEmail)},
			TestAppFactory:  setupTestApp,
			BeforeTestFunc:  seedPublisherLinks,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
	}
	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestAssignmentCreate_Assignee(t *testing.T) {
	conductorToken, err := generateToken("conductor@alpha.test")
	if err != nil {
		t.Fatal(err)
	}

	body := func(assignees ...string) *strings.Reader {
		raw, _ := json.Marshal(map[string]any{
			"map": "testmapalpha01a", "congregation": "testcongalpha01", "user": "testuseralpha02",
			"assignees": assignees, "type": "normal", "publisher": "Alpha Publisher",
			"expiry_date": "2099-01-01 00:00:00.000Z",
		})
		return strings.NewReader(string(raw))
	}

	scenarios := []tests.ApiScenario{
		{
			Name:            "a conductor gives a link to several members of the congregation",
			Method:          http.MethodPost,
			URL:             "/api/collections/assignments/records",
			Body:            body(publisherUserID, "testuseralpha03"),
			Headers:         map[string]string{"Authorization": conductorToken, "Content-Type": "application/json"},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"assignees":["` + publisherUserID + `","testuseralpha03"]`},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, _ *http.Response) {
				var emails string
				err := app.DB().NewQuery(`SELECT assignee_emails FROM assignments_log
					WHERE assignees LIKE {:assignee} AND action = 'assigned' LIMIT 1`).
					Bind(dbx.Params{"assignee": "%" + publisherUserID + "%"}).Row(&emails)
				if err != nil {
					t.Fatalf("no log row for the assignees: %v", err)
				}
				if want := publisherEmail + ", readonly@alpha.test"; emails != want {
					t.Errorf("logged assignee emails = %q, want %q", emails, want)
				}
			},
		},
		{
			Name:            "a link cannot be given to someone outside the congregation",
			Method:          http.MethodPost,
			URL:             "/api/collections/assignments/records",
			Body:            body(publisherUserID, "testuserbeta001"),
			Headers:         map[string]string{"Authorization": conductorToken, "Content-Type": "application/json"},
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  400,
			ExpectedContent: []string{`"An assignee is not a member of this congregation."`},
		},
		{
			Name:               "a link without an assignee is made as before",
			Method:             http.MethodPost,
			URL:                "/api/collections/assignments/records",
			Body:               body(),
			Headers:            map[string]string{"Authorization": conductorToken, "Content-Type": "application/json"},
			TestAppFactory:     setupTestApp,
			ExpectedStatus:     200,
			ExpectedContent:    []string{`"assignees":[]`},
			NotExpectedContent: []string{publisherUserID},
		},
	}
	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestUpdateAssignees(t *testing.T) {
	conductorToken, err := generateToken("conductor@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	betaToken, err := generateToken("admin@beta.test")
	if err != nil {
		t.Fatal(err)
	}
	body := func(assignees ...string) *strings.Reader {
		raw, _ := json.Marshal(map[string]any{
			"assignment": publisherLinkID, "assignees": assignees, "publisher": "Alpha ReadOnly",
		})
		return strings.NewReader(string(raw))
	}
	headers := func(token string) map[string]string {
		return map[string]string{"Authorization": token, "Content-Type": "application/json"}
	}
	// What each account's open "My maps" is told, taken off the link or put on it.
	var toldRemoved, toldAdded atomic.Int32
	listen := func(t testing.TB, app *tests.TestApp, userID string, told *atomic.Int32) {
		t.Helper()
		auth, err := app.FindRecordById("users", userID)
		if err != nil {
			t.Fatal(err)
		}
		client := subscriptions.NewDefaultClient()
		client.Set(apis.RealtimeClientAuthKey, auth)
		client.Subscribe("my-maps/*")
		app.SubscriptionsBroker().Register(client)
		go func() {
			for range client.Channel() {
				told.Add(1)
			}
		}()
	}

	scenarios := []tests.ApiScenario{
		{
			Name:           "a conductor swaps who holds an open link",
			Method:         http.MethodPost,
			URL:            "/assignment/assignees",
			Body:           body("testuseralpha03", "testuseralpha03"),
			Headers:        headers(conductorToken),
			TestAppFactory: setupTestApp,
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, e *core.ServeEvent) {
				seedPublisherLinks(t, app, e)
				listen(t, app, publisherUserID, &toldRemoved)
				listen(t, app, "testuseralpha03", &toldAdded)
			},
			ExpectedStatus:  200,
			ExpectedContent: []string{`"assignees":["testuseralpha03"]`, `"publisher":"Alpha ReadOnly"`},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, _ *http.Response) {
				// The update no longer names the account taken off, so only
				// this message tells it; the one put on hears the update.
				for wait := 0; toldRemoved.Load() == 0 && wait < 100; wait++ {
					time.Sleep(10 * time.Millisecond)
				}
				if toldRemoved.Load() != 1 || toldAdded.Load() != 0 {
					t.Errorf("told removed %d times and added %d, want 1 and 0", toldRemoved.Load(), toldAdded.Load())
				}
				link, err := app.FindRecordById("assignments", publisherLinkID)
				if err != nil {
					t.Fatal(err)
				}
				if got := link.GetStringSlice("assignees"); len(got) != 1 || got[0] != "testuseralpha03" {
					t.Errorf("assignees = %v, want only the read-only user", got)
				}
				var logged int
				err = app.DB().NewQuery(`SELECT COUNT(*) FROM assignments_log
					WHERE assignment = {:id} AND action = 'reassigned'`).
					Bind(dbx.Params{"id": publisherLinkID}).Row(&logged)
				if err != nil || logged != 1 {
					t.Errorf("reassigned log rows = %d (%v), want 1", logged, err)
				}
			},
		},
		{
			Name:            "a link cannot be handed to someone outside the congregation",
			Method:          http.MethodPost,
			URL:             "/assignment/assignees",
			Body:            body("testuserbeta001"),
			Headers:         headers(conductorToken),
			TestAppFactory:  setupTestApp,
			BeforeTestFunc:  seedPublisherLinks,
			ExpectedStatus:  400,
			ExpectedContent: []string{`"An assignee is not a member of this congregation."`},
		},
		{
			Name:            "an expired link takes no new members",
			Method:          http.MethodPost,
			URL:             "/assignment/assignees",
			Body:            strings.NewReader(`{"assignment":"` + publisherExpired + `","assignees":["testuseralpha03"]}`),
			Headers:         headers(conductorToken),
			TestAppFactory:  setupTestApp,
			BeforeTestFunc:  seedPublisherLinks,
			ExpectedStatus:  400,
			ExpectedContent: []string{`"This link has expired."`},
		},
		{
			Name:            "staff of another congregation cannot change it",
			Method:          http.MethodPost,
			URL:             "/assignment/assignees",
			Body:            body(),
			Headers:         headers(betaToken),
			TestAppFactory:  setupTestApp,
			BeforeTestFunc:  seedPublisherLinks,
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
		},
	}
	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

// Losing the role in a congregation takes the account off the links it was
// given there; whoever shared a link with them keeps it.
func TestRoleDelete_RemovesFromHeldLinks(t *testing.T) {
	adminToken, err := generateToken("admin@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	(&tests.ApiScenario{
		Name:           "an administrator removes the publisher",
		Method:         http.MethodDelete,
		URL:            "/api/collections/roles/records/testrolexcng01d",
		Headers:        map[string]string{"Authorization": adminToken},
		TestAppFactory: setupTestApp,
		BeforeTestFunc: seedPublisherLinks,
		ExpectedStatus: 204,
		AfterTestFunc: func(t testing.TB, app *tests.TestApp, _ *http.Response) {
			for id, want := range map[string][]string{
				publisherLinkID: {},
				sharedLinkID:    {"testuseralpha03"},
				otherAssigneeID: {"testuseralpha03"},
			} {
				link, err := app.FindRecordById("assignments", id)
				if err != nil {
					t.Fatal(err)
				}
				if got := link.GetStringSlice("assignees"); !slices.Equal(got, want) {
					t.Errorf("link %s assignees = %v, want %v", id, got, want)
				}
			}
		},
	}).Test(t)
}

func TestPublisher_RealtimeSubscriptionIsDropped(t *testing.T) {
	app := setupTestApp(t)
	defer app.Cleanup()

	publisher, err := app.FindAuthRecordByEmail("users", publisherEmail)
	if err != nil {
		t.Fatal(err)
	}
	sub := realtimeSub("addresses", "*", `map="testmapalpha01a"`, "")
	if got := runSubscribeHook(t, app, publisher, []string{sub}); len(got) != 0 {
		t.Errorf("a publisher's account subscribed to a map's addresses: %s", got[0])
	}
}

// An assignment's id is the link token, so who hears about a new link matters
// as much as who can list them. Each subscriber listens to the whole
// collection with a filter that matches every record: the widest subscription
// the collection's rules let a signed-in account make.
func TestEnrichScope_AssignmentsAndRolesBroadcast(t *testing.T) {
	app := setupTestApp(t)
	defer app.Cleanup()

	if _, err := apis.NewRouter(app); err != nil { // binds the realtime broadcast hooks
		t.Fatal(err)
	}
	seedAssignment(t, app, publisherLinkID, time.Now().Add(6*time.Hour), publisherUserID)

	user := func(email string) *core.Record {
		t.Helper()
		rec, err := app.FindAuthRecordByEmail("users", email)
		if err != nil {
			t.Fatalf("find %s: %v", email, err)
		}
		return rec
	}
	publisher := user(publisherEmail)
	conductor := user("conductor@alpha.test")
	readonly := user("readonly@alpha.test")
	foreignAdmin := user("admin@beta.test")

	type subscriber struct {
		name string
		auth *core.Record
		want bool
	}

	for _, c := range []struct {
		collection, id, filter string
		subscribers            []subscriber
	}{
		{"assignments", publisherLinkID, `id!="" || map=""`, []subscriber{
			{"an assignee", publisher, true},
			{"the member who made it", conductor, true},
			{"staff of the congregation", readonly, true},
			{"an administrator elsewhere", foreignAdmin, false},
		}},
		// A link nobody here was given: the publisher must not hear of it.
		{"assignments", "testassignalpha01", `id!="" || map=""`, []subscriber{
			{"a publisher of the congregation", publisher, false},
			{"staff of the congregation", readonly, true},
			{"an administrator elsewhere", foreignAdmin, false},
		}},
		// The filter "My maps" listens with: links the account made or was given.
		// It matches with ~ because ?= on the relation never passes the
		// realtime access check.
		{"assignments", publisherLinkID, `user="` + publisherUserID + `" || assignees ~ "` + publisherUserID + `"`, []subscriber{
			{"an assignee", publisher, true},
			{"an administrator elsewhere", foreignAdmin, false},
		}},
		{"assignments", "testassignalpha01", `user="` + publisherUserID + `" || assignees ~ "` + publisherUserID + `"`, []subscriber{
			{"a publisher of the congregation", publisher, false},
		}},
		{"roles", "testrolexcng01c", `id!="" || user=""`, []subscriber{
			{"the member", readonly, true},
			{"staff of the congregation", conductor, true},
			{"a publisher of the congregation", publisher, false},
			{"an administrator elsewhere", foreignAdmin, false},
		}},
	} {
		t.Run(c.collection+"/"+c.id, func(t *testing.T) {
			stop := make(chan struct{})
			defer close(stop)
			received := make([]atomic.Int32, len(c.subscribers))

			options, _ := json.Marshal(map[string]any{"query": map[string]any{"filter": c.filter, "fields": "id"}})
			topic := c.collection + "/*?options=" + url.QueryEscape(string(options))

			for i, sub := range c.subscribers {
				client := subscriptions.NewDefaultClient()
				client.Set(apis.RealtimeClientAuthKey, sub.auth)
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

			record, err := app.FindRecordById(c.collection, c.id)
			if err != nil {
				t.Fatal(err)
			}
			// Saving unchanged still broadcasts an update.
			if err := app.SaveNoValidate(record); err != nil {
				t.Fatal(err)
			}

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

// Quick Link takes what the assistant's cards add to it: accounts to give the
// link to, and no position when a map is picked ahead of time. The screen's
// own request, one name and a position, is covered in endpoint_territory_test.
func TestQuicklink_AccountsAndNoPosition(t *testing.T) {
	conductorToken, err := generateToken("conductor@alpha.test")
	if err != nil {
		t.Fatal(err)
	}
	headers := map[string]string{"Authorization": conductorToken, "Content-Type": "application/json"}

	scenarios := []tests.ApiScenario{
		{
			Name:   "a link picked without a position goes to the accounts named",
			Method: http.MethodPost,
			URL:    "/territory/link",
			Body: strings.NewReader(`{"territory":"testterralpha01","publisher":"Alpha Publisher, Sis Ong",` +
				`"assignees":["` + publisherUserID + `","` + publisherUserID + `"]}`),
			Headers:         headers,
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  200,
			ExpectedContent: []string{`"linkId"`, `"mapName"`},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, _ *http.Response) {
				link, err := app.FindFirstRecordByFilter("assignments", `publisher = "Alpha Publisher, Sis Ong"`)
				if err != nil {
					t.Fatal(err)
				}
				if got := link.GetStringSlice("assignees"); !slices.Equal(got, []string{publisherUserID}) {
					t.Errorf("assignees = %v, want the publisher once", got)
				}
				if link.GetString("type") != "normal" || link.GetString("user") != "testuseralpha02" {
					t.Errorf("link = %v", link.PublicExport())
				}
			},
		},
		{
			Name:   "a link cannot be given to someone outside the congregation",
			Method: http.MethodPost,
			URL:    "/territory/link",
			Body: strings.NewReader(`{"territory":"testterralpha01","publisher":"Beta Admin",` +
				`"coordinates":{"lat":1.3521,"lng":103.8198},"assignees":["testuserbeta001"]}`),
			Headers:         headers,
			TestAppFactory:  setupTestApp,
			ExpectedStatus:  400,
			ExpectedContent: []string{`"An assignee is not a member of this congregation."`},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, _ *http.Response) {
				if _, err := app.FindFirstRecordByFilter("assignments", `publisher = "Beta Admin"`); err == nil {
					t.Error("a refused quick link still made a link")
				}
			},
		},
	}
	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}
