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

func TestBuildPrompt_ReviewNeededTerritoryListed(t *testing.T) {
	data := minimalSummaryData()
	data.NotHomeFatigue = []NotHomeFatigue{
		{TerritoryCode: "T1", MaxedOut: 40, Retrying: 10, MaxedOutPct: 80.0, ReviewNeeded: true},
	}

	_, userMsg := BuildPrompt(data)

	if !strings.Contains(userMsg, "[Nobody home after all tries] T1: 40 homes had nobody home on every allowed visit. Decide whether to reset them, mark them invalid, or plan a special visit.") {
		t.Errorf("user message should carry the review action item; got:\n%s", userMsg)
	}
}

func TestBuildPrompt_UnflaggedTerritoryNotListed(t *testing.T) {
	data := minimalSummaryData()
	data.NotHomeFatigue = []NotHomeFatigue{
		{TerritoryCode: "T2", MaxedOut: 5, Retrying: 40, MaxedOutPct: 11.1},
	}

	_, userMsg := BuildPrompt(data)

	if strings.Contains(userMsg, "T2") {
		t.Error("a territory below the review thresholds must not appear as an action item")
	}
	if !strings.Contains(userMsg, "ACTION ITEMS: none") {
		t.Error("user message should say there are no action items")
	}
}

func TestSummaryData_ActionItems(t *testing.T) {
	data := SummaryData{
		NotHomeFatigue: []NotHomeFatigue{
			{TerritoryCode: "A", MaxedOut: 12, MaxedOutPct: 40, ReviewNeeded: true, Stale: 3},
			{TerritoryCode: "B", MaxedOut: 48, MaxedOutPct: 92, ReviewNeeded: true, Stale: 25},
			{TerritoryCode: "C", MaxedOut: 1, MaxedOutPct: 100, Stale: 429},
			{TerritoryCode: "D", MaxedOut: 20, MaxedOutPct: 50, ReviewNeeded: true},
			{TerritoryCode: "E", MaxedOut: 15, MaxedOutPct: 60, ReviewNeeded: true},
		},
		StalledMaps: []MapHealthItem{{TerritoryCode: "W17", MapDescription: "903A North Woodlands Drive", NotDone: 318}},
		HighDNCMaps: []MapHealthItem{{TerritoryCode: "M08", MapDescription: "149, Woodlands Street 13", DNC: 7}},
	}

	items := data.ActionItems()

	got := make([]string, len(items))
	for i, it := range items {
		got[i] = it.Category + " | " + it.Text
	}
	want := []string{
		// review items: most maxed-out first, capped at three (A is dropped)
		"Nobody home after all tries | B: 48 homes had nobody home on every allowed visit. Decide whether to reset them, mark them invalid, or plan a special visit.",
		"Nobody home after all tries | D: 20 homes had nobody home on every allowed visit. Decide whether to reset them, mark them invalid, or plan a special visit.",
		"Nobody home after all tries | E: 15 homes had nobody home on every allowed visit. Decide whether to reset them, mark them invalid, or plan a special visit.",
		// stale: C qualifies on count alone even though it is not flagged for review; A is below the floor
		"Return visits overdue | C: 429 homes are waiting for a return visit and have not been tried for over two weeks.",
		"Return visits overdue | B: 25 homes are waiting for a return visit and have not been tried for over two weeks.",
		"Map not started | W17, map \"903A North Woodlands Drive\": 318 homes, none visited yet.",
		"Many do-not-call homes | M08, map \"149, Woodlands Street 13\": 7 homes have asked not to be called again.",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("action items:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if len(SummaryData{}.ActionItems()) != 0 {
		t.Error("empty data must produce no action items")
	}
}

func TestStripTerritorySuffix(t *testing.T) {
	cases := map[[2]string]string{
		{"26, Marsiling Drive (M04)", "M04"}: "26, Marsiling Drive",
		{"26, Marsiling Drive (M04)", "M05"}: "26, Marsiling Drive (M04)",
		{"Blk 412", "M04"}:                   "Blk 412",
		{"(M04)", ""}:                        "(M04)",
	}
	for in, want := range cases {
		if got := stripTerritorySuffix(in[0], in[1]); got != want {
			t.Errorf("stripTerritorySuffix(%q, %q) = %q; want %q", in[0], in[1], got, want)
		}
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

func TestBuildPrompt_PreviousPeriodRendered(t *testing.T) {
	data := minimalSummaryData()
	data.HasPrevious = true
	data.PreviousPeriod = "January 2026"
	data.HouseholdsReached, data.PrevHouseholdsReached = 142, 98
	data.Visits, data.PrevVisits = 300, 320
	data.ActiveTerritories, data.PrevActiveTerritories = 5, 5
	data.MonthlyByTerritory = []TerritoryMonthlyActivity{{TerritoryCode: "T1", Done: 40, PrevDone: 25}}
	_, userMsg := BuildPrompt(data)

	for _, expected := range []string{
		"PREVIOUS PERIOD (January 2026)",
		"98   change: +44",
		"320   change: -20",
		"5   change: +0",
		"Prev Done",
	} {
		if !strings.Contains(userMsg, expected) {
			t.Errorf("user message missing %q", expected)
		}
	}
	if strings.Contains(userMsg, "nothing to compare against") {
		t.Error("user message says there is nothing to compare when previous figures exist")
	}
}

func TestBuildPrompt_NoPreviousPeriod(t *testing.T) {
	data := minimalSummaryData()
	data.PreviousPeriod = "January 2026"
	_, userMsg := BuildPrompt(data)

	if !strings.Contains(userMsg, "PREVIOUS PERIOD (January 2026): no visits recorded, so there is nothing to compare against.") {
		t.Errorf("user message should say the previous period has nothing to compare; got:\n%s", userMsg)
	}
	if strings.Contains(userMsg, "change:") {
		t.Error("no change values should be printed without a previous period")
	}
}

func TestReportPeriod_Previous(t *testing.T) {
	monthly := PreviousCalendarMonth()
	prev := monthly.previous()
	if !prev.End.Equal(monthly.Start) || !prev.Start.Equal(monthly.Start.AddDate(0, -1, 0)) {
		t.Errorf("monthly previous = %s..%s; want the month before %s", prev.Start, prev.End, monthly.Start)
	}
	if prev.Label != prev.Start.Format("January 2006") {
		t.Errorf("monthly previous label = %q", prev.Label)
	}

	rolling := RollingDays(30)
	prev = rolling.previous()
	if !prev.End.Equal(rolling.Start) || prev.End.Sub(prev.Start) != rolling.End.Sub(rolling.Start) {
		t.Errorf("rolling previous = %s..%s; want an equal-length window ending at %s", prev.Start, prev.End, rolling.Start)
	}
	if !strings.Contains(prev.Label, " – ") {
		t.Errorf("rolling previous label = %q; want a range", prev.Label)
	}
}
