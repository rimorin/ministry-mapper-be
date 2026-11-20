package assistant

import (
	"encoding/json"
	"strings"

	"ganigeorgiev.com/pocketbase/dbx"

	"ministry-mapper/internal/authz"
)

var peopleAndRoles = Tool{
	Name: "people_and_roles",
	Description: "People with access to this congregation in the app and their role: administrator, conductor " +
		"or read_only. Includes when each last signed in and whether their account is disabled.",
	PersonColumns: []string{"name"},
	Params: object(map[string]any{
		"role": nullableEnum("Only this role, or null for everyone.", authz.StaffRoles...),
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
			WHERE r.congregation = {:congregation} AND r.role IN ` + staffRoleSQL + `
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

var members = Tool{
	Name: "members",
	Description: "The members who can be given a map on their own account, publishers and staff alike, with how " +
		"many open links each holds now and on which maps. Use it for who has no map yet, who is on which map, or " +
		"to give the rest a map: pass each person's token to the link function as it is. It lists nobody where " +
		"links are not given to accounts.",
	PersonColumns: []string{"name"},
	Params: object(map[string]any{
		"without_link": nullable("boolean", "True for only members holding no open link."),
		"territory":    nullable("string", "Count only links in this territory code, or null for every territory."),
	}),
	Run: func(s Scope, raw json.RawMessage) (Result, error) {
		var args struct {
			WithoutLink *bool   `json:"without_link"`
			Territory   *string `json:"territory"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return Result{}, err
		}
		r := Result{Columns: []string{"name", "role", "open_links", "maps"}}
		if !s.MemberLinks {
			r.Note = "This congregation does not give links to members' accounts."
			return r, nil
		}
		territoryID, err := territoryIDByCode(s, args.Territory)
		if err != nil {
			return Result{}, err
		}
		// The same accounts a link's names are matched to (see matchMembers).
		var people []struct {
			ID   string `db:"id"`
			Name string `db:"name"`
			Role string `db:"role"`
		}
		err = s.App.DB().NewQuery(`
			SELECT u.id, u.name, MIN(r.role) AS role FROM roles r JOIN users u ON u.id = r.user
			WHERE r.congregation = {:congregation} AND u.disabled = FALSE AND TRIM(COALESCE(u.name, '')) != ''
			GROUP BY u.id ORDER BY lower(u.name), u.id
		`).Bind(dbx.Params{"congregation": s.CongregationID}).All(&people)
		if err != nil {
			return Result{}, err
		}
		var held []struct {
			User string `db:"user"`
			Map  string `db:"map"`
		}
		err = s.App.DB().NewQuery(`
			SELECT j.value AS user, COALESCE(m.description, '') AS map
			FROM assignments a JOIN json_each(a.assignees) j JOIN maps m ON m.id = a.map
			WHERE a.congregation = {:congregation} AND a.expiry_date > {:now} AND json_valid(a.assignees)
			  AND ({:territory} = '' OR m.territory = {:territory})
			ORDER BY a.created
		`).Bind(dbx.Params{"congregation": s.CongregationID, "now": dbTime(s.Now), "territory": territoryID}).All(&held)
		if err != nil {
			return Result{}, err
		}
		maps := map[string][]string{}
		for _, h := range held {
			maps[h.User] = append(maps[h.User], resolveLocalized(h.Map, s.Locale))
		}
		for _, p := range people {
			links := maps[p.ID]
			if args.WithoutLink != nil && *args.WithoutLink && len(links) > 0 {
				continue
			}
			r.Rows = append(r.Rows, map[string]any{
				"name": p.Name, "role": p.Role, "open_links": len(links), "maps": strings.Join(links, ", "),
			})
		}
		return truncate(r, maxLimit), nil
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
