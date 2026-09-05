package jobs

import (
	"bytes"
	"strings"
	"testing"
	"text/template"
)

func renderReportEmail(t *testing.T, summary SummaryData) string {
	t.Helper()
	tmpl, err := template.ParseFiles("../../templates/report.html")
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	err = tmpl.Execute(&body, ReportTemplateData{
		CongregationName: "Alpha", ReportDate: "August 2026", ReportTitle: "Monthly Report", Summary: summary,
	})
	if err != nil {
		t.Fatal(err)
	}
	return body.String()
}

// The figures come from the database, so they must render whether or not the
// AI narrative is available.
func TestReportEmail_StatsRenderWithoutNarrative(t *testing.T) {
	body := renderReportEmail(t, SummaryData{
		Available: false,
		Visits:    6,
		Activity:  []ActivityItem{{Status: "done", Count: 2}, {Status: "not_home", Count: 4}},
	})
	for _, want := range []string{">6<", "Visits", ">2<", "Reached", ">4<", "Nobody home", "attached"} {
		if !strings.Contains(body, want) {
			t.Errorf("email missing %q", want)
		}
	}
	if strings.Contains(body, "This period at a glance") {
		t.Error("narrative block rendered without an AI summary")
	}
}

func TestReportEmail_NarrativeRendersWhenAvailable(t *testing.T) {
	body := renderReportEmail(t, SummaryData{
		Available: true, Coverage: "Publishers reached 142 households.", NeedsAttention: "Nothing to act on.",
		Visits: 6, Activity: []ActivityItem{{Status: "done", Count: 2}},
	})
	for _, want := range []string{"This period at a glance", "What was done", "What needs your attention", "Publishers reached 142 households.", "Nothing to act on.", "Visits"} {
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
