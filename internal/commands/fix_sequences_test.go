package commands

import (
	"strings"
	"testing"
)

// rowsOf builds the (code, sequence, rowcount) grouping planColumns consumes.
// One entry per code, each present on the same number of floors.
func rowsOf(floors int, pairs ...any) []codeSequence {
	rows := make([]codeSequence, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		rows = append(rows, codeSequence{
			Code:     pairs[i].(string),
			Sequence: pairs[i+1].(int),
			N:        floors,
		})
	}
	return rows
}

func TestNaturalLess(t *testing.T) {
	ordered := []struct{ a, b string }{
		{"9", "10"},        // numeric, not lexical
		{"10", "10A"},      // bare number before its suffixed neighbour
		{"10A", "10B"},     // suffix order
		{"2A", "2B"},       // ditto with a leading digit
		{"05", "7"},        // leading zeros ignored
		{"4301", "4303"},   // the reported case
		{"20A", "20B"},     // Tiong Poh style
		{"1-9", "1-10"},    // hyphenated flats compare numerically
		{"E-2-1", "E-2-2"}, // block-prefixed flats
		{"K1", "K2"},
	}

	for _, c := range ordered {
		if !naturalLess(c.a, c.b) {
			t.Errorf("naturalLess(%q, %q) = false, want true", c.a, c.b)
		}
		if naturalLess(c.b, c.a) {
			t.Errorf("naturalLess(%q, %q) = true, want false", c.b, c.a)
		}
	}

	if naturalLess("10", "10") {
		t.Error("naturalLess should be false for equal codes")
	}
}

func TestDetectDirection(t *testing.T) {
	cases := []struct {
		name           string
		order          []string
		seq            map[string]int
		wantDescending bool
		wantUnclear    bool
	}{
		{
			name:  "ascending corridor",
			order: []string{"4299", "4301", "4303", "4305"},
			seq:   map[string]int{"4299": 0, "4301": 1, "4303": 2, "4305": 3},
		},
		{
			// 625 Ang Mo Kio Avenue 9 — a legitimate high-to-low corridor.
			name:           "descending corridor",
			order:          []string{"114", "112", "110", "108"},
			seq:            map[string]int{"114": 0, "112": 1, "110": 2, "108": 3},
			wantDescending: true,
		},
		{
			name:        "hand ordered walking route",
			order:       []string{"362", "363", "364", "354", "361", "356"},
			seq:         map[string]int{"362": 0, "363": 1, "364": 2, "354": 3, "361": 4, "356": 5},
			wantUnclear: true,
		},
		{
			// Tied pairs are exactly what is undefined, so they must not vote.
			name:           "tied pair does not sway the vote",
			order:          []string{"20", "18", "16", "9", "14"},
			seq:            map[string]int{"20": 0, "18": 1, "16": 2, "9": 3, "14": 3},
			wantDescending: true,
		},
		{
			name:        "single column has no direction",
			order:       []string{"1"},
			seq:         map[string]int{"1": 0},
			wantUnclear: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			descending, unclear := detectDirection(c.order, c.seq)
			if descending != c.wantDescending {
				t.Errorf("descending = %v, want %v", descending, c.wantDescending)
			}
			if unclear != c.wantUnclear {
				t.Errorf("unclear = %v, want %v", unclear, c.wantUnclear)
			}
		})
	}
}

// The reported defect: Yishun 804, where 4303 was added four months late and
// inherited 4301's sequence.
func TestPlanColumnsResolvesReportedCollision(t *testing.T) {
	plan := planColumns(rowsOf(13,
		"4299", 0, "4301", 1, "4303", 1, "4305", 2,
		"4307", 3, "4309", 4, "4311", 5, "4321", 10,
	))

	want := "4299 4301 4303 4305 4307 4309 4311 4321"
	if got := strings.Join(plan.Order, " "); got != want {
		t.Fatalf("column order = %q, want %q", got, want)
	}

	for i, code := range plan.Order {
		if plan.NewSeq[code] != i {
			t.Errorf("NewSeq[%s] = %d, want %d", code, plan.NewSeq[code], i)
		}
	}

	if len(plan.Collisions) != 1 {
		t.Fatalf("got %d collisions, want 1", len(plan.Collisions))
	}
	if got := strings.Join(plan.Collisions[0], ","); got != "4301,4303" {
		t.Errorf("collision group = %q, want %q", got, "4301,4303")
	}
	if plan.Unclear || len(plan.Drift) > 0 {
		t.Errorf("expected a clean ascending plan, got unclear=%v drift=%v", plan.Unclear, plan.Drift)
	}
}

func TestPlanColumnsRenumberingIsGapless(t *testing.T) {
	plan := planColumns(rowsOf(4, "10", 0, "12", 1, "14", 1, "16", 7, "18", 9))

	seen := map[int]bool{}
	for _, code := range plan.Order {
		seq := plan.NewSeq[code]
		if seq < 0 || seq >= len(plan.Order) {
			t.Fatalf("NewSeq[%s] = %d out of range", code, seq)
		}
		if seen[seq] {
			t.Fatalf("sequence %d assigned twice", seq)
		}
		seen[seq] = true
	}
}

// A collision must not disturb the columns around it.
func TestPlanColumnsKeepsUntiedColumnsInPlace(t *testing.T) {
	plan := planColumns(rowsOf(3, "90", 0, "70", 1, "50", 2, "51", 2, "30", 3))

	want := "90 70 51 50 30"
	if got := strings.Join(plan.Order, " "); got != want {
		t.Fatalf("column order = %q, want %q", got, want)
	}
	if !plan.Descending {
		t.Error("expected the descending layout to be detected")
	}
}

func TestPlanColumnsDescendingTieOrder(t *testing.T) {
	// 179 Yung Sheng Road: odd numbers running high-to-low, 145 added later.
	plan := planColumns(rowsOf(9, "143", 0, "141", 1, "139", 2, "137", 3, "145", 3))

	if !plan.Descending {
		t.Fatal("expected descending")
	}
	if got := strings.Join(plan.Order, " "); got != "143 141 139 145 137" {
		t.Errorf("column order = %q, want %q", got, "143 141 139 145 137")
	}
}

// BLK 215C: code 703 sits at two sequences on every floor, i.e. the map holds
// duplicate unit records. Renumbering cannot pick which to keep.
func TestPlanColumnsFlagsDuplicateUnitRecords(t *testing.T) {
	plan := planColumns([]codeSequence{
		{Code: "701", Sequence: 3, N: 13},
		{Code: "703", Sequence: 4, N: 13},
		{Code: "703", Sequence: 5, N: 13},
		{Code: "705", Sequence: 6, N: 13},
	})

	if !plan.blocked() {
		t.Fatal("expected the plan to be blocked")
	}
	if got := strings.Join(plan.Drift, ","); got != "703" {
		t.Errorf("drift = %q, want %q", got, "703")
	}
}

// When a code drifts, the sequence backed by the most rows wins.
func TestPlanColumnsPrefersTheMajoritySequence(t *testing.T) {
	plan := planColumns([]codeSequence{
		{Code: "10", Sequence: 0, N: 12},
		{Code: "12", Sequence: 1, N: 2},
		{Code: "12", Sequence: 9, N: 10},
		{Code: "14", Sequence: 2, N: 12},
	})

	if plan.OldSeq["12"] != 9 {
		t.Errorf("OldSeq[12] = %d, want 9", plan.OldSeq["12"])
	}
	if got := strings.Join(plan.Order, " "); got != "10 14 12" {
		t.Errorf("column order = %q, want %q", got, "10 14 12")
	}
}

func TestPlanColumnsCleanMapNeedsNoChange(t *testing.T) {
	plan := planColumns(rowsOf(5, "1", 0, "2", 1, "3", 2))

	if len(plan.Collisions) != 0 {
		t.Errorf("got %d collisions, want 0", len(plan.Collisions))
	}
	if changed := plan.changedCodes(); len(changed) != 0 {
		t.Errorf("got %v changed, want none", changed)
	}
}
