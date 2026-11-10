//go:build testdata

package assistant

import (
	"strings"
	"testing"
)

func TestResolveView(t *testing.T) {
	s := scopeFor(t, newTestApp(t), alphaCong)

	v := ResolveView(s, "testterralpha01", "testmapalpha01a")
	if v == nil || v.TerritoryCode != "T01" || v.TerritoryName != "Alpha Territory 01" || v.MapName != "Blk 100A" {
		t.Fatalf("view = %+v", v)
	}
	prompt := viewPrompt(v)
	for _, want := range []string{"territory T01 (Alpha Territory 01) open", "scrolled to the map Blk 100A", `"this map"`} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt lacks %q: %s", want, prompt)
		}
	}

	// A map from another territory is left out; the territory still counts.
	if v := ResolveView(s, "testterralpha01", "testmapalpha02a"); v == nil || v.MapName != "" {
		t.Fatalf("map from another territory = %+v", v)
	}
	if strings.Contains(viewPrompt(ResolveView(s, "testterralpha01", "")), "this map") {
		t.Error("without a map in view, the prompt must not define this map")
	}

	// Another congregation's territory, or none, is no view at all.
	if v := ResolveView(s, "testterrbeta001", "testmapbeta001a"); v != nil {
		t.Fatalf("another congregation's territory = %+v", v)
	}
	if ResolveView(s, "", "testmapalpha01a") != nil || viewPrompt(nil) != "" {
		t.Fatal("no territory must mean no view")
	}
}

func TestSystemPrompt_IncludesTheView(t *testing.T) {
	s := scopeFor(t, newTestApp(t), alphaCong)
	s.View = ResolveView(s, "testterralpha01", "testmapalpha01a")
	if p := systemPrompt(s); !strings.Contains(p, "VIEW: The admin has territory T01") {
		t.Fatalf("prompt = %s", p)
	}
	s.View = nil
	if p := systemPrompt(s); strings.Contains(p, "VIEW:") {
		t.Fatal("no view, no VIEW section")
	}
}
