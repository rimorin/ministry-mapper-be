package jobs

import (
	"strings"
	"testing"
	"time"
)

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

func TestPreviousCalendarMonth_Label(t *testing.T) {
	p := PreviousCalendarMonth()
	if _, err := time.Parse("January 2006", p.Label); err != nil {
		t.Errorf("PreviousCalendarMonth().Label = %q; expected format 'Month YYYY': %v", p.Label, err)
	}
	now := time.Now()
	firstOfThisMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	lastMonth := firstOfThisMonth.AddDate(0, -1, 0)
	if p.Label != lastMonth.Format("January 2006") {
		t.Errorf("PreviousCalendarMonth().Label = %q; want %q", p.Label, lastMonth.Format("January 2006"))
	}
}

func TestPreviousCalendarMonth_Range(t *testing.T) {
	p := PreviousCalendarMonth()
	if !p.Start.Before(p.End) {
		t.Errorf("Start %s must be before End %s", p.Start, p.End)
	}
	if p.IsOnDemand {
		t.Error("PreviousCalendarMonth().IsOnDemand should be false")
	}
	now := time.Now()
	firstOfThisMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	expectedStart := firstOfThisMonth.AddDate(0, -1, 0)
	if !p.Start.Equal(expectedStart) {
		t.Errorf("Start = %s; want %s", p.Start, expectedStart)
	}
	if !p.End.Equal(firstOfThisMonth) {
		t.Errorf("End = %s; want %s (first of current month)", p.End, firstOfThisMonth)
	}
}

func TestRollingDays_Range(t *testing.T) {
	before := time.Now().UTC()
	p := RollingDays(OnDemandReportDays)
	after := time.Now().UTC()

	if !p.IsOnDemand {
		t.Error("RollingDays().IsOnDemand should be true")
	}
	if !p.Start.Before(p.End) {
		t.Errorf("Start %s must be before End %s", p.Start, p.End)
	}
	// Window should span exactly OnDemandReportDays+1 days (days ago through today inclusive, end is exclusive)
	expectedDays := float64(OnDemandReportDays + 1)
	days := p.End.Sub(p.Start).Hours() / 24
	if days != expectedDays {
		t.Errorf("expected window of %.0f days (%d + today), got %.0f", expectedDays, OnDemandReportDays, days)
	}
	// End must be tomorrow (to include today in queries)
	todayStart := time.Date(before.Year(), before.Month(), before.Day(), 0, 0, 0, 0, time.UTC)
	tomorrowStart := todayStart.AddDate(0, 0, 1)
	if p.End.Before(todayStart) || p.End.After(tomorrowStart.AddDate(0, 0, 1)) {
		t.Errorf("End = %s; expected around %s", p.End, tomorrowStart)
	}
	_ = after
}

func TestRollingDays_Label(t *testing.T) {
	p := RollingDays(OnDemandReportDays)
	if p.Label == "" {
		t.Error("RollingDays().Label must not be empty")
	}
	if !strings.Contains(p.Label, " – ") {
		t.Errorf("RollingDays().Label = %q; expected to contain ' – '", p.Label)
	}
}

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
		"VERIFIED FACTS",
		"coverage",
		"needs_attention",
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
	// Only T1 saw visits this period; T2 had none and must not be rendered at all
	data.MonthlyByTerritory = []TerritoryMonthlyActivity{
		{TerritoryCode: "T1", Done: 10, NotHome: 2},
	}

	_, userMsg := BuildPrompt(data)

	// Territories use code, not description
	if !strings.Contains(userMsg, "T1") {
		t.Error("user message should contain territory code 'T1'")
	}
	if strings.Contains(userMsg, "T2") {
		t.Error("user message should NOT contain a territory with no visits this period")
	}
	if strings.Contains(userMsg, "North District") || strings.Contains(userMsg, "South District") {
		t.Error("user message should NOT contain territory descriptions — use codes only")
	}
	// Total and Invalid columns should appear in the activity table
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
	// T3 must have visits this period to appear in the user message
	data.MonthlyByTerritory = []TerritoryMonthlyActivity{
		{TerritoryCode: "T3", Done: 4},
	}

	_, userMsg := BuildPrompt(data)

	// Territories always use Code, never Description
	if !strings.Contains(userMsg, "T3") {
		t.Error("user message should always use territory code")
	}
	if strings.Contains(userMsg, "Some Description") {
		t.Error("user message should NOT use territory description — only code")
	}
}

func TestBuildPrompt_ActivityStatsRendered(t *testing.T) {
	data := minimalSummaryData()
	data.Activity = []ActivityItem{
		{Status: "done", Count: 200},
		{Status: "not_home", Count: 112},
		{Status: "not_done", Count: 80},
	}

	_, userMsg := BuildPrompt(data)

	for _, expected := range []string{"200", "112", "80"} {
		if !strings.Contains(userMsg, expected) {
			t.Errorf("user message should contain status change count %q", expected)
		}
	}
	if !strings.Contains(userMsg, "not visits") {
		t.Error("user message must say status changes are not all visits")
	}
}

// The narrative previously miscounted active territories and quoted total status
// changes as visits. Both figures are now pre-counted and labelled verified facts.
func TestBuildPrompt_VerifiedFactsRendered(t *testing.T) {
	data := minimalSummaryData()
	data.ActiveTerritories = 7
	data.HouseholdsReached = 1183
	data.Visits = 2291

	_, userMsg := BuildPrompt(data)

	for _, expected := range []string{"VERIFIED FACTS", "7", "1183", "2291"} {
		if !strings.Contains(userMsg, expected) {
			t.Errorf("user message should contain verified fact %q", expected)
		}
	}
}

// Re-opened counts are admin detail; the prompt used to send them with a
// "do not mention" rule attached and the narrative mentioned them anyway.
func TestBuildPrompt_OmitsReopenedColumn(t *testing.T) {
	data := minimalSummaryData()
	data.MonthlyByTerritory = []TerritoryMonthlyActivity{
		{TerritoryCode: "T1", Done: 5},
	}

	_, userMsg := BuildPrompt(data)

	if strings.Contains(userMsg, "Re-opened") {
		t.Error("user message must not include a Re-opened column")
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

func TestBuildPrompt_CumulativeStateRendered(t *testing.T) {
	data := minimalSummaryData()
	data.Territories = []TerritoryProgress{
		{Code: "T5", Description: "East", Total: 400, Invalid: 12, Progress: 55, NotDone: 60},
	}
	data.MonthlyByTerritory = []TerritoryMonthlyActivity{
		{TerritoryCode: "T5", Done: 5},
	}

	_, userMsg := BuildPrompt(data)

	// Remaining is the territory's current not_done count, not this period's activity
	for _, expected := range []string{"400", "12", "55%", "60"} {
		if !strings.Contains(userMsg, expected) {
			t.Errorf("user message should render cumulative value %q", expected)
		}
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

	for _, field := range []string{`"coverage"`, `"needs_attention"`} {
		if !strings.Contains(systemMsg, field) {
			t.Errorf("system message missing JSON schema field %s", field)
		}
	}
}

// minimalSummaryData returns a SummaryData with just enough fields populated
// to let BuildPrompt run without panicking.
func minimalSummaryData() SummaryData {
	return SummaryData{
		CongregationName: "Test Congregation",
		Period:           "February 2026",
		Territories:      []TerritoryProgress{},
		Activity:         []ActivityItem{},
	}
}
