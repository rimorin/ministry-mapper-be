package assistant

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"ganigeorgiev.com/pocketbase/dbx"
)

// homesNote is what the home counts mean, the same in every function that
// reports them, as the app counts a map's progress (processMapAggregates).
const homesNote = "homes are the homes that count toward progress: a counted household type, and done, not done " +
	"or nobody home. at_try_limit homes are part of not_home and count as covered, so progress is done plus " +
	"at_try_limit out of homes. dnc and invalid homes are apart from homes."

// homeCountsNote adds the two columns only functions counting from the homes
// themselves have.
const homeCountsNote = homesNote + " not_counted homes have only household types that are not counted. all_homes " +
	"is every home: homes, dnc, invalid and not_counted together."

// homeCounts are the count columns, in the order every function lists them.
var homeCounts = []string{"homes", "done", "not_done", "not_home", "at_try_limit", "dnc", "invalid", "not_counted", "all_homes"}

// countedHome is the app's rule for a home that counts toward progress: it has
// a household type that is counted.
const countedHome = `EXISTS (SELECT 1 FROM address_options ao JOIN options o ON o.id = ao.option
	WHERE ao.address = a.id AND ao.map = a.map AND o.is_countable = TRUE)`

// atTryLimit is a nobody-home home that has had every try the congregation allows.
const atTryLimit = `(a.status = 'not_home' AND c.max_tries > 0 AND COALESCE(a.not_home_tries, 0) >= c.max_tries)`

// homeCountColumns counts the homes of a query over addresses a joined to its
// congregation c, in homeCounts order.
const homeCountColumns = `
	COALESCE(SUM(` + countedHome + ` AND a.status IN ('done', 'not_done', 'not_home')), 0) AS homes,
	COALESCE(SUM(` + countedHome + ` AND a.status = 'done'), 0) AS done,
	COALESCE(SUM(` + countedHome + ` AND a.status = 'not_done'), 0) AS not_done,
	COALESCE(SUM(` + countedHome + ` AND a.status = 'not_home'), 0) AS not_home,
	COALESCE(SUM(` + countedHome + ` AND ` + atTryLimit + `), 0) AS at_try_limit,
	COALESCE(SUM(` + countedHome + ` AND a.status = 'do_not_call'), 0) AS dnc,
	COALESCE(SUM(` + countedHome + ` AND a.status = 'invalid'), 0) AS invalid,
	COALESCE(SUM(NOT ` + countedHome + `), 0) AS not_counted,
	COUNT(*) AS all_homes`

type homeCountRow struct {
	Homes      int `db:"homes"`
	Done       int `db:"done"`
	NotDone    int `db:"not_done"`
	NotHome    int `db:"not_home"`
	AtTryLimit int `db:"at_try_limit"`
	DNC        int `db:"dnc"`
	Invalid    int `db:"invalid"`
	NotCounted int `db:"not_counted"`
	AllHomes   int `db:"all_homes"`
}

func (c homeCountRow) into(row map[string]any) map[string]any {
	row["homes"], row["done"], row["not_done"], row["not_home"] = c.Homes, c.Done, c.NotDone, c.NotHome
	row["at_try_limit"], row["dnc"], row["invalid"] = c.AtTryLimit, c.DNC, c.Invalid
	row["not_counted"], row["all_homes"] = c.NotCounted, c.AllHomes
	return row
}

var homesTool = Tool{
	Name: "homes",
	Description: "Homes, filtered any way and either listed or counted. Use it for every question about homes " +
		"themselves: which units of a map are not done, which floors are left, one home's status, homes where " +
		"nobody was home or at the try limit, do-not-call homes and how long ago each was marked, homes of a " +
		"household type such as a language, homes with notes, homes that do not count toward progress. Every " +
		"filter is optional and they combine. group_by home lists each home; territory, map, floor or " +
		"household_type counts them by that; total gives one row of counts.",
	Params: object(map[string]any{
		"territory":  nullable("string", "Exact territory code, or null for every territory."),
		"map":        nullable("string", "Exact map name, or null for every map. Use find_places first if unsure."),
		"floor_from": nullable("integer", "With a building: the lowest floor to include, or null."),
		"floor_to":   nullable("integer", "With a building: the highest floor to include, or null."),
		"unit":       nullable("string", "One unit or house number, such as 123 or 12A, or null."),
		"status": map[string]any{
			"type":        []string{"array", "null"},
			"items":       map[string]any{"type": "string", "enum": homeStatuses},
			"description": "Only homes with one of these statuses, or null for any. not_home is nobody home.",
		},
		"household_type":      nullable("string", "Only homes of this household type: its code or part of its name, such as Malay."),
		"at_try_limit":        nullable("boolean", "True for only nobody-home homes at the try limit, false for only those below it."),
		"has_notes":           nullable("boolean", "True for only homes with a note, false for only those without."),
		"counted":             nullable("boolean", "False for only homes that do not count toward progress, true for only those that do."),
		"dnc_older_than_days": nullable("integer", "Only do-not-call homes marked at least this many days ago."),
		"group_by": enumOf("home to list each home, the others to count.",
			"home", "floor", "map", "territory", "household_type", "total"),
		"limit": limitParam(),
	}),
	Run: func(s Scope, raw json.RawMessage) (Result, error) {
		var args struct {
			Territory    *string  `json:"territory"`
			Map          *string  `json:"map"`
			FloorFrom    *int     `json:"floor_from"`
			FloorTo      *int     `json:"floor_to"`
			Unit         *string  `json:"unit"`
			Status       []string `json:"status"`
			Type         *string  `json:"household_type"`
			AtTryLimit   *bool    `json:"at_try_limit"`
			HasNotes     *bool    `json:"has_notes"`
			Counted      *bool    `json:"counted"`
			DncOlderThan *int     `json:"dnc_older_than_days"`
			GroupBy      string   `json:"group_by"`
			Limit        *int     `json:"limit"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return Result{}, err
		}
		if err := validDays("dnc_older_than_days", args.DncOlderThan); err != nil {
			return Result{}, err
		}
		mapID, territoryID, err := placeScope(s, args.Map, args.Territory)
		if err != nil {
			return Result{}, err
		}
		if args.GroupBy == "floor" && mapID == "" {
			return Result{}, badArgs("group_by floor needs one map; name the map")
		}
		options, err := householdOptions(s)
		if err != nil {
			return Result{}, err
		}

		where := []string{"a.congregation = {:congregation}"}
		params := dbx.Params{"congregation": s.CongregationID}
		add := func(condition string, values dbx.Params) {
			where = append(where, condition)
			for k, v := range values {
				params[k] = v
			}
		}
		flag := func(want *bool, condition string) {
			if want == nil {
				return
			}
			if !*want {
				condition = "NOT " + condition
			}
			add(condition, nil)
		}
		if territoryID != "" {
			add("a.territory = {:territory}", dbx.Params{"territory": territoryID})
		}
		if mapID != "" {
			add("a.map = {:map}", dbx.Params{"map": mapID})
		}
		if args.FloorFrom != nil {
			add("COALESCE(a.floor, 0) >= {:floor_from}", dbx.Params{"floor_from": *args.FloorFrom})
		}
		if args.FloorTo != nil {
			add("COALESCE(a.floor, 0) <= {:floor_to}", dbx.Params{"floor_to": *args.FloorTo})
		}
		if args.Unit != nil && strings.TrimSpace(*args.Unit) != "" {
			add("lower(a.code) = lower({:unit})", dbx.Params{"unit": strings.TrimSpace(*args.Unit)})
		}
		if len(args.Status) > 0 {
			names := make([]string, len(args.Status))
			for i, status := range args.Status {
				names[i] = fmt.Sprintf("{:status%d}", i)
				params[fmt.Sprintf("status%d", i)] = status
			}
			add("a.status IN ("+strings.Join(names, ", ")+")", nil)
		}
		// The household types asked for, or all of them: grouping by type
		// counts only these.
		wanted := options
		if args.Type != nil && strings.TrimSpace(*args.Type) != "" {
			wanted = matchingOptions(options, *args.Type)
			if len(wanted) == 0 {
				return Result{}, badArgs("no household type matches %q; the congregation's types are %s",
					*args.Type, optionLabels(options))
			}
			names := make([]string, len(wanted))
			for i, o := range wanted {
				names[i] = fmt.Sprintf("{:type%d}", i)
				params[fmt.Sprintf("type%d", i)] = o.ID
			}
			add("EXISTS (SELECT 1 FROM address_options f WHERE f.address = a.id AND f.option IN ("+
				strings.Join(names, ", ")+"))", nil)
		}
		flag(args.AtTryLimit, atTryLimit)
		flag(args.HasNotes, "TRIM(COALESCE(a.notes, '')) != ''")
		flag(args.Counted, countedHome)
		if args.DncOlderThan != nil {
			// A home with no date was marked before dates were recorded.
			add("a.status = 'do_not_call' AND COALESCE(a.dnc_time, '') < {:dnc_before}",
				dbx.Params{"dnc_before": daysAgo(s, *args.DncOlderThan)})
		}
		from := ` FROM addresses a JOIN maps m ON m.id = a.map JOIN territories t ON t.id = a.territory
			JOIN congregations c ON c.id = a.congregation`
		filter := " WHERE " + strings.Join(where, " AND ")
		limit := clampLimit(args.Limit)

		if args.GroupBy == "home" {
			return listHomes(s, from+filter, params, options, limit, len(args.Status) == 1 && args.Status[0] == "do_not_call")
		}

		// What each row is counted by: the columns that name it and how it joins.
		var keys, groupBy, join string
		var columns []string
		switch args.GroupBy {
		case "territory":
			keys, groupBy = "t.id AS territory_id, t.code AS territory", "t.id"
			columns = []string{"territory"}
		case "map":
			keys, groupBy = "t.id AS territory_id, t.code AS territory, m.id AS map_id, m.description AS map", "m.id"
			columns = []string{"territory", "map"}
		case "floor":
			keys, groupBy = "COALESCE(a.floor, 0) AS floor", "COALESCE(a.floor, 0)"
			columns = []string{"floor"}
		case "household_type":
			keys, groupBy = "g.option AS option", "g.option"
			join = " JOIN address_options g ON g.address = a.id"
			columns = []string{"household_type"}
		case "total":
			keys = "'' AS territory"
		default:
			return Result{}, badArgs("unknown group_by %q", args.GroupBy)
		}
		query := "SELECT " + keys + "," + homeCountColumns + from + join + filter
		if groupBy != "" {
			query += " GROUP BY " + groupBy
		}
		var rows []struct {
			homeCountRow
			TerritoryID string `db:"territory_id"`
			Territory   string `db:"territory"`
			MapID       string `db:"map_id"`
			Map         string `db:"map"`
			Floor       int    `db:"floor"`
			Option      string `db:"option"`
		}
		if err := s.App.DB().NewQuery(query).Bind(params).All(&rows); err != nil {
			return Result{}, err
		}
		labels := map[string]string{}
		for _, o := range wanted {
			labels[o.ID] = o.label(s.Locale)
		}
		r := Result{Columns: append(columns, homeCounts...), Note: homeCountsNote}
		if args.GroupBy == "household_type" {
			r.Note += " A home with several household types is counted under each."
		}
		for _, row := range rows {
			out := map[string]any{}
			switch args.GroupBy {
			case "territory":
				out["territory_id"], out["territory"] = row.TerritoryID, row.Territory
			case "map":
				out["territory_id"], out["territory"], out["map_id"], out["map"] = row.TerritoryID, row.Territory, row.MapID, row.Map
			case "floor":
				out["floor"] = row.Floor
			case "household_type":
				label, ok := labels[row.Option]
				if !ok {
					continue
				}
				out["household_type"] = label
			}
			r.Rows = append(r.Rows, row.into(out))
		}
		if args.GroupBy == "floor" {
			sort.SliceStable(r.Rows, func(i, j int) bool { return r.Rows[i]["floor"].(int) < r.Rows[j]["floor"].(int) })
		} else {
			sort.SliceStable(r.Rows, func(i, j int) bool { return r.Rows[i]["all_homes"].(int) > r.Rows[j]["all_homes"].(int) })
		}
		return truncate(r, limit), nil
	},
}

// listHomes is the homes themselves, in visiting order, or oldest first when
// only do-not-call homes are asked for.
func listHomes(s Scope, fromWhere string, params dbx.Params, options []householdOption, limit int, oldestFirst bool) (Result, error) {
	var total int
	if err := s.App.DB().NewQuery("SELECT COUNT(*)" + fromWhere).Bind(params).Row(&total); err != nil {
		return Result{}, err
	}
	order := " ORDER BY t.code, m.description, a.floor, a.sequence"
	if oldestFirst {
		order = " ORDER BY COALESCE(a.dnc_time, ''), t.code, m.description, a.floor, a.sequence"
	}
	var rows []struct {
		TerritoryID string `db:"territory_id"`
		Territory   string `db:"territory"`
		MapID       string `db:"map_id"`
		Map         string `db:"map"`
		MapType     string `db:"map_type"`
		Floor       int    `db:"floor"`
		Code        string `db:"code"`
		Status      string `db:"status"`
		Tries       int    `db:"tries"`
		AtTryLimit  bool   `db:"at_try_limit"`
		Counted     bool   `db:"counted"`
		HasNotes    bool   `db:"has_notes"`
		Marked      string `db:"marked"`
		Types       string `db:"types"`
		MaxTries    int    `db:"max_tries"`
	}
	params["limit"] = limit
	err := s.App.DB().NewQuery(`
		SELECT t.id AS territory_id, t.code AS territory, m.id AS map_id, m.description AS map,
		       COALESCE(m.type, '') AS map_type, COALESCE(a.floor, 0) AS floor, a.code,
		       COALESCE(a.status, '') AS status, COALESCE(a.not_home_tries, 0) AS tries,
		       ` + atTryLimit + ` AS at_try_limit, ` + countedHome + ` AS counted,
		       TRIM(COALESCE(a.notes, '')) != '' AS has_notes, COALESCE(a.dnc_time, '') AS marked,
		       COALESCE((SELECT group_concat(g.option) FROM address_options g WHERE g.address = a.id), '') AS types,
		       COALESCE(c.max_tries, 0) AS max_tries` + fromWhere + order + ` LIMIT {:limit}`).Bind(params).All(&rows)
	if err != nil {
		return Result{}, err
	}
	labels := map[string]string{}
	for _, o := range options {
		labels[o.ID] = o.label(s.Locale)
	}
	r := Result{
		Columns: []string{"territory", "map", "home", "status", "tries", "at_try_limit", "household_types", "counted", "has_notes", "marked"},
		Total:   total,
		Note: "counted says whether the home counts toward progress. marked is when a do-not-call home was marked; " +
			"one with no date was marked before dates were recorded.",
	}
	for _, h := range rows {
		var types []string
		for _, id := range strings.Split(h.Types, ",") {
			if label := labels[id]; label != "" {
				types = append(types, label)
			}
		}
		marked := ""
		if h.Status == "do_not_call" {
			marked = localDate(s, h.Marked)
		}
		r.Rows = append(r.Rows, map[string]any{
			"territory_id": h.TerritoryID, "territory": h.Territory, "map_id": h.MapID, "map": h.Map,
			"home": home(h.MapType, h.Floor, h.Code), "status": h.Status, "tries": h.Tries,
			"at_try_limit": h.AtTryLimit, "household_types": strings.Join(types, ", "),
			"counted": h.Counted, "has_notes": h.HasNotes, "marked": marked,
		})
	}
	if len(rows) > 0 && rows[0].MaxTries > 0 {
		r.Note += fmt.Sprintf(" The try limit is %d.", rows[0].MaxTries)
	}
	return r, nil
}

// householdOption is one household type the congregation uses.
type householdOption struct {
	ID          string `db:"id"`
	Code        string `db:"code"`
	Description string `db:"description"`
}

func (o householdOption) label(locale string) string {
	if o.Description == "" {
		return o.Code
	}
	return resolveLocalized(o.Description, locale)
}

func householdOptions(s Scope) ([]householdOption, error) {
	var options []householdOption
	err := s.App.DB().NewQuery(`SELECT id, code, COALESCE(description, '') AS description FROM options
		WHERE congregation = {:congregation} ORDER BY sequence`).
		Bind(dbx.Params{"congregation": s.CongregationID}).All(&options)
	return options, err
}

// matchingOptions are the household types a word names: by code, or by part
// of the name.
func matchingOptions(options []householdOption, want string) []householdOption {
	want = strings.ToLower(strings.TrimSpace(want))
	var out []householdOption
	for _, o := range options {
		if strings.ToLower(o.Code) == want || strings.Contains(strings.ToLower(o.Description), want) {
			out = append(out, o)
		}
	}
	return out
}

func optionLabels(options []householdOption) string {
	labels := make([]string, len(options))
	for i, o := range options {
		labels[i] = o.label("en")
	}
	return strings.Join(labels, ", ")
}
