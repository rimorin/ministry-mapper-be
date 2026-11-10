//go:build testdata

package assistant

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestAttachChart_KeepsOnlyWhatTheDataCanDraw(t *testing.T) {
	steps := func() []Step {
		return []Step{
			{Tool: "activity", Result: &Result{Columns: []string{"week", "visits", "reached"}, Rows: []map[string]any{
				{"week": "2026-10-26", "visits": 4, "reached": 3}, {"week": "2026-11-02", "visits": 6, "reached": nil},
			}}},
			{Tool: "territory_overview", Result: &Result{Columns: []string{"territory", "description"}, Rows: []map[string]any{
				{"territory": "T01", "description": "North"}, {"territory": "T02", "description": "South"},
			}}},
		}
	}
	for _, c := range []struct {
		name  string
		req   *chartRequest
		drawn bool
	}{
		{"a trend of two series", &chartRequest{Step: 1, Type: "line", X: "week", Y: []string{"visits", "reached"}}, true},
		{"no chart asked", nil, false},
		{"a step that is not there", &chartRequest{Step: 3, Type: "line", X: "week", Y: []string{"visits"}}, false},
		{"an unknown kind", &chartRequest{Step: 1, Type: "pie", X: "week", Y: []string{"visits"}}, false},
		{"a column that is not there", &chartRequest{Step: 1, Type: "line", X: "day", Y: []string{"visits"}}, false},
		{"a column of words", &chartRequest{Step: 2, Type: "bar", X: "territory", Y: []string{"description"}}, false},
		{"too many series", &chartRequest{Step: 1, Type: "line", X: "week", Y: []string{"visits", "reached", "visits", "reached"}}, false},
	} {
		got := steps()
		attachChart(got, c.req)
		if drawn := got[0].Chart != nil || got[1].Chart != nil; drawn != c.drawn {
			t.Errorf("%s: drawn = %v, want %v", c.name, drawn, c.drawn)
		}
	}
}

func TestAsk_PutsTheAskedChartOnItsStep(t *testing.T) {
	answer, _ := json.Marshal(map[string]any{
		"answer": "Visits rose this month.", "answered": true,
		"chart": map[string]any{"step": 1, "type": "line", "x": "month", "y": []string{"visits"}},
	})
	chat := &scriptedChat{t: t, replies: []string{
		toolCalls([2]string{"activity", `{"from": "2026-09-01", "to": "2026-11-06", "group_by": "month", "territory": null, "limit": null}`}),
		answeredWith(string(answer), 200, 20),
	}}
	resp := ask(t, chat, "How are visits trending?")

	if len(resp.Steps) != 1 || !reflect.DeepEqual(resp.Steps[0].Chart, &Chart{Type: "line", X: "month", Y: []string{"visits"}}) {
		t.Fatalf("steps = %+v", resp.Steps)
	}
}
