package assistant

import (
	"encoding/json"
	"sort"
	"strings"

	"ganigeorgiev.com/pocketbase/dbx"
)

// completionTrackingNote is attached wherever completion dates appear: rounds were
// first recorded in October 2026, so an empty date can mean "before tracking".
const completionTrackingNote = "Completion dates are recorded from October 2026. A territory with no " +
	"last_completed date has not been completed since then; it may have been completed before. " +
	"A territory is completed when its progress reaches the congregation's completion threshold."

var territoryOverview = Tool{
	Name: "territory_overview",
	Description: "One row per territory: homes, progress, status counts, when the current round started, " +
		"when it was last completed, when anyone last recorded a visit there, and whether it has a boundary. " +
		"Use the filters for questions like \"not completed in 6 months\" or \"not worked since June\".",
	Params: object(map[string]any{
		"territory":                 nullable("string", "Exact territory code, or null for every territory."),
		"not_completed_within_days": nullable("integer", "Only territories not completed in this many days, including never completed."),
		"not_worked_within_days":    nullable("integer", "Only territories with no visit recorded in this many days."),
		"min_progress":              nullable("integer", "Only territories at or above this progress percentage."),
		"max_progress":              nullable("integer", "Only territories at or below this progress percentage."),
		"sort":                      nullableEnum("Sort order, default by code.", "code", "progress", "last_completed", "last_worked", "homes"),
		"limit":                     limitParam(),
	}),
	Run: func(s Scope, raw json.RawMessage) (Result, error) {
		var args struct {
			Territory      *string `json:"territory"`
			NotCompletedIn *int    `json:"not_completed_within_days"`
			NotWorkedIn    *int    `json:"not_worked_within_days"`
			MinProgress    *int    `json:"min_progress"`
			MaxProgress    *int    `json:"max_progress"`
			Sort           *string `json:"sort"`
			Limit          *int    `json:"limit"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return Result{}, err
		}
		if err := validDays("not_completed_within_days", args.NotCompletedIn); err != nil {
			return Result{}, err
		}
		if err := validDays("not_worked_within_days", args.NotWorkedIn); err != nil {
			return Result{}, err
		}
		territoryID, err := territoryIDByCode(s, args.Territory)
		if err != nil {
			return Result{}, err
		}

		var rows []struct {
			ID            string `db:"id"`
			Code          string `db:"code"`
			Description   string `db:"description"`
			Progress      int    `db:"progress"`
			RoundStarted  string `db:"round_started_at"`
			LastCompleted string `db:"last_completed_at"`
			HasBoundary   bool   `db:"has_boundary"`
			Maps          int    `db:"maps"`
			Homes         int    `db:"homes"`
			Done          int    `db:"done"`
			NotHome       int    `db:"not_home"`
			AtTryLimit    int    `db:"at_try_limit"`
			NotDone       int    `db:"not_done"`
			DNC           int    `db:"dnc"`
			Invalid       int    `db:"invalid"`
			LastWorked    string `db:"last_worked"`
		}
		err = s.App.DB().NewQuery(`
			SELECT t.id, t.code, t.description, COALESCE(t.progress, 0) AS progress,
			       COALESCE(t.round_started_at, '') AS round_started_at,
			       COALESCE(t.last_completed_at, '') AS last_completed_at,
			       (CASE WHEN json_valid(t.coordinates) THEN json_array_length(t.coordinates) ELSE 0 END) >= 3 AS has_boundary,
			       COUNT(m.id) AS maps,
			       COALESCE(SUM(` + agg("total") + `), 0) AS homes,
			       COALESCE(SUM(` + agg("done") + `), 0) AS done,
			       COALESCE(SUM(` + agg("notHome") + `), 0) AS not_home,
			       COALESCE(SUM(` + aggAtTryLimit + `), 0) AS at_try_limit,
			       COALESCE(SUM(` + agg("notDone") + `), 0) AS not_done,
			       COALESCE(SUM(` + agg("dnc") + `), 0) AS dnc,
			       COALESCE(SUM(` + agg("invalid") + `), 0) AS invalid,
			       COALESCE((SELECT MAX(l.created) FROM addresses_log l
			                 WHERE l.territory = t.id AND l.new_status IN ` + coverageStatuses + `), '') AS last_worked
			FROM territories t
			LEFT JOIN maps m ON m.territory = t.id
			WHERE t.congregation = {:congregation} AND ({:territory} = '' OR t.id = {:territory})
			GROUP BY t.id
		`).Bind(dbx.Params{"congregation": s.CongregationID, "territory": territoryID}).All(&rows)
		if err != nil {
			return Result{}, err
		}

		completedCutoff, workedCutoff := "", ""
		if args.NotCompletedIn != nil {
			completedCutoff = daysAgo(s, *args.NotCompletedIn)
		}
		if args.NotWorkedIn != nil {
			workedCutoff = daysAgo(s, *args.NotWorkedIn)
		}

		r := Result{
			Columns: []string{"territory", "description", "maps", "homes", "progress", "done", "not_done", "not_home", "at_try_limit", "dnc", "invalid", "round_open_since", "last_completed", "last_worked", "has_boundary"},
			Note:    completionTrackingNote + " " + homesNote,
		}
		for _, t := range rows {
			if completedCutoff != "" && t.LastCompleted != "" && t.LastCompleted >= completedCutoff {
				continue
			}
			if workedCutoff != "" && t.LastWorked != "" && t.LastWorked >= workedCutoff {
				continue
			}
			if args.MinProgress != nil && t.Progress < *args.MinProgress {
				continue
			}
			if args.MaxProgress != nil && t.Progress > *args.MaxProgress {
				continue
			}
			roundOpenSince := ""
			if t.RoundStarted != "" && (t.LastCompleted == "" || t.LastCompleted < t.RoundStarted) {
				roundOpenSince = localDate(s, t.RoundStarted)
			}
			r.Rows = append(r.Rows, map[string]any{
				"territory_id": t.ID, "territory": t.Code, "description": t.Description,
				"maps": t.Maps, "homes": t.Homes, "progress": t.Progress,
				"done": t.Done, "not_done": t.NotDone, "not_home": t.NotHome + t.AtTryLimit, "at_try_limit": t.AtTryLimit,
				"dnc": t.DNC, "invalid": t.Invalid,
				"round_open_since": roundOpenSince, "last_completed": localDate(s, t.LastCompleted),
				"last_worked": localDate(s, t.LastWorked), "has_boundary": t.HasBoundary,
			})
		}
		sortRows(r.Rows, args.Sort, "territory")
		return truncate(r, clampLimit(args.Limit)), nil
	},
}

var mapOverview = Tool{
	Name: "map_overview",
	Description: "One row per map (a building or a street of houses): its territory, homes, progress, status " +
		"counts, when a visit was last recorded, how many share links are active now, and whether it has a " +
		"location. Use it for questions about blocks or streets, and for what is free to assign.",
	Params: object(map[string]any{
		"territory":                nullable("string", "Exact territory code, or null for every territory."),
		"not_worked_within_days":   nullable("integer", "Only maps with no visit recorded in this many days."),
		"min_progress":             nullable("integer", "Only maps at or above this progress percentage."),
		"max_progress":             nullable("integer", "Only maps at or below this progress percentage."),
		"unassigned_only":          nullable("boolean", "Only maps with no active share link."),
		"not_assigned_within_days": nullable("integer", "Only maps nobody was given a share link for in this many days, including never."),
		"min_not_done":             nullable("integer", "Only maps with at least this many homes not done."),
		"sort":                     nullableEnum("Sort order, default by territory then map.", "code", "progress", "last_worked", "last_assigned", "homes", "not_done"),
		"limit":                    limitParam(),
	}),
	Run: func(s Scope, raw json.RawMessage) (Result, error) {
		var args struct {
			Territory      *string `json:"territory"`
			NotWorkedIn    *int    `json:"not_worked_within_days"`
			MinProgress    *int    `json:"min_progress"`
			MaxProgress    *int    `json:"max_progress"`
			UnassignedOnly *bool   `json:"unassigned_only"`
			NotAssignedIn  *int    `json:"not_assigned_within_days"`
			MinNotDone     *int    `json:"min_not_done"`
			Sort           *string `json:"sort"`
			Limit          *int    `json:"limit"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return Result{}, err
		}
		if err := validDays("not_worked_within_days", args.NotWorkedIn); err != nil {
			return Result{}, err
		}
		if err := validDays("not_assigned_within_days", args.NotAssignedIn); err != nil {
			return Result{}, err
		}
		territoryID, err := territoryIDByCode(s, args.Territory)
		if err != nil {
			return Result{}, err
		}

		var rows []struct {
			ID           string `db:"id"`
			LastAssigned string `db:"last_assigned"`
			Map          string `db:"map"`
			Type         string `db:"type"`
			TerritoryID  string `db:"territory_id"`
			Territory    string `db:"territory"`
			Progress     int    `db:"progress"`
			Homes        int    `db:"homes"`
			Done         int    `db:"done"`
			NotHome      int    `db:"not_home"`
			AtTryLimit   int    `db:"at_try_limit"`
			NotDone      int    `db:"not_done"`
			DNC          int    `db:"dnc"`
			Invalid      int    `db:"invalid"`
			HasLocation  bool   `db:"has_location"`
			LastWorked   string `db:"last_worked"`
			ActiveLinks  int    `db:"active_links"`
		}
		err = s.App.DB().NewQuery(`
			SELECT m.id, m.description AS map, m.type, t.id AS territory_id, t.code AS territory,
			       COALESCE(m.progress, 0) AS progress,
			       COALESCE(` + agg("total") + `, 0) AS homes,
			       COALESCE(` + agg("done") + `, 0) AS done,
			       COALESCE(` + agg("notHome") + `, 0) AS not_home,
			       COALESCE(` + aggAtTryLimit + `, 0) AS at_try_limit,
			       COALESCE(` + agg("notDone") + `, 0) AS not_done,
			       COALESCE(` + agg("dnc") + `, 0) AS dnc,
			       COALESCE(` + agg("invalid") + `, 0) AS invalid,
			       (json_valid(m.coordinates) AND json_extract(m.coordinates, '$.lat') IS NOT NULL) AS has_location,
			       COALESCE((SELECT MAX(l.created) FROM addresses_log l
			                 WHERE l.map = m.id AND l.new_status IN ` + coverageStatuses + `), '') AS last_worked,
			       (SELECT COUNT(*) FROM assignments a WHERE a.map = m.id AND a.expiry_date > {:now}) AS active_links,
			       COALESCE((SELECT MAX(al.created) FROM assignments_log al
			                 WHERE al.map = m.id AND al.action = 'assigned'), '') AS last_assigned
			FROM maps m JOIN territories t ON t.id = m.territory
			WHERE m.congregation = {:congregation} AND ({:territory} = '' OR t.id = {:territory})
		`).Bind(dbx.Params{"congregation": s.CongregationID, "territory": territoryID, "now": dbTime(s.Now)}).All(&rows)
		if err != nil {
			return Result{}, err
		}

		workedCutoff, assignedCutoff := "", ""
		if args.NotWorkedIn != nil {
			workedCutoff = daysAgo(s, *args.NotWorkedIn)
		}
		if args.NotAssignedIn != nil {
			assignedCutoff = daysAgo(s, *args.NotAssignedIn)
		}
		r := Result{
			Columns: []string{"territory", "map", "type", "homes", "progress", "done", "not_done", "not_home", "at_try_limit", "dnc", "invalid", "last_worked", "last_assigned", "active_links", "has_location"},
			Note:    homesNote + " Share links are recorded from May 2026, and links from the quick-assign button only from November 2026; an empty last_assigned can mean before then.",
		}
		for _, m := range rows {
			switch {
			case workedCutoff != "" && m.LastWorked != "" && m.LastWorked >= workedCutoff,
				args.MinProgress != nil && m.Progress < *args.MinProgress,
				args.MaxProgress != nil && m.Progress > *args.MaxProgress,
				args.UnassignedOnly != nil && *args.UnassignedOnly && m.ActiveLinks > 0,
				assignedCutoff != "" && m.LastAssigned != "" && m.LastAssigned >= assignedCutoff,
				args.MinNotDone != nil && m.NotDone < *args.MinNotDone:
				continue
			}
			r.Rows = append(r.Rows, map[string]any{
				"map_id": m.ID, "map": m.Map, "type": m.Type, "territory_id": m.TerritoryID, "territory": m.Territory,
				"homes": m.Homes, "progress": m.Progress, "done": m.Done, "not_done": m.NotDone,
				"not_home": m.NotHome + m.AtTryLimit, "at_try_limit": m.AtTryLimit, "dnc": m.DNC, "invalid": m.Invalid,
				"last_worked": localDate(s, m.LastWorked), "last_assigned": localDate(s, m.LastAssigned),
				"active_links": m.ActiveLinks, "has_location": m.HasLocation,
			})
		}
		sortRows(r.Rows, args.Sort, "map")
		return truncate(r, clampLimit(args.Limit)), nil
	},
}

// agg reads one count from maps.aggregates, which is empty until the map's first
// recalculation; json_extract on an empty string is an error, not NULL.
func agg(field string) string {
	return "(CASE WHEN json_valid(m.aggregates) THEN json_extract(m.aggregates, '$." + field + "') END)"
}

// aggAtTryLimit is a map's nobody-home homes at the try limit. The app stores
// them inside completed and keeps notHome for those below the limit; the
// assistant reports every nobody-home home as not_home, with these as a part.
var aggAtTryLimit = "(" + agg("completed") + " - " + agg("done") + ")"

// sortRows orders rows by the requested key. Dates sort oldest first with empty
// (never) first, so "least recently" questions read top-down; counts sort largest
// first; codes sort naturally.
func sortRows(rows []map[string]any, by *string, nameKey string) {
	key := "code"
	if by != nil {
		key = *by
	}
	less := func(a, b map[string]any) bool {
		return naturalLess(a["territory"].(string)+" "+a[nameKey].(string), b["territory"].(string)+" "+b[nameKey].(string))
	}
	switch key {
	case "progress":
		less = func(a, b map[string]any) bool { return a["progress"].(int) < b["progress"].(int) }
	case "last_completed", "last_worked", "last_assigned":
		less = func(a, b map[string]any) bool { return a[key].(string) < b[key].(string) }
	case "homes", "not_done":
		less = func(a, b map[string]any) bool { return a[key].(int) > b[key].(int) }
	}
	sort.SliceStable(rows, func(i, j int) bool { return less(rows[i], rows[j]) })
}

// naturalLess compares strings with embedded numbers by value, so T2 < T10.
func naturalLess(a, b string) bool {
	a, b = strings.ToLower(a), strings.ToLower(b)
	for a != "" && b != "" {
		na, ra := leadingNumber(a)
		nb, rb := leadingNumber(b)
		if na >= 0 && nb >= 0 {
			if na != nb {
				return na < nb
			}
			a, b = ra, rb
			continue
		}
		if a[0] != b[0] {
			return a[0] < b[0]
		}
		a, b = a[1:], b[1:]
	}
	return len(a) < len(b)
}

func leadingNumber(s string) (int, string) {
	i := 0
	n := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' && i < 9 {
		n = n*10 + int(s[i]-'0')
		i++
	}
	if i == 0 {
		return -1, s
	}
	return n, s[i:]
}
