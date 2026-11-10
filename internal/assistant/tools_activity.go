package assistant

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"ganigeorgiev.com/pocketbase/dbx"
	"ganigeorgiev.com/pocketbase/pocketbase/tools/types"
)

// maxPeriodDays bounds a period so one question cannot scan years of logs.
const maxPeriodDays = 400

// period reads from/to as inclusive local dates and returns the stored-datetime
// bounds [start, end).
func period(s Scope, from, to string) (start, end string, err error) {
	f, err := parseLocalDate(s, "from", from)
	if err != nil {
		return "", "", err
	}
	t, err := parseLocalDate(s, "to", to)
	if err != nil {
		return "", "", err
	}
	if t.Before(f) {
		return "", "", badArgs("to must not be before from")
	}
	if t.Sub(f) > maxPeriodDays*24*time.Hour {
		return "", "", badArgs("the period must be at most %d days", maxPeriodDays)
	}
	return dbTime(f), dbTime(t.AddDate(0, 0, 1)), nil
}

func periodParams() (map[string]any, map[string]any) {
	return map[string]any{"type": "string", "description": "First day, YYYY-MM-DD, in the congregation's time zone."},
		map[string]any{"type": "string", "description": "Last day, inclusive, YYYY-MM-DD. Use the same day as from for a single day."}
}

var activity = Tool{
	Name: "activity",
	Description: "Visits recorded in a period, counted from the status history: reached (done), nobody home, " +
		"do not call, and invalid marks. visits = reached + nobody_home + do_not_call; invalid marks and resets " +
		"are not visits. Group by territory, map, day, week (starting Monday) or month, for trends; or by weekday " +
		"or hour of day. Days, weeks and months with no visits are listed with zeros; the first and last week " +
		"or month may be only partly inside the period. To compare two periods, call it once for each.",
	Params: func() map[string]any {
		from, to := periodParams()
		return object(map[string]any{
			"from":      from,
			"to":        to,
			"group_by":  enumOf("How to break the counts down; none gives one total row.", "none", "territory", "map", "day", "week", "month", "weekday", "hour"),
			"territory": nullable("string", "Exact territory code, or null for every territory."),
			"limit":     limitParam(),
		})
	}(),
	Run: func(s Scope, raw json.RawMessage) (Result, error) {
		var args struct {
			From      string  `json:"from"`
			To        string  `json:"to"`
			GroupBy   string  `json:"group_by"`
			Territory *string `json:"territory"`
			Limit     *int    `json:"limit"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return Result{}, err
		}
		start, end, err := period(s, args.From, args.To)
		if err != nil {
			return Result{}, err
		}
		territoryID, err := territoryIDByCode(s, args.Territory)
		if err != nil {
			return Result{}, err
		}

		var logs []struct {
			Created     string `db:"created"`
			Status      string `db:"new_status"`
			TerritoryID string `db:"territory_id"`
			Territory   string `db:"territory"`
			MapID       string `db:"map_id"`
			Map         string `db:"map"`
		}
		err = s.App.DB().NewQuery(`
			SELECT l.created, l.new_status, COALESCE(t.id, '') AS territory_id, COALESCE(t.code, '') AS territory,
			       COALESCE(m.id, '') AS map_id, COALESCE(m.description, '') AS map
			FROM addresses_log l
			LEFT JOIN territories t ON t.id = l.territory
			LEFT JOIN maps m ON m.id = l.map
			WHERE l.congregation = {:congregation} AND l.created >= {:start} AND l.created < {:end}
			  AND l.new_status IN ('done', 'not_home', 'do_not_call', 'invalid')
			  AND ({:territory} = '' OR l.territory = {:territory})
		`).Bind(dbx.Params{"congregation": s.CongregationID, "start": start, "end": end, "territory": territoryID}).All(&logs)
		if err != nil {
			return Result{}, err
		}

		type bucket struct {
			row                                     map[string]any
			order                                   int
			reached, nobodyHome, doNotCall, invalid int
		}
		// Days, weeks and months form a timeline: every one is listed, in order.
		timeline := args.GroupBy == "day" || args.GroupBy == "week" || args.GroupBy == "month"
		buckets := map[string]*bucket{}
		for _, l := range logs {
			at, err := types.ParseDateTime(l.Created)
			if err != nil || at.IsZero() {
				continue
			}
			local := at.Time().In(s.Location)
			key, row, order := "all", map[string]any{}, 0
			switch args.GroupBy {
			case "territory":
				key, row = l.TerritoryID, map[string]any{"territory_id": l.TerritoryID, "territory": l.Territory}
			case "map":
				key, row = l.MapID, map[string]any{"map_id": l.MapID, "map": l.Map, "territory_id": l.TerritoryID, "territory": l.Territory}
			case "day", "week", "month":
				key, row = timeBucket(args.GroupBy, local)
			case "weekday":
				key, order = local.Weekday().String(), int(local.Weekday())
				row = map[string]any{"weekday": key}
			case "hour":
				key, order = local.Format("15:00"), local.Hour()
				row = map[string]any{"hour": key}
			}
			b := buckets[key]
			if b == nil {
				b = &bucket{row: row, order: order}
				buckets[key] = b
			}
			switch l.Status {
			case "done":
				b.reached++
			case "not_home":
				b.nobodyHome++
			case "do_not_call":
				b.doNotCall++
			case "invalid":
				b.invalid++
			}
		}

		var keyColumns []string
		switch args.GroupBy {
		case "map":
			keyColumns = []string{"territory", "map"}
		case "day":
			keyColumns = []string{"day", "weekday"}
		case "territory", "week", "month", "weekday", "hour":
			keyColumns = []string{args.GroupBy}
		}
		if timeline {
			first, _ := parseLocalDate(s, "from", args.From)
			last, _ := parseLocalDate(s, "to", args.To)
			for day := first; !day.After(last); day = day.AddDate(0, 0, 1) {
				if key, row := timeBucket(args.GroupBy, day); buckets[key] == nil {
					buckets[key] = &bucket{row: row}
				}
			}
		}
		r := Result{Columns: append(keyColumns, "visits", "reached", "nobody_home", "do_not_call", "invalid")}
		ordered := make([]*bucket, 0, len(buckets))
		for _, b := range buckets {
			b.row["reached"], b.row["nobody_home"], b.row["do_not_call"], b.row["invalid"] = b.reached, b.nobodyHome, b.doNotCall, b.invalid
			b.row["visits"] = b.reached + b.nobodyHome + b.doNotCall
			ordered = append(ordered, b)
		}
		sort.SliceStable(ordered, func(i, j int) bool {
			a, b := ordered[i], ordered[j]
			switch args.GroupBy {
			case "day", "week", "month":
				return a.row[args.GroupBy].(string) < b.row[args.GroupBy].(string)
			case "weekday", "hour":
				return a.order < b.order
			}
			return a.row["visits"].(int) > b.row["visits"].(int)
		})
		for _, b := range ordered {
			r.Rows = append(r.Rows, b.row)
		}
		if args.GroupBy == "none" && len(r.Rows) == 0 {
			r.Rows = []map[string]any{{"visits": 0, "reached": 0, "nobody_home": 0, "do_not_call": 0, "invalid": 0}}
		}
		limit := clampLimit(args.Limit)
		if timeline {
			limit = maxPeriodDays // a trend is read whole
		}
		return truncate(r, limit), nil
	},
}

// timeBucket is the day, week or month a local time falls in, as its key and
// the row's opening columns. A week is named by its Monday.
func timeBucket(groupBy string, local time.Time) (string, map[string]any) {
	switch groupBy {
	case "week":
		monday := local.AddDate(0, 0, -((int(local.Weekday()) + 6) % 7))
		key := monday.Format(time.DateOnly)
		return key, map[string]any{"week": key}
	case "month":
		key := local.Format("2006-01")
		return key, map[string]any{"month": key}
	}
	key := local.Format(time.DateOnly)
	return key, map[string]any{"day": key, "weekday": local.Weekday().String()}
}

var whoWorked = Tool{
	Name: "who_worked",
	Description: "Who came out in the ministry in a period, and how often: days_out counts the days a person " +
		"was given a share link or recorded a visit. Use sort days_out for questions like \"who came out most " +
		"often\" or \"most regular publishers\". Names are as typed on each link, and one link may be shared " +
		"by a group, so this is attendance as recorded, not exact.",
	PersonColumns: []string{"name"},
	Params: func() map[string]any {
		from, to := periodParams()
		return object(map[string]any{
			"from":  from,
			"to":    to,
			"sort":  nullableEnum("name (default) lists people alphabetically; days_out puts those who came out most often first.", "name", "days_out"),
			"limit": limitParam(),
		})
	}(),
	Run: func(s Scope, raw json.RawMessage) (Result, error) {
		var args struct {
			From  string  `json:"from"`
			To    string  `json:"to"`
			Sort  *string `json:"sort"`
			Limit *int    `json:"limit"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return Result{}, err
		}
		start, end, err := period(s, args.From, args.To)
		if err != nil {
			return Result{}, err
		}
		params := dbx.Params{"congregation": s.CongregationID, "start": start, "end": end}

		var links []struct {
			Name    string `db:"name"`
			Map     string `db:"map"`
			Created string `db:"created"`
		}
		err = s.App.DB().NewQuery(`
			SELECT TRIM(COALESCE(l.publisher, '')) AS name,
			       COALESCE(m.description, '') AS map, l.created
			FROM assignments_log l LEFT JOIN maps m ON m.id = l.map
			WHERE l.congregation = {:congregation} AND l.action = 'assigned'
			  AND l.created >= {:start} AND l.created < {:end}
		`).Bind(params).All(&links)
		if err != nil {
			return Result{}, err
		}
		var visits []struct {
			Name    string `db:"name"`
			Created string `db:"created"`
		}
		err = s.App.DB().NewQuery(`
			SELECT TRIM(changed_by) AS name, created FROM addresses_log
			WHERE congregation = {:congregation} AND created >= {:start} AND created < {:end}
			  AND new_status IN ` + coverageStatuses + ` AND TRIM(COALESCE(changed_by, '')) != ''
		`).Bind(params).All(&visits)
		if err != nil {
			return Result{}, err
		}

		type person struct {
			name          string
			links, visits int
			maps          []string
			days          map[string]bool
		}
		people := map[string]*person{}
		get := func(name string) *person {
			key := strings.ToLower(name)
			if people[key] == nil {
				people[key] = &person{name: name, days: map[string]bool{}}
			}
			return people[key]
		}
		for _, l := range links {
			if l.Name == "" {
				continue
			}
			p := get(l.Name)
			p.links++
			if day := localDate(s, l.Created); day != "" {
				p.days[day] = true
			}
			if name := resolveLocalized(l.Map, s.Locale); name != "" && len(p.maps) < 5 && !containsFold(p.maps, name) {
				p.maps = append(p.maps, name)
			}
		}
		for _, v := range visits {
			p := get(v.Name)
			p.visits++
			if day := localDate(s, v.Created); day != "" {
				p.days[day] = true
			}
		}

		r := Result{
			Columns: []string{"name", "days_out", "links_given", "visits_recorded", "maps"},
			Note: "days_out counts days with a link given or a visit recorded under that name. visits_recorded " +
				"includes changes saved by admins. Links from the quick-assign button are recorded only from " +
				"November 2026.",
		}
		for _, p := range people {
			r.Rows = append(r.Rows, map[string]any{
				"name": p.name, "days_out": len(p.days), "links_given": p.links, "visits_recorded": p.visits,
				"maps": strings.Join(p.maps, ", "),
			})
		}
		byName := func(i, j int) bool {
			return strings.ToLower(r.Rows[i]["name"].(string)) < strings.ToLower(r.Rows[j]["name"].(string))
		}
		sort.Slice(r.Rows, byName)
		// Ranking is by attendance only, never by visits or homes reached.
		if args.Sort != nil && *args.Sort == "days_out" {
			sort.SliceStable(r.Rows, func(i, j int) bool {
				return r.Rows[i]["days_out"].(int) > r.Rows[j]["days_out"].(int)
			})
		}
		limit := maxLimit
		if args.Limit != nil {
			limit = clampLimit(args.Limit)
		}
		return truncate(r, limit), nil
	},
}

func containsFold(list []string, v string) bool {
	for _, x := range list {
		if strings.EqualFold(x, v) {
			return true
		}
	}
	return false
}
