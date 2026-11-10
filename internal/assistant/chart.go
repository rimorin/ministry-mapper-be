package assistant

import "slices"

// Chart draws one function result. The panel plots X against up to three Y
// columns from that step's own rows, so every figure on it is the data's,
// never the model's.
type Chart struct {
	Type string   `json:"type"` // line or bar
	X    string   `json:"x"`
	Y    []string `json:"y"`
}

// chartRequest is the chart the model asks for, naming the function call by
// its number in the answer.
type chartRequest struct {
	Step int `json:"step"`
	Chart
}

const maxChartSeries = 3

var chartParam = map[string]any{
	"anyOf": []any{
		object(map[string]any{
			"step": map[string]any{"type": "integer", "description": "The function call to chart, by its number in this answer: 1 for the first."},
			"type": enumOf("line for a trend over days, weeks or months; bar to compare territories, maps, weekdays or hours.", "line", "bar"),
			"x":    map[string]any{"type": "string", "description": "The column along the axis: the dates, or the names compared."},
			"y": map[string]any{"type": "array", "items": map[string]any{"type": "string"},
				"description": "One to three columns of numbers to plot."},
		}),
		map[string]any{"type": "null"},
	},
	"description": "A chart of one function result when it shows the answer better than words: a trend, or several " +
		"rows compared. null otherwise. The answer then gives the finding, not every figure.",
}

// attachChart puts the requested chart on its step when the step can draw it:
// a result with at least two rows, x one of its columns, and every y a column
// of numbers (or gaps). A chart the data cannot draw is dropped.
func attachChart(steps []Step, c *chartRequest) {
	if c == nil || c.Step < 1 || c.Step > len(steps) || (c.Type != "line" && c.Type != "bar") ||
		len(c.Y) == 0 || len(c.Y) > maxChartSeries {
		return
	}
	step := &steps[c.Step-1]
	r := step.Result
	if r == nil || len(r.Rows) < 2 || !slices.Contains(r.Columns, c.X) {
		return
	}
	for _, y := range c.Y {
		if y == c.X || !slices.Contains(r.Columns, y) {
			return
		}
		for _, row := range r.Rows {
			switch row[y].(type) {
			case int, int64, float64, nil:
			default:
				return
			}
		}
	}
	step.Chart = &c.Chart
}
