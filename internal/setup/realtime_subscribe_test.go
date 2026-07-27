//go:build testdata

package setup

import (
	"encoding/json"
	"net/url"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// realtimeSub builds a subscription string the way the JS SDK does:
// "<collection>/<topic>?options=<urlencoded json>".
func realtimeSub(collection, topic, filter, linkId string) string {
	opts := map[string]any{
		"query":   map[string]any{"filter": filter, "fields": "id"},
		"headers": map[string]any{"link-id": linkId},
	}
	raw, err := json.Marshal(opts)
	if err != nil {
		panic(err)
	}
	return collection + "/" + topic + "?options=" + url.QueryEscape(string(raw))
}

// runSubscribeHook fires the registered OnRealtimeSubscribeRequest chain and
// returns the subscriptions the hook decided to keep.
func runSubscribeHook(t *testing.T, app core.App, auth *core.Record, subs []string) []string {
	t.Helper()
	event := &core.RealtimeSubscribeRequestEvent{
		RequestEvent:  &core.RequestEvent{},
		Subscriptions: subs,
	}
	event.RequestEvent.App = app
	event.RequestEvent.Auth = auth

	err := app.OnRealtimeSubscribeRequest().Trigger(event, func(*core.RealtimeSubscribeRequestEvent) error {
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe hook returned error: %v", err)
	}
	return event.Subscriptions
}

// A realtime subscription string is the SSE channel name: PocketBase broadcasts
// each event under the exact string it stored, and the client's EventSource
// listens under the string it sent. A hook that rewrites the string publishes
// on a channel nobody is listening to, and every event is silently dropped with
// no error on either side.
//
// So the subscribe hook may only keep or drop a subscription — never edit one.
func TestRealtimeSubscribeKeptSubscriptionsAreUnmodified(t *testing.T) {
	app := setupTestApp(t)
	defer app.Cleanup()

	subs := []string{
		realtimeSub("addresses", "*", `map="testmapalpha01a"`, "testassignalpha01"),
		realtimeSub("address_options", "*", `map="testmapalpha01a"`, "testassignalpha01"),
		realtimeSub("messages", "*", `map="testmapalpha01a" && type!="admin"`, "testassignalpha01"),
	}

	got := runSubscribeHook(t, app, nil, subs)

	if len(got) != len(subs) {
		t.Fatalf("kept %d of %d valid subscriptions: %v", len(got), len(subs), got)
	}
	for i, want := range subs {
		if got[i] != want {
			t.Errorf("subscription %d was modified by the hook.\n got: %s\nwant: %s\n"+
				"Rewriting the string changes the SSE channel name and silently "+
				"breaks realtime delivery for every client.", i, got[i], want)
		}
	}
}

func TestRealtimeSubscribeScopeEnforcement(t *testing.T) {
	app := setupTestApp(t)
	defer app.Cleanup()

	tests := []struct {
		name string
		sub  string
		keep bool
	}{
		{
			name: "valid link, own map",
			sub:  realtimeSub("addresses", "*", `map="testmapalpha01a"`, "testassignalpha01"),
			keep: true,
		},
		{
			name: "valid link, another congregation's map",
			sub:  realtimeSub("addresses", "*", `map="testmapbeta001a"`, "testassignalpha01"),
			keep: false,
		},
		{
			name: "expired link",
			sub:  realtimeSub("addresses", "*", `map="testmapalpha01a"`, "testassignexprd01"),
			keep: false,
		},
		{
			name: "no link at all",
			sub:  realtimeSub("addresses", "*", `map="testmapalpha01a"`, ""),
			keep: false,
		},
		{
			name: "no map in filter",
			sub:  realtimeSub("addresses", "*", `status="done"`, "testassignalpha01"),
			keep: false,
		},
		{
			// The injection 6d07c78 closed: names an authorized map, then widens
			// with a tautology. Refused because the filter as a whole can match
			// records outside the link's map.
			name: "tautology widens past the link's map",
			sub:  realtimeSub("addresses", "*", `map="testmapalpha01a" || id!=""`, "testassignalpha01"),
			keep: false,
		},
		{
			name: "tautology on address_options",
			sub:  realtimeSub("address_options", "*", `map="testmapalpha01a" || id!=""`, "testassignalpha01"),
			keep: false,
		},
		{
			// maps is not in protectedCollections, so it is passed through as-is.
			// This is the subscription that kept working while the others were
			// silently broken.
			name: "unprotected collection passes through",
			sub:  realtimeSub("maps", "testmapalpha01a", "", "testassignalpha01"),
			keep: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := runSubscribeHook(t, app, nil, []string{tc.sub})

			if tc.keep {
				if len(got) != 1 {
					t.Fatalf("subscription was dropped; want kept")
				}
				if got[0] != tc.sub {
					t.Errorf("kept subscription was modified.\n got: %s\nwant: %s", got[0], tc.sub)
				}
				return
			}
			if len(got) != 0 {
				t.Errorf("subscription was kept; want dropped: %s", got[0])
			}
		})
	}
}
