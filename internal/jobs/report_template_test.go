package jobs

import (
	"strings"
	"testing"
)

func renderReportEmail(t *testing.T, summary SummaryData) string {
	t.Helper()
	html, _, err := renderEmail("report.html", ReportTemplateData{
		emailChrome: reportChrome("Alpha", RollingDays(OnDemandReportDays), summary),
		ReportDate:  "August 2026", FileName: "ALPHA.xlsx", Summary: summary,
	})
	if err != nil {
		t.Fatal(err)
	}
	return html
}

// The figures come from the database, so they must render whether or not the
// AI narrative is available.
func TestReportEmail_StatsRenderWithoutNarrative(t *testing.T) {
	body := renderReportEmail(t, SummaryData{
		Available: false,
		Visits:    6,
		Activity:  []ActivityItem{{Status: "done", Count: 2}, {Status: "not_home", Count: 4}},
	})
	for _, want := range []string{">6<", "Visits", ">2<", "Reached", ">4<", "Nobody home", "is attached"} {
		if !strings.Contains(body, want) {
			t.Errorf("email missing %q", want)
		}
	}
	if strings.Contains(body, "What was done") {
		t.Error("narrative block rendered without an AI summary")
	}
}

func TestReportEmail_NarrativeRendersWhenAvailable(t *testing.T) {
	body := renderReportEmail(t, SummaryData{
		Available: true, Coverage: "Publishers reached 142 households.", NeedsAttention: "Nothing to act on.",
		Visits: 6, Activity: []ActivityItem{{Status: "done", Count: 2}},
	})
	for _, want := range []string{"What was done", "What needs your attention", "Publishers reached 142 households.", "Nothing to act on.", "Visits"} {
		if !strings.Contains(body, want) {
			t.Errorf("email missing %q", want)
		}
	}
}

func TestReportEmail_ActionItemsRenderWithoutNarrative(t *testing.T) {
	body := renderReportEmail(t, SummaryData{
		Activity:    []ActivityItem{{Status: "done", Count: 2}},
		StalledMaps: []MapHealthItem{{TerritoryCode: "W17", MapDescription: "<b>903A</b> North Woodlands Drive", NotDone: 318}},
	})
	for _, want := range []string{"To do", "Map not started", "&lt;b&gt;903A&lt;/b&gt; North Woodlands Drive", "318 homes, none visited yet"} {
		if !strings.Contains(body, want) {
			t.Errorf("email missing %q", want)
		}
	}
	if strings.Contains(body, "<b>903A</b>") {
		t.Error("map names must be HTML-escaped in the checklist")
	}
	if strings.Contains(renderReportEmail(t, SummaryData{}), "To do") {
		t.Error("checklist rendered with no action items")
	}
}

func TestReportSubjectAndPreheader(t *testing.T) {
	period := RollingDays(OnDemandReportDays)
	withVisits := SummaryData{Visits: 612, HouseholdsReached: 339, StalledMaps: []MapHealthItem{{TerritoryCode: "W17", NotDone: 1}}}
	if got := reportSubject("Woodlands North", period, withVisits); got != "Woodlands North: 339 homes reached, 1 thing to do" {
		t.Errorf("subject = %q", got)
	}
	if got := reportChrome("Woodlands North", period, withVisits).Preheader; got != "339 homes reached, 1 thing to do." {
		t.Errorf("preheader = %q", got)
	}
	quiet := reportSubject("Woodlands North", period, SummaryData{})
	if !strings.HasPrefix(quiet, "Activity report for Woodlands North, ") {
		t.Errorf("fallback subject = %q", quiet)
	}
	if got := todoPhrase(0); got != "nothing to do" {
		t.Errorf("todoPhrase(0) = %q", got)
	}
}
