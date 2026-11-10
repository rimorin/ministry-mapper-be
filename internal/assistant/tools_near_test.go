//go:build testdata

package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"ganigeorgiev.com/pocketbase/dbx"
	"ganigeorgiev.com/pocketbase/pocketbase/tests"
)

// placeMaps puts Blk 100A at the origin and the other alpha maps at growing
// distances to the north (0.001 degrees of latitude is about 111 metres).
func placeMaps(t *testing.T, app *tests.TestApp) {
	t.Helper()
	for i, id := range []string{"testmapalpha01a", "testmapalpha01b", "testmapalpha02a", "testmapalpha02b"} {
		exec(t, app, `UPDATE maps SET coordinates = {:at} WHERE id = {:id}`, dbx.Params{
			"id": id, "at": fmt.Sprintf(`{"lat":%.4f,"lng":103.8000}`, 1.3000+0.001*float64(i)),
		})
	}
}

func TestMapsNear_FromAPlaceNearestFirst(t *testing.T) {
	app := newTestApp(t)
	placeMaps(t, app)
	s := scopeFor(t, app, alphaCong)

	r := run(t, s, mapsNear, `{"from": "place", "place": "Blk 100A"}`)
	// The seed has other located maps further away; the nearest three are ours.
	got := column(r, "map")
	if len(got) < 3 || got[0] != "Blk 100B" || got[1] != "Blk 200A" || got[2] != "Blk 200B" {
		t.Fatalf("maps = %v, want Blk 100B, Blk 200A, Blk 200B first", got)
	}
	for _, m := range got {
		if m == "Blk 100A" {
			t.Fatal("the place itself must not be listed as near itself")
		}
	}
	if d := r.Rows[0]["distance_m"].(int); d < 100 || d > 120 {
		t.Errorf("distance to the next block = %d m, want about 111", d)
	}
	if _, ok := r.Rows[0]["lat"]; !ok {
		t.Error("rows must carry the location for the panel's map")
	}

	near := run(t, s, mapsNear, `{"from": "place", "place": "Blk 100A", "within_meters": 250}`)
	if len(near.Rows) != 2 {
		t.Errorf("within 250 m = %v, want 2 maps", column(near, "map"))
	}
	free := run(t, s, mapsNear, `{"from": "place", "place": "Blk 200A", "unassigned_only": true}`)
	for _, row := range free.Rows {
		if row["map_id"] == "testmapalpha01a" {
			t.Error("a map with an active link is not free")
		}
	}
}

func TestMapsNear_MyLocationNeedsSharing(t *testing.T) {
	app := newTestApp(t)
	placeMaps(t, app)
	s := scopeFor(t, app, alphaCong)

	_, err := mapsNear.Run(s, json.RawMessage(`{"from": "my_location"}`))
	if !errors.As(err, &needLocationError{}) {
		t.Fatalf("err = %v, want needLocationError", err)
	}

	s.Origin = &Location{Lat: 1.3029, Lng: 103.8}
	r := run(t, s, mapsNear, `{"from": "my_location", "limit": 2}`)
	if got := column(r, "map"); len(got) != 2 || got[0] != "Blk 200B" || r.Total < 4 {
		t.Fatalf("maps = %v (total %d), want Blk 200B first, limited to 2 of at least 4", got, r.Total)
	}
}

func TestMapsNear_UnknownPlace(t *testing.T) {
	app := newTestApp(t)
	_, err := mapsNear.Run(scopeFor(t, app, alphaCong), json.RawMessage(`{"from": "place", "place": "Nowhere 9"}`))
	var bad argError
	if !errors.As(err, &bad) || !strings.Contains(bad.msg, "find_places") {
		t.Fatalf("err = %v, want an argument error pointing at find_places", err)
	}
}

func TestAsk_AsksToShareLocationAndNeverSendsIt(t *testing.T) {
	app := newTestApp(t)
	placeMaps(t, app)

	chat := &scriptedChat{t: t, replies: []string{
		toolCalls([2]string{"maps_near", `{"from":"my_location"}`}),
		final("Please share your location so I can find maps near you.", true),
	}}
	resp, err := Ask(context.Background(), chat, scopeFor(t, app, alphaCong), Request{Question: "What is near me?"})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.NeedsLocation {
		t.Fatal("the response must ask the panel to offer location sharing")
	}

	s := scopeFor(t, app, alphaCong)
	s.Origin = &Location{Lat: 1.30291, Lng: 103.80017}
	chat = &scriptedChat{t: t, replies: []string{
		toolCalls([2]string{"maps_near", `{"from":"my_location"}`}),
		final("Blk 200B is nearest.", true),
	}}
	resp, err = Ask(context.Background(), chat, s, Request{Question: "What is near me?"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.NeedsLocation || strings.Contains(chat.sent(), "1.30291") || strings.Contains(chat.sent(), "103.80017") {
		t.Fatalf("the admin's location reached the model: %s", chat.sent())
	}
}
