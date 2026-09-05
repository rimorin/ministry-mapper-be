package jobs

import (
	"strings"
	"testing"
)

// Tests run from the package directory; the binary runs from the repo root.
func init() { templateDir = "../../templates" }

func sampleOverview() OverviewSummary {
	return OverviewSummary{Available: true, Overview: "Two maps report missing units.", Todo: []string{"Blk 149: add units #12-451 to #12-457."}}
}

// Every email renders through the shared layout with realistic data, carries its
// preheader, and yields a plain-text part without markup.
func TestRenderEmail_AllTemplates(t *testing.T) {
	chrome := emailChrome{Preheader: "Preview line.", Kicker: "Alpha", Title: "Sample title", Subtitle: "Sub", ButtonLabel: "Open", ButtonURL: "https://example.test/app", Footer: "Footer line."}
	messages := []messagesData{{Publisher: "Bro Lim", Message: "Unit 07-03 is missing.", Date: "09:42 AM, 05 Sep 2026", MapName: "149, Woodlands Street 13"}}

	cases := map[string]struct {
		data any
		want []string
	}{
		"report.html": {ReportTemplateData{emailChrome: chrome, FileName: "WN_2026.xlsx", ReportDate: "5 Aug – 4 Sep 2026", Summary: SummaryData{
			Available: true, Coverage: "339 homes were reached.", NeedsAttention: "W03 has 223 homes with nobody home.", Visits: 612,
			Activity:    []ActivityItem{{Status: "done", Count: 339}, {Status: "not_home", Count: 251}},
			StalledMaps: []MapHealthItem{{TerritoryCode: "W17", MapDescription: "903A North Woodlands Drive", NotDone: 318}},
		}}, []string{"Visits", ">612<", "Reached", ">339<", "What was done", "What needs your attention", "To do", "Map not started", "WN_2026.xlsx"}},
		"notes.html":                            {NotesTemplateData{emailChrome: chrome, Notes: []notesData{{Publisher: "Sis Tan", Message: "Large dog.", Date: "02 Aug", Address: "Blk 412 #05-12"}}, Summary: OverviewSummary{Available: true, Overview: "One note about a dog."}}, []string{"In short", "One note about a dog.", "Blk 412 #05-12", "Large dog.", "Sis Tan"}},
		"messages.html":                         {EmailTemplateData{emailChrome: chrome, Messages: messages, Summary: sampleOverview()}, []string{"In short", "To do", "Blk 149: add units", "149, Woodlands Street 13", "Unit 07-03 is missing."}},
		"instructions.html":                     {EmailTemplateData{emailChrome: chrome, Messages: messages, MapName: "T01 - Blk 412"}, []string{"Instructions", "Unit 07-03 is missing.", "Bro Lim"}},
		"new_addresses.html":                    {NewAddressesTemplateData{emailChrome: chrome, Count: 2, Maps: []newAddressMapGroup{{MapName: "Blk 412", Territory: "T01", Entries: []newAddressEntry{{Display: "#05 - 12", Date: "09:00 AM", CreatedBy: "Sis Tan", StatusLabel: "Done", StatusColor: "22C55E", Types: []string{"NH"}, Notes: "Corner unit", HasDetails: true}, {Display: "#05 - 13", Date: "09:01 AM"}}}}}, []string{"2 addresses across 1 map", "Blk 412", "T01", "#05 - 12", "Done", "NH", "Corner unit", "#05 - 13"}},
		"user_inactive_warning.html":            {inactiveUserTmplData{emailChrome: chrome, UserName: "Ana", LastLogin: "1 May 2026", DeadlineDate: "1 Dec 2026", DaysLeft: 88}, []string{"Hello Ana", "1 May 2026", "1 Dec 2026", "about 88"}},
		"user_inactive_final_warning.html":      {inactiveUserTmplData{emailChrome: chrome, UserName: "Ana", LastLogin: "1 May 2026", DeadlineDate: "1 Dec 2026", DaysLeft: 30}, []string{"last reminder", "1 Dec 2026", "about 30"}},
		"user_unprovisioned_warning.html":       {unprovisionedUserTmplData{emailChrome: chrome, UserName: "Ana", DaysRemaining: 4}, []string{"Hello Ana", "4 days", "What to do", "congregation administrator"}},
		"user_unprovisioned_final_warning.html": {unprovisionedUserTmplData{emailChrome: chrome, UserName: "Ana", DaysRemaining: 1}, []string{"last reminder", "tomorrow", "What to do today"}},
		"user_unprovisioned_admin_alert.html":   {unprovisionedAdminAlertTmplData{emailChrome: chrome, NewUsers: []unprovisionedNewUser{{Name: "New Person", Email: "new@example.test", Created: "5 Sep 2026 09:00 UTC"}}}, []string{"1 new account has", "New Person", "new@example.test", "What to do"}},
	}

	for name, tc := range cases {
		html, text, err := renderEmail(name, tc.data)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		for _, want := range append(tc.want, "Preview line.", "Sample title", "Footer line.", `href="https://example.test/app"`, "Ministry Mapper") {
			if !strings.Contains(html, want) {
				t.Errorf("%s: html missing %q", name, want)
			}
		}
		if strings.Contains(html, "{{") {
			t.Errorf("%s: unrendered template action in html", name)
		}
		for _, want := range []string{"Sample title", "Footer line.", "https://example.test/app"} {
			if !strings.Contains(text, want) {
				t.Errorf("%s: text missing %q", name, want)
			}
		}
		for _, banned := range []string{"<", "Preview line.", "em-muted", "@media"} {
			if strings.Contains(text, banned) {
				t.Errorf("%s: text part contains %q", name, banned)
			}
		}
	}
}

func TestHtmlToText(t *testing.T) {
	src := `<html><head><style>p{color:red}</style><title>T</title></head><body>
<div style="display:none;max-height:0">hidden preheader</div>
<table><tr><td>Last login</td><td>1 May 2026</td></tr></table>
<p>Read <a href="https://example.test/x">the guide</a> today.</p>
<p>Line one<br>line two</p></body></html>`
	got := htmlToText(src)
	for _, want := range []string{"Last login  1 May 2026", "Read the guide (https://example.test/x) today.", "Line one\nline two"} {
		if !strings.Contains(got, want) {
			t.Errorf("text missing %q in:\n%s", want, got)
		}
	}
	for _, banned := range []string{"hidden preheader", "color:red", "<", "T\n"} {
		if strings.Contains(got, banned) {
			t.Errorf("text contains %q in:\n%s", banned, got)
		}
	}
}
