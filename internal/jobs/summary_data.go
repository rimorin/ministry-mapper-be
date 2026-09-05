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
	PrevDone      int // done in the previous period, for a pre-computed comparison
}

// Thresholds that turn raw counts into something worth an admin's attention.
// Small territories trip percentage rules on one or two addresses, so every
// rule also carries a minimum count.
const (
	fatigueReviewPct   = 35 // share of not-home addresses at the attempt limit
	fatigueReviewMin   = 10 // ...and at least this many of them
	staleMin           = 10 // retrying addresses untouched for over 14 days
	highDNCMin         = 5  // do-not-call addresses on one map
	actionItemsPerKind = 3
)

// NotHomeFatigue summarises not-home retry state per territory.
type NotHomeFatigue struct {
	TerritoryCode string
	MaxedOut      int
	Retrying      int
	Stale         int     // retrying addresses not retried in >14 days
	MaxedOutPct   float64 // maxed_out / total * 100, pre-computed
	ReviewNeeded  bool    // MaxedOut >= fatigueReviewMin and MaxedOutPct >= fatigueReviewPct
}

// ActionItem is one line the territory servant should act on. The list is
// derived from thresholds in Go, shown in the email as a checklist, and handed
// to the model so the narrative and the list never disagree.
type ActionItem struct {
	Category string
	Text     string
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
	// The period of equal length before this one, pre-counted so any comparison
	// the narrative makes quotes a computed change rather than deriving one.
	PreviousPeriod        string
	HasPrevious           bool // false when the earlier window saw no visits
	PrevHouseholdsReached int
	PrevVisits            int
	PrevActiveTerritories int
	NotHomeFatigue        []NotHomeFatigue
	StalledMaps           []MapHealthItem
	CompletedMaps         []MapHealthItem
	HighDNCMaps           []MapHealthItem // top 3 by DNC count
	Coverage              string          // paragraph 1: what was covered this period
	NeedsAttention        string          // paragraph 2: items for the territory servant to act on
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

// previous returns the period of equal length that ends where p starts, labelled
// in the same style, so the narrative compares like with like.
func (p ReportPeriod) previous() ReportPeriod {
	if !p.IsOnDemand {
		start := p.Start.AddDate(0, -1, 0)
		return ReportPeriod{Start: start, End: p.Start, Label: start.Format("January 2006")}
	}
	start := p.Start.Add(-p.End.Sub(p.Start))
	last := p.Start.AddDate(0, 0, -1)
	return ReportPeriod{
		Start:      start,
		End:        p.Start,
		Label:      fmt.Sprintf("%s – %s", start.Format("2 Jan 2006"), last.Format("2 Jan 2006")),
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
// Stale counts retrying addresses that nobody has re-attempted in more than 14 days;
// maxed-out addresses are excluded because no retry is expected for them.
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
		       SUM(CASE WHEN anh.retry_status = 'retrying'
		                 AND JULIANDAY('now') - JULIANDAY(anh.updated) > 14 THEN 1 ELSE 0 END) AS stale
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
			ReviewNeeded:  r.MaxedOut >= fatigueReviewMin && pct >= fatigueReviewPct,
		})
	}
	return result, nil
}

// stripTerritorySuffix removes a trailing "(CODE)" from a map description when
// it repeats the territory code, so the narrative does not say "M04 ... (M04)".
func stripTerritorySuffix(description, territoryCode string) string {
	suffix := "(" + territoryCode + ")"
	if territoryCode != "" && strings.HasSuffix(description, suffix) {
		return strings.TrimSpace(strings.TrimSuffix(description, suffix))
	}
	return description
}

// queryMapHealth fetches per-map progress from analytics_maps and classifies
// maps as stalled (0%, work remaining; the three with most unworked addresses),
// completed (100%), or high DNC (at least highDNCMin, top three).
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
			MapDescription: stripTerritorySuffix(r.MapDescription, r.TerritoryCode),
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

	sort.SliceStable(stalled, func(i, j int) bool { return stalled[i].NotDone > stalled[j].NotDone })
	if len(stalled) > actionItemsPerKind {
		stalled = stalled[:actionItemsPerKind]
	}

	byDNC := append([]MapHealthItem(nil), items...)
	sort.SliceStable(byDNC, func(i, j int) bool { return byDNC[i].DNC > byDNC[j].DNC })
	for i, m := range byDNC {
		if i >= actionItemsPerKind || m.DNC < highDNCMin {
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

	prev := period.previous()
	_, prevReached, prevVisits, err := queryMonthlyActivity(app, cid, prev)
	if err != nil {
		return SummaryData{}, err
	}
	prevByTerritory, err := queryMonthlyActivityByTerritory(app, cid, prev)
	if err != nil {
		return SummaryData{}, err
	}
	prevDone := make(map[string]int, len(prevByTerritory))
	for _, a := range prevByTerritory {
		prevDone[a.TerritoryCode] = a.Done
	}
	for i := range monthlyByTerritory {
		monthlyByTerritory[i].PrevDone = prevDone[monthlyByTerritory[i].TerritoryCode]
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
		Available:             false,
		CongregationName:      name,
		Period:                period.Label,
		Territories:           territories,
		MonthlyByTerritory:    monthlyByTerritory,
		Activity:              activity,
		ActiveTerritories:     len(monthlyByTerritory),
		HouseholdsReached:     reached,
		Visits:                visits,
		PreviousPeriod:        prev.Label,
		HasPrevious:           prevVisits > 0,
		PrevHouseholdsReached: prevReached,
		PrevVisits:            prevVisits,
		PrevActiveTerritories: len(prevByTerritory),
		NotHomeFatigue:        fatigue,
		StalledMaps:           stalled,
		CompletedMaps:         completed,
		HighDNCMaps:           highDNC,
	}, nil
}

// ActionItems derives the prioritised checklist from the thresholded data: the
// territories needing a not-home review, those with stale retries, stalled maps
// and DNC-heavy maps, at most actionItemsPerKind of each, most severe first.
func (d SummaryData) ActionItems() []ActionItem {
	var items []ActionItem

	review := make([]NotHomeFatigue, 0, len(d.NotHomeFatigue))
	stale := make([]NotHomeFatigue, 0, len(d.NotHomeFatigue))
	for _, f := range d.NotHomeFatigue {
		if f.ReviewNeeded {
			review = append(review, f)
		}
		if f.Stale >= staleMin {
			stale = append(stale, f)
		}
	}
	sort.SliceStable(review, func(i, j int) bool { return review[i].MaxedOut > review[j].MaxedOut })
	sort.SliceStable(stale, func(i, j int) bool { return stale[i].Stale > stale[j].Stale })

	// Everyday words, one fact each: the reader is often an elder on a phone.
	for _, f := range capItems(review) {
		items = append(items, ActionItem{"Nobody home after all tries", fmt.Sprintf(
			"%s: %d homes had nobody home on every allowed visit. Decide whether to reset them, mark them invalid, or plan a special visit.",
			f.TerritoryCode, f.MaxedOut)})
	}
	for _, f := range capItems(stale) {
		items = append(items, ActionItem{"Return visits overdue", fmt.Sprintf(
			"%s: %d homes are waiting for a return visit and have not been tried for over two weeks.", f.TerritoryCode, f.Stale)})
	}
	for _, m := range capItems(d.StalledMaps) {
		items = append(items, ActionItem{"Map not started", fmt.Sprintf(
			"%s, map \"%s\": %d homes, none visited yet.", m.TerritoryCode, m.MapDescription, m.NotDone)})
	}
	for _, m := range capItems(d.HighDNCMaps) {
		items = append(items, ActionItem{"Many do-not-call homes", fmt.Sprintf(
			"%s, map \"%s\": %d homes have asked not to be called again.", m.TerritoryCode, m.MapDescription, m.DNC)})
	}
	return items
}

func capItems[T any](items []T) []T {
	if len(items) > actionItemsPerKind {
		return items[:actionItemsPerKind]
	}
	return items
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
- PREVIOUS PERIOD — the same figures for the period of equal length before this
  one, with the change already computed (this period minus previous). These are
  verified facts too. When the previous period line says no visits were recorded,
  there is nothing to compare against.
- TERRITORY ACTIVITY — one row per territory that saw real visits this period,
  ordered by Done, highest first. Territories with no visits are not listed.
    Done / Not Home / DNC                  = this period's visits only
    Prev Done                              = done in the previous period, for comparison
    Total / Invalid / Overall% / Remaining = cumulative state as of the report date
    Total     = every address in the territory, all statuses combined
    Invalid   = permanently unreachable; counted in Total but can never be completed,
                so a high Invalid lowers the maximum achievable progress
    Overall%  = (done + exhausted not-home) / Total x 100, cumulative across all periods
    Remaining = not_done addresses in the territory right now, not just this period
- ACTION ITEMS — the concerns the territory servant should act on, already
  thresholded and ordered most serious first: territories with high not home tries
  above the review level, stale return visits, stalled maps, and DNC-heavy maps.
  The reader sees this same list beside your text, so you never need to repeat it all.
- COMPLETED MAPS — maps that reached 100% as of the report date

WRITE EXACTLY TWO PARAGRAPHS:

Paragraph 1 — WHAT WAS DONE (2-3 sentences):
  How many territories had visits and how many homes were reached, from VERIFIED FACTS.
  If PREVIOUS PERIOD figures are given, one sentence: "up N from M" or "down N from M",
  quoting both figures exactly as given.
  Name the three territories where the most homes were reached, with their Done figures.
  If a map was finished, name it — that is the clearest result worth reporting.

Paragraph 2 — WHAT NEEDS ATTENTION (1-3 sentences):
  Pick the single most serious item from up to three categories of ACTION ITEMS,
  at most three items in total. Name the place, give the number, say what it means.
  Do not list every item; the reader has the full list next to your text.
  If ACTION ITEMS is empty, say in one sentence that there is nothing to act on.

WRITING RULES:
- Write for a busy reader on a phone. Sentences of 15 words or fewer. One fact per sentence.
- Start each sentence with the place (territory or map), then the number, then what it means.
- Everyday words only. Say "homes", not households, addresses or units. Say "nobody home",
  not not_home. Say "do not call", not DNC. Say "finished", not completed or 100%. Say
  "not started", not stalled or 0%. Call reached homes "reached", every time.
- Never use these words: attempt limit, threshold, review level, concentration, cumulative,
  retrying, status, progress percentage.
- Use only the figures given. Never invent, infer or recompute a number. Compare with the
  previous period only through the change values given; when none are given, make no
  comparison at all.
- Do not judge the period as good, busy, strong, slow or encouraging. No praise, no
  exhortation. A high nobody-home count is a concern, never a strength.
- Do not recommend which territory to assign next — that is the territory servant's decision.
- Name a map by its description as shown, e.g. map "Block 412" in territory M05 — never
  slash notation like "M05/112 (5)".

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

	if data.HasPrevious {
		fmt.Fprintf(&sb, "\nPREVIOUS PERIOD (%s), with change to this period already computed:\n", data.PreviousPeriod)
		fmt.Fprintf(&sb, "  %-37s %d   change: %+d\n", "Territories with visits:", data.PrevActiveTerritories, data.ActiveTerritories-data.PrevActiveTerritories)
		fmt.Fprintf(&sb, "  %-37s %d   change: %+d\n", "Households reached (done):", data.PrevHouseholdsReached, data.HouseholdsReached-data.PrevHouseholdsReached)
		fmt.Fprintf(&sb, "  %-37s %d   change: %+d\n", "Total visits (done + not home + DNC):", data.PrevVisits, data.Visits-data.PrevVisits)
	} else {
		fmt.Fprintf(&sb, "\nPREVIOUS PERIOD (%s): no visits recorded, so there is nothing to compare against.\n", data.PreviousPeriod)
	}

	// Build a lookup map: territory code → TerritoryProgress for enriching the activity table
	territoryByCode := make(map[string]TerritoryProgress, len(data.Territories))
	for _, t := range data.Territories {
		territoryByCode[t.Code] = t
	}

	// ── Per-territory activity, ranked by Done (primary analysis signal) ──
	fmt.Fprintf(&sb, "\nTERRITORY ACTIVITY — %s (ordered by Done, highest first):\n", data.Period)
	if len(data.MonthlyByTerritory) > 0 {
		sb.WriteString("Done/Prev Done/Not Home/DNC = this and previous period | Total/Invalid/Overall%/Remaining = cumulative state\n")
		sb.WriteString("Territory | Done | Prev Done | Not Home | DNC | Total | Invalid | Overall% | Remaining\n")
		for _, a := range data.MonthlyByTerritory {
			t := territoryByCode[a.TerritoryCode]
			fmt.Fprintf(&sb, "%-10s| %4d | %9d | %8d | %3d | %5d | %7d | %7.0f%% | %9d\n",
				truncate(a.TerritoryCode, 10),
				a.Done, a.PrevDone, a.NotHome, a.DNC,
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

	// ── Action items (thresholded and prioritised in Go) ──
	items := data.ActionItems()
	if len(items) == 0 {
		sb.WriteString("\nACTION ITEMS: none\n")
	} else {
		sb.WriteString("\nACTION ITEMS (most serious first; the reader sees this list too):\n")
		for _, item := range items {
			fmt.Fprintf(&sb, "  [%s] %s\n", item.Category, item.Text)
		}
	}

	// ── Completed maps ──
	if len(data.CompletedMaps) > 0 {
		parts := make([]string, len(data.CompletedMaps))
		for i, m := range data.CompletedMaps {
			parts[i] = fmt.Sprintf("territory %s, map \"%s\"", m.TerritoryCode, m.MapDescription)
		}
		fmt.Fprintf(&sb, "\nCOMPLETED MAPS (100%%): %s\n", strings.Join(parts, ", "))
	} else {
		sb.WriteString("\nCOMPLETED MAPS (100%): none\n")
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
