//go:build testdata

package assistant

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"ministry-mapper/internal/llm"
)

// TestLiveReader has the real model read the photos in testdata/photos and
// scores each reading home by home against expected.json. It costs money, so
// it needs the same opt-in as TestLiveEval:
//
//	export $(grep -v '^#' .env | grep OPENAI_API_KEY | xargs)
//	OPENAI_LIVE_TEST=1 go test -tags testdata -run TestLiveReader -v ./internal/assistant/
//
// To add a photo, drop it in testdata/photos and describe it in expected.json:
// the floors with their units in order, and any marks by unit.
func TestLiveReader(t *testing.T) {
	if os.Getenv("OPENAI_LIVE_TEST") == "" {
		t.Skip("set OPENAI_LIVE_TEST=1 to read the test photos with the live model")
	}
	chat := llm.New(Model)
	if chat == nil {
		t.Skip("OPENAI_API_KEY not set")
	}
	raw, err := os.ReadFile("testdata/photos/expected.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases map[string]struct {
		Question string `json:"question"`
		Expect   struct {
			Place  string              `json:"place"`
			Floors map[string][]string `json:"floors"`
			Marks  map[string]string   `json:"marks"`
		} `json:"expect"`
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	var homes, right int
	for name, c := range cases {
		photo, err := os.ReadFile("testdata/photos/" + name)
		if err != nil {
			t.Fatal(err)
		}
		kind := "jpeg"
		if strings.HasSuffix(name, ".png") {
			kind = "png"
		}
		image := "data:image/" + kind + ";base64," + base64.StdEncoding.EncodeToString(photo)
		if !ValidImage(image) {
			t.Fatalf("%s is not a photo the route would accept (%d characters)", name, len(image))
		}
		seen, r, err := readImage(context.Background(), chat, c.Question, image)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		t.Logf("%s: kind %s, place %q, %d homes listed, %d counted, unclear %v, %d input and %d output tokens",
			name, seen.Kind, seen.Place, seen.units(), seen.Counted, seen.Unclear, r.Usage.InputTokens, r.Usage.OutputTokens)
		if !strings.Contains(seen.Place, c.Expect.Place) {
			t.Errorf("%s: place = %q, want it to hold %q", name, seen.Place, c.Expect.Place)
		}
		got, marks := map[string][]string{}, map[string]string{}
		for _, f := range seen.Floors {
			for _, u := range f.Units {
				got[f.Floor] = append(got[f.Floor], u.Unit)
				if u.Mark != "" {
					marks[u.Unit] = u.Mark
				}
			}
		}
		for floor, units := range c.Expect.Floors {
			for i, unit := range units {
				homes++
				if i < len(got[floor]) && got[floor][i] == unit {
					right++
				} else {
					t.Logf("%s: floor %q home %d: want %s, got %v", name, floor, i+1, unit, got[floor])
				}
			}
			if len(got[floor]) > len(units) {
				t.Errorf("%s: floor %q lists %d homes the photo does not have", name, floor, len(got[floor])-len(units))
			}
		}
		for unit, mark := range c.Expect.Marks {
			if marks[unit] == "" {
				t.Errorf("%s: home %s: mark %q was not read", name, unit, mark)
			}
		}
		if extra := len(marks) - len(c.Expect.Marks); extra > 0 {
			t.Errorf("%s: %d marks read that the photo does not have: %v", name, extra, marks)
		}
	}
	t.Logf("%d of %d homes read in the right place", right, homes)
	if right != homes {
		t.Errorf("%d homes were misread or missed", homes-right)
	}
}
