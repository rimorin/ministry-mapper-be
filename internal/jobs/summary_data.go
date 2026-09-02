package jobs

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// TerritoryProgress holds the current cumulative status snapshot for a single territory.
type TerritoryProgress struct {
	Id          string
	Code        string
	Description string
	Progress    float64
	Total       int
	Done        int
	NotDone     int
	NotHome     int
	DNC         int
	Invalid     int
	IsComplete  bool
}

// ActivityItem represents the count of a single status in monthly activity.
type ActivityItem struct {
	Status string
	Count  int
}

// TerritoryMonthlyActivity holds what actually happened in a territory during the report month.
type TerritoryMonthlyActivity struct {
	TerritoryCode string
	Done          int
	NotHome       int
	DNC           int
}

// NotHomeFatigue summarises not-home retry state per territory.
type NotHomeFatigue struct {
	TerritoryCode string
	MaxedOut      int
	Retrying      int
	Stale         int     // not-home addresses not retried in >14 days
	MaxedOutPct   float64 // maxed_out / total * 100, pre-computed
}

// MapHealthItem represents a single map for health reporting.
type MapHealthItem struct {
	TerritoryCode  string
	MapCode        string
	MapDescription string // display name; falls back to MapCode if empty
	Progress       float64
	DNC            int
	NotDone        int
}

// LLMResponse holds the parsed JSON output from the AI model.
type LLMResponse struct {
	Coverage       string `json:"coverage"`
	NeedsAttention string `json:"needs_attention"`
}

// SummaryData is the full data payload assembled from analytics views.
// Available is set true only after a successful LLM call populates all narrative fields.
type SummaryData struct {
	Available          bool
	CongregationName   string
	Period             string // "February 2026"
	Territories        []TerritoryProgress
	MonthlyByTerritory []TerritoryMonthlyActivity // per-territory activity for the report month
	Activity           []ActivityItem
	// Pre-counted facts handed to the LLM so it never has to derive a number.
	ActiveTerritories int // territories with at least one visit this period
	HouseholdsReached int // "done" changes this period
	Visits            int // done + not_home + do_not_call this period (excludes resets/invalid)
	NotHomeFatigue    []NotHomeFatigue
	StalledMaps       []MapHealthItem
	CompletedMaps     []MapHealthItem
	HighDNCMaps       []MapHealthItem // top 3 by DNC count
	Coverage          string          // paragraph 1: what was covered this period
	NeedsAttention    string          // paragraph 2: items for the territory servant to act on
}

// OnDemandReportDays is the default rolling window size for on-demand reports.
const OnDemandReportDays = 30

// ReportPeriod defines the time window for an activity report.
// Start is inclusive, End is exclusive (used in SQL: day >= Start AND day < End).
type ReportPeriod struct {
	Start      time.Time
	End        time.Time
	Label      string // human-readable label, e.g. "February 2026" or "26 Feb – 26 Mar 2026"
	fileTag    string // used in the Excel filename, e.g. "03_26" or "20260226_20260326"
	IsOnDemand bool   // true for on-demand reports, false for scheduled monthly reports
}

// PreviousCalendarMonth returns a ReportPeriod covering the previous full calendar month.
// Used by the scheduled monthly report job.
func PreviousCalendarMonth() ReportPeriod {
	rm := reportMonth()
	return ReportPeriod{
		Start:      rm,
		End:        rm.AddDate(0, 1, 0),
		Label:      rm.Format("January 2006"),
		fileTag:    fmt.Sprintf("%s_%s", rm.AddDate(0, 1, 0).Format("01"), rm.AddDate(0, 1, 0).Format("06")),
		IsOnDemand: false,
	}
}

// RollingDays returns a ReportPeriod covering the given number of days up to
// and including today. Used by the on-demand report endpoint.
func RollingDays(days int) ReportPeriod {
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	start := today.AddDate(0, 0, -days)
	end := today.AddDate(0, 0, 1) // exclusive upper bound — includes today
	return ReportPeriod{
		Start:      start,
		End:        end,
		Label:      fmt.Sprintf("%s – %s", start.Format("2 Jan 2006"), today.Format("2 Jan 2006")),
		fileTag:    fmt.Sprintf("%s_%s", start.Format("20060102"), today.Format("20060102")),
		IsOnDemand: true,
	}
}

// reportMonth returns the first day of the previous calendar month in UTC.
func reportMonth() time.Time {
	n := time.Now()
	return time.Date(n.Year(), n.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -1, 0)
}

// queryTerritoryProgress fetches the current territory status snapshot from analytics_territories.
func queryTerritoryProgress(app core.App, congregationId string) ([]TerritoryProgress, error) {
	type row struct {
		Id          string  `db:"id"`
		Code        string  `db:"code"`
		Description string  `db:"description"`
		Progress    float64 `db:"progress"`
		Total       int     `db:"total_addresses"`
		Done        int     `db:"done"`
		NotDone     int     `db:"not_done"`
		NotHome     int     `db:"not_home"`
		DNC         int     `db:"dnc"`
		Invalid     int     `db:"invalid"`
	}
	var rows []row
	err := app.DB().NewQuery(`
		SELECT id, code, description, progress, total_addresses,
		       done, not_done, not_home, dnc, invalid
		FROM analytics_territories
		WHERE congregation = {:congregation}
		ORDER BY code
	`).Bind(dbx.Params{"congregation": congregationId}).All(&rows)
	if err != nil {
		return nil, fmt.Errorf("query territory progress: %w", err)
	}

	result := make([]TerritoryProgress, 0, len(rows))
	for _, r := range rows {
		tp := TerritoryProgress{
			Id:          r.Id,
			Code:        r.Code,
			Description: r.Description,
			Progress:    r.Progress,
			Total:       r.Total,
			Done:        r.Done,
			NotDone:     r.NotDone,
			NotHome:     r.NotHome,
			DNC:         r.DNC,
			Invalid:     r.Invalid,
			IsComplete:  r.Progress >= 100,
		}
		result = append(result, tp)
	}
	return result, nil
}

// queryMonthlyActivity fetches status-change totals for the given period from analytics_daily_status.
// visits counts real household contact only (done + not_home + do_not_call); resets to
// not_done and invalid marks are status changes but not visits.
func queryMonthlyActivity(app core.App, congregationId string, period ReportPeriod) (items []ActivityItem, reached, visits int, err error) {
	type statusRow struct {
		Status string `db:"new_status"`
		Count  int    `db:"total"`
	}
	var statusRows []statusRow
	err = app.DB().NewQuery(`
		SELECT new_status, SUM(change_count) AS total
		FROM analytics_daily_status
		WHERE congregation = {:congregation}
		  AND day >= {:start}
		  AND day <  {:end}
		GROUP BY new_status
		ORDER BY total DESC
	`).Bind(dbx.Params{
		"congregation": congregationId,
		"start":        period.Start.Format("2006-01-02"),
		"end":          period.End.Format("2006-01-02"),
	}).All(&statusRows)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("query monthly activity: %w", err)
	}

	items = make([]ActivityItem, 0, len(statusRows))
	for _, r := range statusRows {
		switch r.Status {
		case "done":
			reached = r.Count
			visits += r.Count
		case "not_home", "do_not_call":
			visits += r.Count
		}
		items = append(items, ActivityItem{Status: r.Status, Count: r.Count})
	}

	return items, reached, visits, nil
}

// queryMonthlyActivityByTerritory breaks down the period's status changes per territory.
// This is the primary "what happened this period" signal for each territory.
func queryMonthlyActivityByTerritory(app core.App, congregationId string, period ReportPeriod) ([]TerritoryMonthlyActivity, error) {
	monthStart := period.Start.Format("2006-01-02")
	monthEnd := period.End.Format("2006-01-02")

	type row struct {
		TerritoryCode string `db:"territory_code"`
		Status        string `db:"new_status"`
		Count         int    `db:"total"`
	}
	var rows []row
	err := app.DB().NewQuery(`
		SELECT t.code AS territory_code, ads.new_status, SUM(ads.change_count) AS total
		FROM analytics_daily_status ads
		JOIN territories t ON t.id = ads.territory
		WHERE ads.congregation = {:congregation}
		  AND ads.day >= {:start}
		  AND ads.day <  {:end}
		GROUP BY ads.territory, ads.new_status
		ORDER BY t.code, ads.new_status
	`).Bind(dbx.Params{
		"congregation": congregationId,
		"start":        monthStart,
		"end":          monthEnd,
	}).All(&rows)
	if err != nil {
		return nil, fmt.Errorf("query monthly activity by territory: %w", err)
	}

	// Aggregate into a map keyed by territory code
	byTerritory := make(map[string]*TerritoryMonthlyActivity)
	for _, r := range rows {
		if _, ok := byTerritory[r.TerritoryCode]; !ok {
			byTerritory[r.TerritoryCode] = &TerritoryMonthlyActivity{TerritoryCode: r.TerritoryCode}
		}
		a := byTerritory[r.TerritoryCode]
		switch r.Status {
		case "done":
			a.Done = r.Count
		case "not_home":
			a.NotHome = r.Count
		case "do_not_call":
			a.DNC = r.Count
		}
	}

	// Territories whose only change was a reset render as a row of zeros, which the
	// narrative then comments on. Rank by Done so the top territories are the first rows.
	result := make([]TerritoryMonthlyActivity, 0, len(byTerritory))
	for _, v := range byTerritory {
		if v.Done+v.NotHome+v.DNC == 0 {
			continue
		}
		result = append(result, *v)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Done != result[j].Done {
			return result[i].Done > result[j].Done
		}
		return result[i].TerritoryCode < result[j].TerritoryCode
	})
	return result, nil
}

// queryNotHomeFatigue fetches not-home retry counts per territory from analytics_not_home.
// Stale counts are addresses where the publisher has not re-attempted in more than 14 days.
func queryNotHomeFatigue(app core.App, congregationId string) ([]NotHomeFatigue, error) {
	type row struct {
		TerritoryCode string `db:"territory_code"`
		MaxedOut      int    `db:"maxed_out"`
		Retrying      int    `db:"retrying"`
		Stale         int    `db:"stale"`
	}
	var rows []row
	err := app.DB().NewQuery(`
		SELECT t.code AS territory_code,
		       SUM(CASE WHEN anh.retry_status = 'maxed_out' THEN 1 ELSE 0 END) AS maxed_out,
		       SUM(CASE WHEN anh.retry_status = 'retrying'  THEN 1 ELSE 0 END) AS retrying,
		       SUM(CASE WHEN JULIANDAY('now') - JULIANDAY(anh.updated) > 14 THEN 1 ELSE 0 END) AS stale
		FROM analytics_not_home anh
		JOIN territories t ON t.id = anh.territory
		WHERE anh.congregation = {:congregation}
		GROUP BY anh.territory
		ORDER BY territory_code
	`).Bind(dbx.Params{"congregation": congregationId}).All(&rows)
	if err != nil {
		return nil, fmt.Errorf("query not-home fatigue: %w", err)
	}

	result := make([]NotHomeFatigue, 0, len(rows))
	for _, r := range rows {
		pct := 0.0
		if r.MaxedOut+r.Retrying > 0 {
			pct = math.Round(float64(r.MaxedOut)/float64(r.MaxedOut+r.Retrying)*1000) / 10
		}
		result = append(result, NotHomeFatigue{
			TerritoryCode: r.TerritoryCode,
			MaxedOut:      r.MaxedOut,
			Retrying:      r.Retrying,
			Stale:         r.Stale,
			MaxedOutPct:   pct,
		})
	}
	return result, nil
}

// queryMapHealth fetches per-map progress from analytics_maps and classifies
// maps as stalled (0%, work remaining), completed (100%), or high DNC (top 3).
func queryMapHealth(app core.App, congregationId string) (stalled, completed, highDNC []MapHealthItem, err error) {
	type row struct {
		TerritoryCode  string  `db:"territory_code"`
		MapCode        string  `db:"map_code"`
		MapDescription string  `db:"map_description"`
		Progress       float64 `db:"progress"`
		DNC            int     `db:"dnc"`
		NotDone        int     `db:"not_done"`
	}
	var rows []row
	err = app.DB().NewQuery(`
		SELECT t.code AS territory_code, m.code AS map_code,
		       COALESCE(NULLIF(m.description, ''), m.code) AS map_description,
		       am.progress,
		       COALESCE(am.dnc, 0)      AS dnc,
		       COALESCE(am.not_done, 0) AS not_done
		FROM analytics_maps am
		JOIN maps m ON m.id = am.id
		JOIN territories t ON t.id = am.territory
		WHERE am.congregation = {:congregation}
		ORDER BY t.code, m.code
	`).Bind(dbx.Params{"congregation": congregationId}).All(&rows)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("query map health: %w", err)
	}

	items := make([]MapHealthItem, len(rows))
	for i, r := range rows {
		items[i] = MapHealthItem{
			TerritoryCode:  r.TerritoryCode,
			MapCode:        r.MapCode,
			MapDescription: r.MapDescription,
			Progress:       r.Progress,
			DNC:            r.DNC,
			NotDone:        r.NotDone,
		}
		if r.Progress == 0 && r.NotDone > 0 {
			stalled = append(stalled, items[i])
		}
		if r.Progress >= 100 {
			completed = append(completed, items[i])
		}
	}

	// Top 3 maps by DNC count (minimum 1 DNC to be relevant)
	byDNC := append([]MapHealthItem(nil), items...)
	sort.SliceStable(byDNC, func(i, j int) bool { return byDNC[i].DNC > byDNC[j].DNC })
	for i, m := range byDNC {
		if i >= 3 || m.DNC == 0 {
			break
		}
		highDNC = append(highDNC, m)
	}

	return stalled, completed, highDNC, nil
}

// BuildSummaryData queries the analytics views and assembles a SummaryData struct
// ready for prompt building. The Available field is left false until the LLM call succeeds.
func BuildSummaryData(app core.App, congregation *core.Record, period ReportPeriod) (SummaryData, error) {
	cid := congregation.Id
	name, _ := congregation.Get("name").(string)

	activity, reached, visits, err := queryMonthlyActivity(app, cid, period)
	if err != nil {
		return SummaryData{}, err
	}

	monthlyByTerritory, err := queryMonthlyActivityByTerritory(app, cid, period)
	if err != nil {
		return SummaryData{}, err
	}

	territories, err := queryTerritoryProgress(app, cid)
	if err != nil {
		return SummaryData{}, err
	}

	fatigue, err := queryNotHomeFatigue(app, cid)
	if err != nil {
		return SummaryData{}, err
	}

	stalled, completed, highDNC, err := queryMapHealth(app, cid)
	if err != nil {
		return SummaryData{}, err
	}

	return SummaryData{
		Available:          false,
		CongregationName:   name,
		Period:             period.Label,
		Territories:        territories,
		MonthlyByTerritory: monthlyByTerritory,
		Activity:           activity,
		ActiveTerritories:  len(monthlyByTerritory),
		HouseholdsReached:  reached,
		Visits:             visits,
		NotHomeFatigue:     fatigue,
		StalledMaps:        stalled,
		CompletedMaps:      completed,
		HighDNCMaps:        highDNC,
	}, nil
}

// BuildPrompt constructs the system and user messages sent to the LLM.
// Figures the narrative needs are pre-counted in data, and anything that must not
// appear in the narrative is left out of the prompt rather than forbidden by a rule —
// re-opened counts and zero-visit territories were mentioned anyway when forbidden.
func BuildPrompt(data SummaryData) (systemMsg, userMsg string) {
	systemMsg = `You are the territory servant for a Jehovah's Witness congregation, writing the
territory activity report for your service overseer and fellow elders.

MINISTRY CONTEXT:
- Publishers work their assigned territories house-to-house, calling on each household
  to share Bible-based material and offer the good news of God's Kingdom
  (Matthew 24:14; Acts 20:20)
- Each household visit is recorded with a status:
    done         — a householder was home and the publisher could share the good news
    not_home     — no one answered; publisher will make a return visit
    do_not_call  — householder declined further visits (DNC); permanently recorded
    invalid      — address is inaccessible, non-existent, or otherwise unreachable
    not_done     — address is unworked, awaiting a publisher
- A not_home address is retried up to an attempt limit:
    retrying             — within the limit; return visits are planned (normal)
    high not home tries  — limit reached with no contact; the territory servant must
                           decide: reset the address, note it invalid, or organise a
                           special return visit effort
- A stalled map (0% progress with unworked addresses) means those householders have
  not been reached at all — it is likely unassigned or inadvertently overlooked

REPORT READERS:
- Territory servant: assigns maps to publishers and follows up high not-home-tries addresses
- Service overseer: has oversight of overall field ministry health and encourages publishers

YOUR DATA:
- VERIFIED FACTS — already counted for you. Quote these figures exactly. Never
  recount them from the tables below and never adjust them.
- TERRITORY ACTIVITY — one row per territory that saw real visits this period,
  ordered by Done, highest first. Territories with no visits are not listed.
    Done / Not Home / DNC                  = this period's visits only
    Total / Invalid / Overall% / Remaining = cumulative state as of the report date
    Total     = every address in the territory, all statuses combined
    Invalid   = permanently unreachable; counted in Total but can never be completed,
                so a high Invalid lowers the maximum achievable progress
    Overall%  = (done + exhausted not-home) / Total x 100, cumulative across all periods
    Remaining = not_done addresses in the territory right now, not just this period
- NOT-HOME STANDING and MAP HEALTH — current state as of the report date

WRITE EXACTLY TWO PARAGRAPHS:

Paragraph 1 — COVERAGE (2-4 sentences):
  What the congregation accomplished in the house-to-house ministry this period.
  Take the territory count and households reached from VERIFIED FACTS.
  Name the three highest territories from TERRITORY ACTIVITY with their Done figures.
  If a map was completed, name it — that is the clearest result worth reporting.

Paragraph 2 — NEEDS ATTENTION (2-4 sentences):
  Only what the territory servant should act on, drawn from NOT-HOME STANDING and
  MAP HEALTH. Name the territory or map, give the figure, and say plainly what the
  concern is. Cover whichever of these the data shows:
    - territories flagged "review needed" for high not home tries
    - not-home addresses gone stale (not retried in over two weeks)
    - stalled maps sitting at 0% with addresses unworked
    - maps carrying the heaviest DNC concentration
  If there is genuinely nothing to act on, say that in one sentence and stop.

WRITING RULES:
- Every sentence must name a territory, a map, or a figure from the data. Delete any
  sentence that carries no specific fact.
- Do not judge whether the period was good, busy, strong, slow or encouraging, and do
  not compare it with any other period — you have no earlier figures to compare against.
- Do not add general praise or exhortation.
- One idea per sentence. Short sentences, everyday words, no corporate phrasing.
- Call done addresses "reached", using that same word every time.
- A high not-home count is a concern, never a strength — never call one "strong" or "good".
- State the threshold whenever you rely on one, e.g. "above the 35% review level", so
  the reader knows what the flag means.
- Do not recommend which territory to assign next — that is the territory servant's
  decision, not the report's.
- Use only the figures given. Do not invent, infer or recompute anything.
- Name a map by its description as shown, e.g. map "Block 412" in territory M05 —
  never slash notation like "M05/112 (5)".

Respond only in this exact JSON schema:
{
  "coverage": "<paragraph 1>",
  "needs_attention": "<paragraph 2>"
}`

	var sb strings.Builder
	fmt.Fprintf(&sb, "CONGREGATION: %s\nREPORT PERIOD: %s\n\n", data.CongregationName, data.Period)

	// ── Verified facts (current state) ──
	sb.WriteString("VERIFIED FACTS (quote exactly; do not recount):\n")
	fmt.Fprintf(&sb, "  %-37s %d\n", "Territories with visits this period:", data.ActiveTerritories)
	fmt.Fprintf(&sb, "  %-37s %d\n", "Households reached (done):", data.HouseholdsReached)
	fmt.Fprintf(&sb, "  %-37s %d\n", "Total visits (done + not home + DNC):", data.Visits)

	// Build a lookup map: territory code → TerritoryProgress for enriching the activity table
	territoryByCode := make(map[string]TerritoryProgress, len(data.Territories))
	for _, t := range data.Territories {
		territoryByCode[t.Code] = t
	}

	// ── Per-territory activity, ranked by Done (primary analysis signal) ──
	fmt.Fprintf(&sb, "\nTERRITORY ACTIVITY — %s (ordered by Done, highest first):\n", data.Period)
	if len(data.MonthlyByTerritory) > 0 {
		sb.WriteString("Done/Not Home/DNC = this period | Total/Invalid/Overall%/Remaining = cumulative state\n")
		sb.WriteString("Territory | Done | Not Home | DNC | Total | Invalid | Overall% | Remaining\n")
		for _, a := range data.MonthlyByTerritory {
			t := territoryByCode[a.TerritoryCode]
			fmt.Fprintf(&sb, "%-10s| %4d | %8d | %3d | %5d | %7d | %7.0f%% | %9d\n",
				truncate(a.TerritoryCode, 10),
				a.Done, a.NotHome, a.DNC,
				t.Total, t.Invalid, t.Progress, t.NotDone)
		}
	} else {
		sb.WriteString("No visits recorded this period.\n")
	}

	// Kept separate from the visit count above: resets and invalid marks are not visits.
	fmt.Fprintf(&sb, "\nAll status changes for %s (includes resets and invalid marks, which are not visits):\n", data.Period)
	for _, a := range data.Activity {
		fmt.Fprintf(&sb, "  %-24s %4d\n", statusLabel(a.Status)+":", a.Count)
	}

	// ── Not-home standing (current state) ──
	if len(data.NotHomeFatigue) > 0 {
		sb.WriteString("\nNOT-HOME STANDING (current state):\n")
		sb.WriteString("  retrying             = within retry limit; return visits planned (normal)\n")
		sb.WriteString("  high not home tries  = max attempts reached; territory servant must decide next step\n")
		sb.WriteString("  stale (>14 days)     = not-home addresses not retried in over 2 weeks\n")
		sb.WriteString("  flag (≥35% maxed)    = territory servant review needed\n")
		for _, f := range data.NotHomeFatigue {
			flag := ""
			if f.MaxedOutPct >= 35 {
				flag = "  ← review needed (≥35% maxed)"
			}
			fmt.Fprintf(&sb, "  %-10s %d high not home tries (%.0f%%), %d retrying, %d stale (>14 days)%s\n",
				f.TerritoryCode+":", f.MaxedOut, f.MaxedOutPct, f.Retrying, f.Stale, flag)
		}
	}

	// ── Map health (current state) ──
	sb.WriteString("\nMAP HEALTH (current state):\n")
	if len(data.StalledMaps) > 0 {
		sb.WriteString("Stalled maps (0% progress, work remaining):\n")
		for _, m := range data.StalledMaps {
			fmt.Fprintf(&sb, "  territory %s, map \"%s\" — %d addresses unworked\n", m.TerritoryCode, m.MapDescription, m.NotDone)
		}
	} else {
		sb.WriteString("Stalled maps: none\n")
	}
	if len(data.CompletedMaps) > 0 {
		parts := make([]string, len(data.CompletedMaps))
		for i, m := range data.CompletedMaps {
			parts[i] = fmt.Sprintf("territory %s, map \"%s\"", m.TerritoryCode, m.MapDescription)
		}
		fmt.Fprintf(&sb, "Completed maps (100%%): %s\n", strings.Join(parts, ", "))
	}
	if len(data.HighDNCMaps) > 0 {
		sb.WriteString("Highest DNC concentration:\n")
		for _, m := range data.HighDNCMaps {
			fmt.Fprintf(&sb, "  territory %s, map \"%s\" — %d DNC addresses\n", m.TerritoryCode, m.MapDescription, m.DNC)
		}
	}

	userMsg = sb.String()
	return systemMsg, userMsg
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func statusLabel(status string) string {
	if status == "not_done" {
		return "not_done (resets)"
	}
	return status
}
