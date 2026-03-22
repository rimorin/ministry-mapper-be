package jobs

import (
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// truncate
// ---------------------------------------------------------------------------

func TestTruncate(t *testing.T) {
	tests := []struct {
		input    string
		n        int
		expected string
	}{
		{"Hello", 10, "Hello"},
		{"Hello", 5, "Hello"},
		{"Hello World", 8, "Hello W…"},
		{"Hello", 1, "…"},
		{"AB", 2, "AB"},
		{"ABC", 2, "A…"},
		{"", 5, ""},
	}

	for _, tc := range tests {
		got := truncate(tc.input, tc.n)
		if got != tc.expected {
			t.Errorf("truncate(%q, %d) = %q; want %q", tc.input, tc.n, got, tc.expected)
		}
	}
}

// ---------------------------------------------------------------------------
// statusLabel
// ---------------------------------------------------------------------------

func TestStatusLabel(t *testing.T) {
	tests := []struct {
		status   string
		contains string // verify the label contains this substring
	}{
		{"done", "done"},
		{"not_done", "not_done"},
		{"not_home", "not_home"},
		{"do_not_call", "do_not_call"},
		{"invalid", "invalid"},
		{"unknown_status", "unknown_status"}, // falls through to default
	}

	for _, tc := range tests {
		got := statusLabel(tc.status)
		if !strings.Contains(got, tc.contains) {
			t.Errorf("statusLabel(%q) = %q; want it to contain %q", tc.status, got, tc.contains)
		}
	}
}

// ---------------------------------------------------------------------------
// previousMonthRange
// ---------------------------------------------------------------------------

func TestPreviousMonthRange_Format(t *testing.T) {
	start, end := previousMonthRange()

	const layout = "2006-01-02"
	startTime, err := time.Parse(layout, start)
	if err != nil {
		t.Fatalf("start %q is not a valid date: %v", start, err)
	}
	endTime, err := time.Parse(layout, end)
	if err != nil {
		t.Fatalf("end %q is not a valid date: %v", end, err)
	}

	if !startTime.Before(endTime) {
		t.Errorf("start %s must be before end %s", start, end)
	}

	// end should be the 1st of the current month
	now := time.Now()
	expectedEnd := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	if !endTime.Equal(expectedEnd) {
		t.Errorf("end = %s; want %s (first of current month)", end, expectedEnd.Format(layout))
	}

	// start should be exactly one month before end
	expectedStart := expectedEnd.AddDate(0, -1, 0)
	if !startTime.Equal(expectedStart) {
		t.Errorf("start = %s; want %s (first of last month)", start, expectedStart.Format(layout))
	}
}

func TestPreviousMonthRange_DifferentMonths(t *testing.T) {
	start, end := previousMonthRange()
	if start == end {
		t.Errorf("start and end must differ, got both %s", start)
	}
	// start must be day 01
	if !strings.HasSuffix(start, "-01") {
		t.Errorf("start %s should end in -01 (first of month)", start)
	}
	if !strings.HasSuffix(end, "-01") {
		t.Errorf("end %s should end in -01 (first of month)", end)
	}
}

// ---------------------------------------------------------------------------
// previousMonthLabel
// ---------------------------------------------------------------------------

func TestPreviousMonthLabel_Format(t *testing.T) {
	label := previousMonthLabel()

	// Should be parseable as "January 2006" format
	if _, err := time.Parse("January 2006", label); err != nil {
		t.Errorf("previousMonthLabel() = %q; expected format 'Month YYYY': %v", label, err)
	}

	// Should refer to the month before the current one
	now := time.Now()
	firstOfThisMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	lastMonth := firstOfThisMonth.AddDate(0, -1, 0)
	expected := lastMonth.Format("January 2006")
	if label != expected {
		t.Errorf("previousMonthLabel() = %q; want %q", label, expected)
	}
}

// ---------------------------------------------------------------------------
// BuildPrompt
// ---------------------------------------------------------------------------

func TestBuildPrompt_SystemMessageContainsDomainContext(t *testing.T) {
	data := minimalSummaryData()
	systemMsg, _ := BuildPrompt(data)

	requiredPhrases := []string{
		"territory",
		"publishers",
		"householder",
		"house-to-house",
		"good news",
		"territory servant",
		"service overseer",
		"not_done",
		"done",
		"not_home",
		"do_not_call",
		"high not home tries",
		"stalled",
		"return visit",
		"Overall%",
		"Invalid",
		"covered_activity",
		"territory_analysis",
		"conclusion",
	}

	for _, phrase := range requiredPhrases {
		if !strings.Contains(systemMsg, phrase) {
			t.Errorf("system message missing expected phrase %q", phrase)
		}
	}
}

func TestBuildPrompt_UserMessageContainsCongregationAndPeriod(t *testing.T) {
	data := minimalSummaryData()
	_, userMsg := BuildPrompt(data)

	for _, expected := range []string{data.CongregationName, data.Period} {
		if !strings.Contains(userMsg, expected) {
			t.Errorf("user message missing expected value %q", expected)
		}
	}
}

func TestBuildPrompt_TerritorySnapshotRendered(t *testing.T) {
	data := minimalSummaryData()
	data.Territories = []TerritoryProgress{
		{
			Id:          "t1",
			Code:        "T1",
			Description: "North District",
			Progress:    75.0,
			Total:       100,
			Done:        75,
			NotDone:     20,
			NotHome:     5,
			DNC:         0,
			Invalid:     0,
			IsComplete:  false,
		},
		{
			Id:          "t2",
			Code:        "T2",
			Description: "South District",
			Progress:    100.0,
			Total:       50,
			Done:        50,
			IsComplete:  true,
		},
	}
	// T1 was active this month; T2 had no activity but is in the inactive list
	data.MonthlyByTerritory = []TerritoryMonthlyActivity{
		{TerritoryCode: "T1", Done: 10, NotHome: 2},
	}
	data.InactiveTerritories = []string{"T2"}

	_, userMsg := BuildPrompt(data)

	// Territories use code, not description
	if !strings.Contains(userMsg, "T1") {
		t.Error("user message should contain territory code 'T1'")
	}
	if !strings.Contains(userMsg, "T2") {
		t.Error("user message should contain territory code 'T2'")
	}
	if strings.Contains(userMsg, "North District") || strings.Contains(userMsg, "South District") {
		t.Error("user message should NOT contain territory descriptions — use codes only")
	}
	if !strings.Contains(userMsg, "complete") {
		t.Error("user message should mark complete territory with 'complete'")
	}
	// Total and Invalid columns should appear in the active-territory table
	if !strings.Contains(userMsg, "Total") {
		t.Error("user message should include Total column in territory table")
	}
	if !strings.Contains(userMsg, "Invalid") {
		t.Error("user message should include Invalid column in territory table")
	}
}

func TestBuildPrompt_TerritoryUsesCodeWhenDescriptionEmpty(t *testing.T) {
	data := minimalSummaryData()
	data.Territories = []TerritoryProgress{
		{Code: "T3", Description: "Some Description", Progress: 50, NotDone: 10},
	}
	// Put T3 in inactive list so it appears in the user message
	data.InactiveTerritories = []string{"T3"}

	_, userMsg := BuildPrompt(data)

	// Territories always use Code, never Description
	if !strings.Contains(userMsg, "T3") {
		t.Error("user message should always use territory code")
	}
	if strings.Contains(userMsg, "Some Description") {
		t.Error("user message should NOT use territory description — only code")
	}
}

func TestBuildPrompt_InactiveTerritoresListed(t *testing.T) {
	data := minimalSummaryData()
	data.InactiveTerritories = []string{"T4", "T7"}

	_, userMsg := BuildPrompt(data)

	if !strings.Contains(userMsg, "T4") || !strings.Contains(userMsg, "T7") {
		t.Error("user message should list inactive territory codes")
	}
}

func TestBuildPrompt_NoInactiveTerritoriesMessage(t *testing.T) {
	data := minimalSummaryData()
	data.InactiveTerritories = nil

	_, userMsg := BuildPrompt(data)

	if !strings.Contains(userMsg, "none") {
		t.Error("user message should state 'none' when there are no inactive territories")
	}
}

func TestBuildPrompt_ActivityStatsRendered(t *testing.T) {
	data := minimalSummaryData()
	data.TotalChanges = 312
	data.Activity = []ActivityItem{
		{Status: "done", Count: 200, Pct: 64.1},
		{Status: "not_home", Count: 112, Pct: 35.9},
	}
	data.PeakDay = "Feb 15 (47 changes)"

	_, userMsg := BuildPrompt(data)

	if !strings.Contains(userMsg, "312") {
		t.Error("user message should contain total change count '312'")
	}
	if !strings.Contains(userMsg, "Feb 15 (47 changes)") {
		t.Error("user message should contain peak day label")
	}
}

func TestBuildPrompt_NotHomeFatigueWithElevatedFlag(t *testing.T) {
	data := minimalSummaryData()
	data.NotHomeFatigue = []NotHomeFatigue{
		{TerritoryCode: "T1", MaxedOut: 40, Retrying: 10, MaxedOutPct: 80.0},
	}

	_, userMsg := BuildPrompt(data)

	if !strings.Contains(userMsg, "high") {
		t.Error("user message should flag ≥35%% maxed-out rate as 'high'")
	}
}

func TestBuildPrompt_NotHomeFatigueNoFlag(t *testing.T) {
	data := minimalSummaryData()
	data.NotHomeFatigue = []NotHomeFatigue{
		{TerritoryCode: "T2", MaxedOut: 5, Retrying: 40, MaxedOutPct: 11.1},
	}

	_, userMsg := BuildPrompt(data)

	if strings.Contains(userMsg, "elevated") {
		t.Error("user message should NOT flag <35%% maxed-out rate as 'elevated'")
	}
}

func TestBuildPrompt_StalledMapsListed(t *testing.T) {
	data := minimalSummaryData()
	data.StalledMaps = []MapHealthItem{
		{TerritoryCode: "T1", MapCode: "M3", MapDescription: "Block 412", Progress: 0, NotDone: 45},
	}

	_, userMsg := BuildPrompt(data)

	if !strings.Contains(userMsg, "Block 412") {
		t.Error("user message should list stalled map description 'Block 412'")
	}
	if !strings.Contains(userMsg, "45") {
		t.Error("user message should include unworked address count for stalled map")
	}
}

func TestBuildPrompt_NoStalledMaps(t *testing.T) {
	data := minimalSummaryData()
	data.StalledMaps = nil

	_, userMsg := BuildPrompt(data)

	if !strings.Contains(userMsg, "none") {
		t.Error("user message should say 'none' when there are no stalled maps")
	}
}

func TestBuildPrompt_CompletedMapsListed(t *testing.T) {
	data := minimalSummaryData()
	data.CompletedMaps = []MapHealthItem{
		{TerritoryCode: "T1", MapCode: "MAP-A", MapDescription: "Ang Mo Kio Ave 1", Progress: 100},
		{TerritoryCode: "T2", MapCode: "MAP-B", MapDescription: "Bishan St 22", Progress: 100},
	}

	_, userMsg := BuildPrompt(data)

	if !strings.Contains(userMsg, "Ang Mo Kio Ave 1") || !strings.Contains(userMsg, "Bishan St 22") {
		t.Error("user message should list completed map descriptions")
	}
}

func TestBuildPrompt_HighDNCMapsListed(t *testing.T) {
	data := minimalSummaryData()
	data.HighDNCMaps = []MapHealthItem{
		{TerritoryCode: "T3", MapCode: "DNC-MAP", MapDescription: "Toa Payoh Lor 4", DNC: 18},
	}

	_, userMsg := BuildPrompt(data)

	if !strings.Contains(userMsg, "Toa Payoh Lor 4") {
		t.Error("user message should list high-DNC map description")
	}
	if !strings.Contains(userMsg, "18") {
		t.Error("user message should include DNC count for high-DNC map")
	}
}

func TestBuildPrompt_EstimatedCompletionRendered(t *testing.T) {
	data := minimalSummaryData()
	data.Territories = []TerritoryProgress{
		{
			Code:                "T5",
			Description:         "East",
			NotDone:             60,
			IsComplete:          false,
			EstMonthsToComplete: 3.5,
		},
	}
	// T5 must appear in either MonthlyByTerritory or InactiveTerritories to be rendered
	data.MonthlyByTerritory = []TerritoryMonthlyActivity{
		{TerritoryCode: "T5", Done: 5},
	}

	_, userMsg := BuildPrompt(data)

	if !strings.Contains(userMsg, "3.5") {
		t.Error("user message should render estimated months to completion '3.5'")
	}
}

func TestBuildPrompt_ReturnsBothMessages(t *testing.T) {
	data := minimalSummaryData()
	systemMsg, userMsg := BuildPrompt(data)

	if systemMsg == "" {
		t.Error("BuildPrompt must return a non-empty system message")
	}
	if userMsg == "" {
		t.Error("BuildPrompt must return a non-empty user message")
	}
}

func TestBuildPrompt_JSONSchemaInSystemMessage(t *testing.T) {
	data := minimalSummaryData()
	systemMsg, _ := BuildPrompt(data)

	// Verify the JSON response schema fields are present in the system message
	for _, field := range []string{`"covered_activity"`, `"territory_analysis"`, `"conclusion"`} {
		if !strings.Contains(systemMsg, field) {
			t.Errorf("system message missing JSON schema field %s", field)
		}
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// minimalSummaryData returns a SummaryData with just enough fields populated
// to let BuildPrompt run without panicking.
func minimalSummaryData() SummaryData {
	return SummaryData{
		CongregationName: "Test Congregation",
		Period:           "February 2026",
		Territories:      []TerritoryProgress{},
		Activity:         []ActivityItem{},
		TotalChanges:     0,
	}
}
