package jobs

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

// TerritoryProgress holds the current status snapshot for a single territory,
// enriched with derived metrics computed before the LLM prompt.
type TerritoryProgress struct {
	Id                  string
	Code                string
	Description         string
	Progress            float64
	Total               int
	Done                int
	NotDone             int
	NotHome             int
	DNC                 int
	Invalid             int
	IsComplete          bool
	EstMonthsToComplete float64 // not_done / monthly_done_rate; 0 if complete or no rate
}

// ActivityItem represents the count and percentage of a single status in monthly activity.
type ActivityItem struct {
	Status string
	Count  int
	Pct    float64
}

// TerritoryMonthlyActivity holds what actually happened in a territory during the report month.
type TerritoryMonthlyActivity struct {
	TerritoryCode string
	Done          int
	NotHome       int
	DNC           int
	NotDone       int // re-opened (status changed back to not_done this month)
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
	CoveredActivity   string `json:"covered_activity"`
	TerritoryAnalysis string `json:"territory_analysis"`
	Conclusion        string `json:"conclusion"`
}

// SummaryData is the full data payload assembled from analytics views.
// Available is set true only after a successful LLM call populates all narrative fields.
type SummaryData struct {
	Available           bool
	CongregationName    string
	Period              string // "February 2026"
	Territories         []TerritoryProgress
	MonthlyByTerritory  []TerritoryMonthlyActivity // per-territory activity for the report month
	TotalChanges        int
	Activity            []ActivityItem
	PeakDay             string // "Feb 15 (47 changes)"
	SlowWeek            string // "Feb 1–7 (12 changes)"
	NotHomeFatigue      []NotHomeFatigue
	StalledMaps         []MapHealthItem
	CompletedMaps       []MapHealthItem
	HighDNCMaps         []MapHealthItem // top 3 by DNC count
	InactiveTerritories []string        // territory codes with no activity this month
	CoveredActivity     string          // section 1: what was covered this month
	TerritoryAnalysis   string          // section 2: per-territory observations
	Conclusion          string          // section 3: overall progress and encouragement
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
func queryTerritoryProgress(app *pocketbase.PocketBase, congregationId string, monthlyDoneRate float64) ([]TerritoryProgress, error) {
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
		if !tp.IsComplete && monthlyDoneRate > 0 && r.NotDone > 0 {
			raw := float64(r.NotDone) / monthlyDoneRate
			tp.EstMonthsToComplete = math.Round(raw*10) / 10
		}
		result = append(result, tp)
	}
	return result, nil
}

// queryMonthlyActivity fetches status-change totals for the given period from analytics_daily_status.
func queryMonthlyActivity(app *pocketbase.PocketBase, congregationId string, period ReportPeriod) (items []ActivityItem, total int, peakDay, slowWeek string, monthlyDoneRate float64, err error) {
	monthStart := period.Start.Format("2006-01-02")
	monthEnd := period.End.Format("2006-01-02")

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
		"start":        monthStart,
		"end":          monthEnd,
	}).All(&statusRows)
	if err != nil {
		return nil, 0, "", "", 0, fmt.Errorf("query monthly activity: %w", err)
	}

	doneCount := 0
	for _, r := range statusRows {
		total += r.Count
		if r.Status == "done" {
			doneCount = r.Count
		}
	}

	items = make([]ActivityItem, 0, len(statusRows))
	for _, r := range statusRows {
		pct := 0.0
		if total > 0 {
			pct = math.Round(float64(r.Count)/float64(total)*1000) / 10
		}
		items = append(items, ActivityItem{Status: r.Status, Count: r.Count, Pct: pct})
	}

	// Peak activity day
	type dayRow struct {
		Day   string `db:"day"`
		Total int    `db:"daily_total"`
	}
	var peak dayRow
	_ = app.DB().NewQuery(`
		SELECT day, SUM(change_count) AS daily_total
		FROM analytics_daily_status
		WHERE congregation = {:congregation}
		  AND day >= {:start}
		  AND day <  {:end}
		GROUP BY day
		ORDER BY daily_total DESC
		LIMIT 1
	`).Bind(dbx.Params{
		"congregation": congregationId,
		"start":        monthStart,
		"end":          monthEnd,
	}).One(&peak)
	if peak.Day != "" {
		if t, parseErr := time.Parse("2006-01-02", peak.Day); parseErr == nil {
			peakDay = fmt.Sprintf("%s (%d changes)", t.Format("Jan 2"), peak.Total)
		}
	}

	// First week of the month for context
	type weekRow struct {
		Total int `db:"week_total"`
	}
	var week weekRow
	_ = app.DB().NewQuery(`
		SELECT SUM(change_count) AS week_total
		FROM analytics_daily_status
		WHERE congregation = {:congregation}
		  AND day >= {:start}
		  AND day <  date(:start, '+7 days')
	`).Bind(dbx.Params{
		"congregation": congregationId,
		"start":        monthStart,
	}).One(&week)
	if week.Total > 0 {
		if t, parseErr := time.Parse("2006-01-02", monthStart); parseErr == nil {
			t7 := t.AddDate(0, 0, 6)
			slowWeek = fmt.Sprintf("Opening week %s–%s (%d changes)", t.Format("Jan 2"), t7.Format("Jan 2"), week.Total)
		}
	}

	monthlyDoneRate = float64(doneCount)
	return items, total, peakDay, slowWeek, monthlyDoneRate, nil
}

// queryMonthlyActivityByTerritory breaks down the period's status changes per territory.
// This is the primary "what happened this period" signal for each territory.
func queryMonthlyActivityByTerritory(app *pocketbase.PocketBase, congregationId string, period ReportPeriod) ([]TerritoryMonthlyActivity, error) {
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
		case "not_done":
			a.NotDone = r.Count
		}
	}

	// Collect into sorted slice (consistent ordering for prompt)
	result := make([]TerritoryMonthlyActivity, 0, len(byTerritory))
	for _, v := range byTerritory {
		result = append(result, *v)
	}
	// Sort by territory code
	for i := 1; i < len(result); i++ {
		for j := i; j > 0 && result[j].TerritoryCode < result[j-1].TerritoryCode; j-- {
			result[j], result[j-1] = result[j-1], result[j]
		}
	}
	return result, nil
}

// queryNotHomeFatigue fetches not-home retry counts per territory from analytics_not_home.
// Stale counts are addresses where the publisher has not re-attempted in more than 14 days.
func queryNotHomeFatigue(app *pocketbase.PocketBase, congregationId string) ([]NotHomeFatigue, error) {
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
func queryMapHealth(app *pocketbase.PocketBase, congregationId string) (stalled, completed, highDNC []MapHealthItem, err error) {
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

	for _, r := range rows {
		item := MapHealthItem{
			TerritoryCode:  r.TerritoryCode,
			MapCode:        r.MapCode,
			MapDescription: r.MapDescription,
			Progress:       r.Progress,
			DNC:            r.DNC,
			NotDone:        r.NotDone,
		}
		if r.Progress == 0 && r.NotDone > 0 {
			stalled = append(stalled, item)
		}
		if r.Progress >= 100 {
			completed = append(completed, item)
		}
	}

	// Top 3 maps by DNC count (minimum 1 DNC to be relevant)
	byDNC := make([]MapHealthItem, len(rows))
	for i, r := range rows {
		byDNC[i] = MapHealthItem{
			TerritoryCode:  r.TerritoryCode,
			MapCode:        r.MapCode,
			MapDescription: r.MapDescription,
			Progress:       r.Progress,
			DNC:            r.DNC,
			NotDone:        r.NotDone,
		}
	}
	for i := 1; i < len(byDNC); i++ {
		for j := i; j > 0 && byDNC[j].DNC > byDNC[j-1].DNC; j-- {
			byDNC[j], byDNC[j-1] = byDNC[j-1], byDNC[j]
		}
	}
	for i, m := range byDNC {
		if i >= 3 || m.DNC == 0 {
			break
		}
		highDNC = append(highDNC, m)
	}

	return stalled, completed, highDNC, nil
}

// queryInactiveTerritories returns codes of non-complete territories that had no
// activity in analytics_daily_status during the given period.
func queryInactiveTerritories(app *pocketbase.PocketBase, congregationId string, territories []TerritoryProgress, period ReportPeriod) []string {
	monthStart := period.Start.Format("2006-01-02")
	monthEnd := period.End.Format("2006-01-02")

	type row struct {
		Territory string `db:"territory"`
	}
	var rows []row
	_ = app.DB().NewQuery(`
		SELECT DISTINCT territory
		FROM analytics_daily_status
		WHERE congregation = {:congregation}
		  AND day >= {:start}
		  AND day <  {:end}
	`).Bind(dbx.Params{
		"congregation": congregationId,
		"start":        monthStart,
		"end":          monthEnd,
	}).All(&rows)

	activeIDs := make(map[string]bool, len(rows))
	for _, r := range rows {
		activeIDs[r.Territory] = true
	}

	var inactive []string
	for _, t := range territories {
		if !t.IsComplete && !activeIDs[t.Id] {
			inactive = append(inactive, t.Code)
		}
	}
	return inactive
}

// BuildSummaryData queries all four analytics views and assembles a SummaryData struct
// ready for prompt building. The Available field is left false until the LLM call succeeds.
func BuildSummaryData(app *pocketbase.PocketBase, congregation *core.Record, period ReportPeriod) (SummaryData, error) {
	cid := congregation.Id
	name, _ := congregation.Get("name").(string)

	activity, totalChanges, peakDay, slowWeek, monthlyDoneRate, err := queryMonthlyActivity(app, cid, period)
	if err != nil {
		return SummaryData{}, err
	}

	monthlyByTerritory, err := queryMonthlyActivityByTerritory(app, cid, period)
	if err != nil {
		return SummaryData{}, err
	}

	territories, err := queryTerritoryProgress(app, cid, monthlyDoneRate)
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

	inactive := queryInactiveTerritories(app, cid, territories, period)

	return SummaryData{
		Available:           false,
		CongregationName:    name,
		Period:              period.Label,
		Territories:         territories,
		MonthlyByTerritory:  monthlyByTerritory,
		TotalChanges:        totalChanges,
		Activity:            activity,
		PeakDay:             peakDay,
		SlowWeek:            slowWeek,
		NotHomeFatigue:      fatigue,
		StalledMaps:         stalled,
		CompletedMaps:       completed,
		HighDNCMaps:         highDNC,
		InactiveTerritories: inactive,
	}, nil
}

// BuildPrompt constructs the system and user messages sent to the LLM.
// All derived metrics must already be populated in data before calling this.
func BuildPrompt(data SummaryData) (systemMsg, userMsg string) {
	systemMsg = `You are the territory servant for a Jehovah's Witness congregation, writing the
territory activity report for your service overseer and fellow elders.

Your role is to give a clear, warm account of how the congregation's house-to-house
ministry progressed during this period — acknowledging the publishers' diligent efforts,
surfacing what needs attention, and encouraging continued zeal in the work of
sharing the good news of the Kingdom (Matthew 24:14; Acts 20:20).

MINISTRY CONTEXT:
- Publishers systematically visit households in assigned territories to share
  Bible-based material and offer the good news of God's Kingdom
- Each household visit is recorded with a status:
    done         — a householder was home and the publisher could share the good news
    not_home     — no one answered; publisher will make a return visit
    do_not_call  — householder declined further visits; permanently recorded
    invalid      — address is inaccessible, non-existent, or otherwise unreachable
    not_done     — address was reset to unworked, awaiting a publisher
- not_home addresses represent householders who still need an opportunity to hear
  the Kingdom message; publishers make return visits to reach them:
    retrying             — within the allowed attempt limit; return visits are planned (normal)
    high not home tries  — reached the maximum attempts with no contact; the territory
                           servant must decide: reset the address, note as invalid,
                           or organise a special return visit effort
- A stalled map (0% progress with unworked addresses) means those householders have
  not yet been reached — it is likely unassigned or inadvertently overlooked
- Territory progress is cumulative coverage — not just this period's work

REPORT READERS:
- Territory servant: assigns maps to publishers, tracks territory progress,
  follows up on high not-home-tries addresses, ensures no territory sits idle
- Service overseer: has oversight of the overall field ministry health,
  encourages publishers, and supports the territory servant in keeping
  the good news reaching every household

DATA SCOPE:
- "TERRITORY ACTIVITY" = what happened during the report period per territory, enriched with each
  territory's current overall state (Overall%, Remaining addresses, Est. Months to finish)
  Column key:
    Done / Not Home / DNC / Re-opened  = this period's counts only
    Total / Invalid / Overall% / Remaining / Est.Months = cumulative territory state as of report date
  "Re-opened" specifically = individual addresses re-opened for ministry this month
    (status changed back to not_done; small counts are normal and not worth mentioning)
  "Total" specifically = total addresses in the territory (all statuses combined)
  "Invalid" specifically = addresses that are permanently unreachable (inaccessible, non-existent);
    these are included in Total but can never be completed — high Invalid lowers the maximum achievable progress
  "Remaining" specifically = total not_done addresses in the territory right now (not just this month)
  "Overall%" = (done + exhausted not-home) / Total × 100 — cumulative, includes all prior months' work
  "Est.Months" = Remaining ÷ this month's Done rate; treat as a rough guide only —
    one unusually active or quiet month can make this estimate misleading
- "NOT-HOME STANDING" and "MAP HEALTH" = current state as of the report date

VERIFY BEFORE WRITING:
Before drafting any section, identify from TERRITORY ACTIVITY:
1. Which territories have Done > 0 this month (only these are "active" for section 2)
2. The exact Done count per active territory — use the number as printed; do not adjust it
3. Whether any active territory has concerns in NOT-HOME STANDING or MAP HEALTH
Use these verified facts as your sole basis for sections 1 and 2.

WRITING STYLE:
- Write in plain, simple English — short sentences, everyday words
- Avoid formal or corporate-sounding phrases (e.g. "in the absence of", "lay the groundwork",
  "rekindling systematic outreach", "tangible progress")
- Write as if speaking warmly to a fellow elder, not drafting an official document
- When there was no activity, say so plainly and briefly — do not pad with generic advice
- Each sentence should carry one clear idea; do not chain multiple clauses together

REPORT STRUCTURE — write exactly three sections in this order:

Section 1 — COVERED ACTIVITY (2–3 sentences):
  Summarise what the congregation accomplished in the field ministry during this period.
  How many territories saw active house-to-house work? How many households were reached?
  Acknowledge the publishers' faithful efforts warmly — even small steps matter in this work.

Section 2 — TERRITORY ANALYSIS (3–5 sentences):
  Focus on the territories that were actively worked this month.
  Describe the meaningful progress made — which territories advanced, how many households
  were reached, and what the numbers reveal about the pace of work.
  If a worked territory has stalled maps or high not-home-tries, mention those specifically
  as items for the territory servant's attention.
  Do NOT mention inactive territories or suggest which territories should be assigned next —
  that is the territory servant's decision, not the report's role.

Section 3 — CONCLUSION (2–3 sentences):
  Give an honest overall picture of the congregation's ministry momentum during this period.
  If there was no activity, acknowledge it plainly and briefly.
  Close with one warm, specific encouragement for the period ahead.

RULES:
- Base analysis primarily on "TERRITORY ACTIVITY" (this period's data)
- Use not-home standing and map health as supporting context
- Cite specific numbers from the data — no vague generalisations
- Do not invent or infer data not present in the input
- Each section: 2–5 sentences, warm and encouraging tone
- Do not mention inactive territories in section 2 — inactive territory data is for the
  territory servant's own reference, not for the narrative analysis
- Do not make assignment recommendations — which territory gets assigned next is the
  territory servant's decision, not the report's role
- Do not mention the "Re-opened" count in narrative — it is an admin-level detail
  (individual addresses re-opened for ministry) and is not meaningful for the activity summary
- When referencing a map in narrative text, use the map's description as shown in the data
  (e.g. "map [description] in territory [territory]") — never use slash notation like "M05/112 (5)"
- Respond only in this exact JSON schema:
{
  "covered_activity": "<section 1 paragraph>",
  "territory_analysis": "<section 2 paragraph>",
  "conclusion": "<section 3 paragraph>"
}`

	var sb strings.Builder
	fmt.Fprintf(&sb, "CONGREGATION: %s\nREPORT PERIOD: %s\n\n", data.CongregationName, data.Period)

	// Build a lookup map: territory code → TerritoryProgress for enriching the monthly table
	territoryByCode := make(map[string]TerritoryProgress, len(data.Territories))
	for _, t := range data.Territories {
		territoryByCode[t.Code] = t
	}

	// ── Section 1: Enriched per-territory activity (primary analysis signal) ──
	// Each row shows what happened this month alongside the territory's current overall state,
	// so the LLM can contextualise monthly activity against total size and cumulative progress.
	fmt.Fprintf(&sb, "TERRITORY ACTIVITY — %s (with overall context):\n", data.Period)
	if len(data.MonthlyByTerritory) > 0 {
		sb.WriteString("Done/Not Home/DNC/Re-opened = this month | Total/Invalid/Overall%/Remaining/Est.Months = cumulative state\n")
		sb.WriteString("Territory | Done | Not Home | DNC | Re-opened | Total | Invalid | Overall% | Remaining | Est.Months\n")
		for _, a := range data.MonthlyByTerritory {
			t := territoryByCode[a.TerritoryCode]
			est := "unknown"
			if t.IsComplete {
				est = "complete"
			} else if t.EstMonthsToComplete > 0 {
				est = fmt.Sprintf("~%.1f", t.EstMonthsToComplete)
			}
			fmt.Fprintf(&sb, "%-10s| %4d | %8d | %3d | %6d | %5d | %7d | %7.0f%% | %9d | %s\n",
				truncate(a.TerritoryCode, 10),
				a.Done, a.NotHome, a.DNC, a.NotDone,
				t.Total, t.Invalid,
				t.Progress, t.NotDone, est)
		}
	} else {
		sb.WriteString("No activity recorded this period.\n")
	}

	// Also list territories that exist but had zero activity this month
	if len(data.InactiveTerritories) > 0 {
		sb.WriteString("\nTerritories with ZERO activity this period (cumulative state shown):\n")
		sb.WriteString("Territory | Total | Invalid | Overall% | Remaining | Est.Months\n")
		for _, code := range data.InactiveTerritories {
			t := territoryByCode[code]
			est := "unknown"
			if t.IsComplete {
				est = "complete"
			} else if t.EstMonthsToComplete > 0 {
				est = fmt.Sprintf("~%.1f", t.EstMonthsToComplete)
			}
			fmt.Fprintf(&sb, "%-10s| %5d | %7d | %7.0f%% | %9d | %s\n",
				truncate(code, 10), t.Total, t.Invalid, t.Progress, t.NotDone, est)
		}
	}

	// Congregation-wide totals
	fmt.Fprintf(&sb, "\nCongregation totals for %s:\n", data.Period)
	fmt.Fprintf(&sb, "Total status changes: %d\n", data.TotalChanges)
	for _, a := range data.Activity {
		fmt.Fprintf(&sb, "  %-24s %4d (%5.1f%%)\n", statusLabel(a.Status)+":", a.Count, a.Pct)
	}
	if data.PeakDay != "" {
		fmt.Fprintf(&sb, "Peak day: %s\n", data.PeakDay)
	}
	if data.SlowWeek != "" {
		fmt.Fprintf(&sb, "Opening week: %s\n", data.SlowWeek)
	}

	// ── Section 2: Not-home standing (current state) ──
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

	// ── Section 3: Map health (current state) ──
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
	switch status {
	case "done":
		return "done"
	case "not_done":
		return "not_done (resets)"
	case "not_home":
		return "not_home"
	case "do_not_call":
		return "do_not_call"
	case "invalid":
		return "invalid"
	default:
		return status
	}
}
