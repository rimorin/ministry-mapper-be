package assistant

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// Strict function calling only guarantees well-formed arguments when every
// property is required and extras are forbidden; optional filters are nullable.
func TestTools_SchemasAreStrict(t *testing.T) {
	names := map[string]bool{}
	for _, tool := range Tools {
		if names[tool.Name] {
			t.Errorf("duplicate tool name %q", tool.Name)
		}
		names[tool.Name] = true
		if !regexp.MustCompile(`^[a-z_]{1,64}$`).MatchString(tool.Name) {
			t.Errorf("%s: name must be lower snake case", tool.Name)
		}
		if strings.TrimSpace(tool.Description) == "" {
			t.Errorf("%s: description is empty", tool.Name)
		}
		if tool.Params["additionalProperties"] != false {
			t.Errorf("%s: additionalProperties must be false", tool.Name)
		}
		props := tool.Params["properties"].(map[string]any)
		required := tool.Params["required"].([]string)
		if len(required) != len(props) {
			t.Errorf("%s: %d required for %d properties", tool.Name, len(required), len(props))
		}
		for name := range props {
			if !slices.Contains(required, name) {
				t.Errorf("%s: property %q must be required (make it nullable instead)", tool.Name, name)
			}
		}
	}
}

func TestPseudonyms_StableLettersOnlyAndRestorable(t *testing.T) {
	p := newPseudonyms("cong1")
	token := p.token("Bro Tan")
	if token != p.token(" bro tan ") {
		t.Errorf("the same person must get the same token regardless of case and spacing")
	}
	if token == newPseudonyms("cong2").token("Bro Tan") {
		t.Errorf("tokens must differ between congregations")
	}
	if !regexp.MustCompile(`^Person [A-Z]{8}$`).MatchString(token) {
		t.Fatalf("token %q must be letters only", token)
	}

	text := "Yes, " + token + " came out. Person ZZZZZZZZ did not."
	if got := p.restore(text); got != "Yes, Bro Tan came out. Person ZZZZZZZZ did not." {
		t.Errorf("restore = %q", got)
	}
	if used := p.used(text); len(used) != 1 || used[token] != "Bro Tan" {
		t.Errorf("used = %v", used)
	}
}

func TestModelView_HidesIdsAndMasksPeople(t *testing.T) {
	r := Result{
		Columns: []string{"name", "map"},
		Rows:    []map[string]any{{"name": "Sis Lim", "map": "Blk 100A", "map_id": "secretmapid0001"}},
		Total:   1,
	}
	people := newPseudonyms("cong1")
	view := modelView(r, Tool{PersonColumns: []string{"name"}}, nil, people)
	if strings.Contains(view, "Sis Lim") || strings.Contains(view, "secretmapid0001") {
		t.Fatalf("model view leaks a name or an id: %s", view)
	}
	var parsed struct {
		Rows [][]any `json:"rows"`
	}
	if err := json.Unmarshal([]byte(view), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Rows[0][0] != people.token("Sis Lim") || parsed.Rows[0][1] != "Blk 100A" {
		t.Errorf("rows = %v", parsed.Rows)
	}
}

func TestModelView_MasksKnownNamesInsideText(t *testing.T) {
	r := Result{
		Columns: []string{"by", "text"},
		Rows:    []map[string]any{{"by": "Sis Lim", "text": "Called back, bro tan will return on Friday. Mdm Wong asked."}},
	}
	people := newPseudonyms("cong1")
	tool := Tool{PersonColumns: []string{"by"}, TextColumns: []string{"text"}}
	view := modelView(r, tool, newNameMask([]string{"Bro Tan", "Sis Lim"}), people)
	if strings.Contains(strings.ToLower(view), "bro tan") || strings.Contains(view, "Sis Lim") {
		t.Fatalf("model view leaks a known name: %s", view)
	}
	// A name the records do not hold cannot be recognised and stays.
	if !strings.Contains(view, people.token("Bro Tan")+" will return") || !strings.Contains(view, "Mdm Wong") {
		t.Errorf("view = %s", view)
	}
}

func TestNaturalLess(t *testing.T) {
	codes := []string{"T10", "t2", "T1", "A", "T02a"}
	slices.SortFunc(codes, func(a, b string) int {
		if naturalLess(a, b) {
			return -1
		}
		if naturalLess(b, a) {
			return 1
		}
		return 0
	})
	if got := strings.Join(codes, ","); got != "A,T1,t2,T02a,T10" {
		t.Errorf("sorted = %s", got)
	}
}

// Two names sharing a token would put one person's name on the other's work.
func TestPseudonyms_LargeCongregationSharesNoToken(t *testing.T) {
	p := newPseudonyms("cong1")
	seen := map[string]string{}
	for i := 0; i < 5000; i++ {
		name := fmt.Sprintf("Publisher %d", i)
		token := p.token(name)
		if other, ok := seen[token]; ok {
			t.Fatalf("%q and %q share the token %q", other, name, token)
		}
		seen[token] = name
	}
}
