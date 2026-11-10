package assistant

import (
	"encoding/json"
	"sort"
	"strings"

	"ganigeorgiev.com/pocketbase/dbx"
)

var peopleAndRoles = Tool{
	Name: "people_and_roles",
	Description: "People with access to this congregation in the app and their role: administrator, conductor " +
		"or read_only. Includes when each last signed in and whether their account is disabled.",
	PersonColumns: []string{"name"},
	Params: object(map[string]any{
		"role": nullableEnum("Only this role, or null for everyone.", "administrator", "conductor", "read_only"),
		"name": nullable("string", "Only people whose name contains this text, or null."),
	}),
	Run: func(s Scope, raw json.RawMessage) (Result, error) {
		var args struct {
			Role *string `json:"role"`
			Name *string `json:"name"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return Result{}, err
		}
		role, name := "", ""
		if args.Role != nil {
			role = *args.Role
		}
		if args.Name != nil {
			// A name the admin typed reaches the model as a token; search for the name.
			var err error
			if name, err = unmaskTokens(s, strings.TrimSpace(*args.Name)); err != nil {
				return Result{}, err
			}
		}
		var rows []struct {
			Name      string `db:"name"`
			Role      string `db:"role"`
			LastLogin string `db:"last_login"`
			Disabled  bool   `db:"disabled"`
		}
		err := s.App.DB().NewQuery(`
			SELECT COALESCE(u.name, '') AS name, r.role, COALESCE(u.last_login, '') AS last_login, u.disabled
			FROM roles r JOIN users u ON u.id = r.user
			WHERE r.congregation = {:congregation}
			  AND ({:role} = '' OR r.role = {:role})
			  AND ({:name} = '' OR instr(lower(u.name), lower({:name})) > 0)
			ORDER BY CASE r.role WHEN 'administrator' THEN 0 WHEN 'conductor' THEN 1 ELSE 2 END, lower(u.name)
		`).Bind(dbx.Params{"congregation": s.CongregationID, "role": role, "name": name}).All(&rows)
		if err != nil {
			return Result{}, err
		}
		r := Result{Columns: []string{"name", "role", "last_signed_in", "disabled"}}
		for _, p := range rows {
			r.Rows = append(r.Rows, map[string]any{
				"name": p.Name, "role": p.Role, "last_signed_in": localDate(s, p.LastLogin), "disabled": p.Disabled,
			})
		}
		return truncate(r, maxLimit), nil
	},
}

var householdTypes = Tool{
	Name: "household_types",
	Description: "Counts of homes by household type, the labels this congregation puts on homes (for example a " +
		"language such as Malay or Chinese). Use it for questions like \"which map has the most Malay homes\". " +
		"If the type asked for does not exist, the result lists the congregation's types instead.",
	Params: object(map[string]any{
		"household_type": nullable("string", "A type's code or name, or null for every type."),
		"territory":      nullable("string", "Exact territory code, or null for every territory."),
		"status":         nullableEnum("Only homes with this status, or null for any.", homeStatuses...),
		"group_by":       enumOf("Break the counts down by map, by territory, or only by type.", "map", "territory", "type"),
		"limit":          limitParam(),
	}),
	Run: func(s Scope, raw json.RawMessage) (Result, error) {
		var args struct {
			Type      *string `json:"household_type"`
			Territory *string `json:"territory"`
			Status    *string `json:"status"`
			GroupBy   string  `json:"group_by"`
			Limit     *int    `json:"limit"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return Result{}, err
		}
		territoryID, err := territoryIDByCode(s, args.Territory)
		if err != nil {
			return Result{}, err
		}

		var options []struct {
			ID          string `db:"id"`
			Code        string `db:"code"`
			Description string `db:"description"`
		}
		err = s.App.DB().NewQuery(`SELECT id, code, description FROM options WHERE congregation = {:congregation} ORDER BY sequence`).
			Bind(dbx.Params{"congregation": s.CongregationID}).All(&options)
		if err != nil {
			return Result{}, err
		}
		wanted := map[string]string{} // option id → label
		for _, o := range options {
			label := o.Description
			if label == "" {
				label = o.Code
			}
			if args.Type == nil || eqFold(o.Code, *args.Type) ||
				strings.Contains(strings.ToLower(o.Description), strings.ToLower(strings.TrimSpace(*args.Type))) {
				wanted[o.ID] = label
			}
		}
		if len(wanted) == 0 {
			r := Result{Columns: []string{"code", "household_type"}, Note: "No household type matches; these are the congregation's types."}
			for _, o := range options {
				r.Rows = append(r.Rows, map[string]any{"code": o.Code, "household_type": o.Description})
			}
			return truncate(r, maxLimit), nil
		}

		status := ""
		if args.Status != nil {
			status = *args.Status
		}
		var counts []struct {
			Option      string `db:"option"`
			MapID       string `db:"map_id"`
			Map         string `db:"map"`
			TerritoryID string `db:"territory_id"`
			Territory   string `db:"territory"`
			Homes       int    `db:"homes"`
		}
		err = s.App.DB().NewQuery(`
			SELECT ao.option, m.id AS map_id, m.description AS map, t.id AS territory_id, t.code AS territory, COUNT(*) AS homes
			FROM address_options ao
			JOIN addresses a ON a.id = ao.address
			JOIN maps m ON m.id = a.map
			JOIN territories t ON t.id = a.territory
			WHERE ao.congregation = {:congregation} AND a.congregation = {:congregation}
			  AND ({:territory} = '' OR a.territory = {:territory})
			  AND ({:status} = '' OR a.status = {:status})
			GROUP BY ao.option, m.id
		`).Bind(dbx.Params{"congregation": s.CongregationID, "territory": territoryID, "status": status}).All(&counts)
		if err != nil {
			return Result{}, err
		}

		type agg struct {
			row   map[string]any
			homes int
		}
		groups := map[string]*agg{}
		for _, c := range counts {
			label, ok := wanted[c.Option]
			if !ok {
				continue
			}
			var key string
			var row map[string]any
			switch args.GroupBy {
			case "territory":
				key = c.Option + "|" + c.TerritoryID
				row = map[string]any{"territory_id": c.TerritoryID, "territory": c.Territory, "household_type": label}
			case "type":
				key = c.Option
				row = map[string]any{"household_type": label}
			default:
				key = c.Option + "|" + c.MapID
				row = map[string]any{"map_id": c.MapID, "map": c.Map, "territory_id": c.TerritoryID, "territory": c.Territory, "household_type": label}
			}
			if groups[key] == nil {
				groups[key] = &agg{row: row}
			}
			groups[key].homes += c.Homes
		}

		columns := map[string][]string{
			"territory": {"territory", "household_type", "homes"},
			"type":      {"household_type", "homes"},
		}[args.GroupBy]
		if columns == nil {
			columns = []string{"territory", "map", "household_type", "homes"}
		}
		r := Result{Columns: columns}
		for _, g := range groups {
			g.row["homes"] = g.homes
			r.Rows = append(r.Rows, g.row)
		}
		sort.SliceStable(r.Rows, func(i, j int) bool { return r.Rows[i]["homes"].(int) > r.Rows[j]["homes"].(int) })
		return truncate(r, clampLimit(args.Limit)), nil
	},
}

// DefaultCompletionThreshold is the progress a territory needs to count as
// completed when the congregation has not chosen its own. Below 100 because
// territories rarely reach it: not-homes short of max tries, and homes that
// were missed or can't be reached. handlers uses it too.
const DefaultCompletionThreshold = 90

var congregationSettings = Tool{
	Name: "congregation_settings",
	Description: "The congregation's settings: name, time zone, how many nobody-home tries count a home as " +
		"covered, how long share links last, the completion threshold, and the household types.",
	Params: object(map[string]any{}),
	Run: func(s Scope, _ json.RawMessage) (Result, error) {
		var c struct {
			Name      string `db:"name"`
			Timezone  string `db:"timezone"`
			MaxTries  int    `db:"max_tries"`
			Expiry    int    `db:"expiry_hours"`
			Threshold int    `db:"completion_threshold"`
			Types     string `db:"types"`
		}
		err := s.App.DB().NewQuery(`
			SELECT c.name, COALESCE(c.timezone, '') AS timezone, COALESCE(c.max_tries, 0) AS max_tries,
			       COALESCE(c.expiry_hours, 0) AS expiry_hours, COALESCE(c.completion_threshold, 0) AS completion_threshold,
			       COALESCE((SELECT group_concat(o.code || ' (' || o.description || ')', ', ')
			                 FROM (SELECT code, description FROM options WHERE congregation = c.id ORDER BY sequence) o), '') AS types
			FROM congregations c WHERE c.id = {:congregation}
		`).Bind(dbx.Params{"congregation": s.CongregationID}).One(&c)
		if err != nil {
			return Result{}, err
		}
		if c.Threshold <= 0 {
			c.Threshold = DefaultCompletionThreshold
		}
		return Result{
			Columns: []string{"name", "time_zone", "max_nobody_home_tries", "link_expiry_hours", "completion_threshold", "household_types"},
			Rows: []map[string]any{{
				"name": c.Name, "time_zone": c.Timezone, "max_nobody_home_tries": c.MaxTries,
				"link_expiry_hours": c.Expiry, "completion_threshold": c.Threshold, "household_types": c.Types,
			}},
		}, nil
	},
}
