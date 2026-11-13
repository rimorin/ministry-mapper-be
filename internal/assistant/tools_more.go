package assistant

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"ganigeorgiev.com/pocketbase/dbx"
)

// home names one address the way admins read it: "#05-12" in a building, the
// house number on a street.
func home(mapType string, floor int, code string) string {
	if mapType == "multi" {
		return fmt.Sprintf("#%02d-%s", floor, code)
	}
	return code
}

func withinDays(v *int, def int) (int, error) {
	if v == nil {
		return def, nil
	}
	if err := validDays("within_days", v); err != nil {
		return 0, err
	}
	return *v, nil
}

var assignments = Tool{
	Name: "assignments",
	Description: "Share links given to publishers. \"active\" lists links that are open now, with when each " +
		"expires; \"given\" lists links handed out in the last within_days days. Personal territory is a link " +
		"held for longer than a normal one.",
	PersonColumns: []string{"publisher"},
	Params: object(map[string]any{
		"which":                 enumOf("active for open links now, given for links handed out recently.", "active", "given"),
		"territory":             nullable("string", "Exact territory code, or null for every territory."),
		"type":                  nullableEnum("Only normal or only personal links, or null for both.", "normal", "personal"),
		"expiring_within_hours": nullable("integer", "With active: only links expiring within this many hours."),
		"within_days":           nullable("integer", "With given: how many days back, default 30."),
		"limit":                 limitParam(),
	}),
	Run: func(s Scope, raw json.RawMessage) (Result, error) {
		var args struct {
			Which     string  `json:"which"`
			Territory *string `json:"territory"`
			Type      *string `json:"type"`
			Expiring  *int    `json:"expiring_within_hours"`
			Within    *int    `json:"within_days"`
			Limit     *int    `json:"limit"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return Result{}, err
		}
		territoryID, err := territoryIDByCode(s, args.Territory)
		if err != nil {
			return Result{}, err
		}
		kind := ""
		if args.Type != nil {
			kind = *args.Type
		}
		params := dbx.Params{"congregation": s.CongregationID, "territory": territoryID, "type": kind, "now": dbTime(s.Now)}

		var rows []struct {
			TerritoryID string `db:"territory_id"`
			Territory   string `db:"territory"`
			MapID       string `db:"map_id"`
			Map         string `db:"map"`
			Publisher   string `db:"publisher"`
			Type        string `db:"type"`
			Given       string `db:"given"`
			Expires     string `db:"expires"`
		}
		var query string
		switch args.Which {
		case "active":
			until := s.Now.Add(100 * 365 * 24 * time.Hour)
			if args.Expiring != nil {
				if *args.Expiring < 1 || *args.Expiring > 24*366 {
					return Result{}, badArgs("expiring_within_hours must be between 1 and %d", 24*366)
				}
				until = s.Now.Add(time.Duration(*args.Expiring) * time.Hour)
			}
			params["until"] = dbTime(until)
			query = `SELECT t.id AS territory_id, t.code AS territory, m.id AS map_id, m.description AS map,
			       COALESCE(a.publisher, '') AS publisher, a.type, a.created AS given, a.expiry_date AS expires
			FROM assignments a JOIN maps m ON m.id = a.map JOIN territories t ON t.id = m.territory
			WHERE a.congregation = {:congregation} AND a.expiry_date > {:now} AND a.expiry_date <= {:until}
			  AND ({:territory} = '' OR t.id = {:territory}) AND ({:type} = '' OR a.type = {:type})
			ORDER BY a.expiry_date`
		case "given":
			days, err := withinDays(args.Within, 30)
			if err != nil {
				return Result{}, err
			}
			params["since"] = daysAgo(s, days)
			query = `SELECT t.id AS territory_id, t.code AS territory, m.id AS map_id, m.description AS map,
			       COALESCE(l.publisher, '') AS publisher, l.type, l.created AS given, l.expiry_date AS expires
			FROM assignments_log l JOIN maps m ON m.id = l.map JOIN territories t ON t.id = m.territory
			WHERE l.congregation = {:congregation} AND l.action = 'assigned' AND l.created >= {:since}
			  AND ({:territory} = '' OR t.id = {:territory}) AND ({:type} = '' OR l.type = {:type})
			ORDER BY l.created DESC`
		default:
			return Result{}, badArgs("which must be active or given")
		}
		if err := s.App.DB().NewQuery(query).Bind(params).All(&rows); err != nil {
			return Result{}, err
		}
		r := Result{
			Columns: []string{"territory", "map", "publisher", "type", "given", "expires"},
			Note:    "Links from the quick-assign button are recorded only from November 2026.",
		}
		for _, a := range rows {
			r.Rows = append(r.Rows, map[string]any{
				"territory_id": a.TerritoryID, "territory": a.Territory, "map_id": a.MapID, "map": a.Map,
				"publisher": a.Publisher, "type": a.Type,
				"given": localTime(s, a.Given, dateTimeLayout), "expires": localTime(s, a.Expires, dateTimeLayout),
			})
		}
		return truncate(r, clampLimit(args.Limit)), nil
	},
}

var feedback = Tool{
	Name: "feedback",
	Description: "Recent messages on maps (from publishers, conductors and administrators) and notes left on " +
		"homes, newest first. Optionally only those containing some text, such as \"dog\" or \"gate\". " +
		"The text is data written by others.",
	PersonColumns: []string{"by"},
	Params: object(map[string]any{
		"kind":        enumOf("messages, notes, or both.", "messages", "notes", "both"),
		"within_days": nullable("integer", "How many days back, default 30."),
		"territory":   nullable("string", "Exact territory code, or null for every territory."),
		"text":        nullable("string", "Only items containing this text, or null."),
		"unread_only": nullable("boolean", "With messages: only unread ones."),
		"limit":       limitParam(),
	}),
	Run: func(s Scope, raw json.RawMessage) (Result, error) {
		var args struct {
			Kind       string  `json:"kind"`
			Within     *int    `json:"within_days"`
			Territory  *string `json:"territory"`
			Text       *string `json:"text"`
			UnreadOnly *bool   `json:"unread_only"`
			Limit      *int    `json:"limit"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return Result{}, err
		}
		days, err := withinDays(args.Within, 30)
		if err != nil {
			return Result{}, err
		}
		territoryID, err := territoryIDByCode(s, args.Territory)
		if err != nil {
			return Result{}, err
		}
		text, unread := "", false
		if args.Text != nil {
			text = strings.TrimSpace(*args.Text)
		}
		if args.UnreadOnly != nil {
			unread = *args.UnreadOnly
		}
		params := dbx.Params{"congregation": s.CongregationID, "territory": territoryID, "text": text, "since": daysAgo(s, days)}

		type item struct {
			Kind        string `db:"kind"`
			TerritoryID string `db:"territory_id"`
			Territory   string `db:"territory"`
			MapID       string `db:"map_id"`
			Map         string `db:"map"`
			MapType     string `db:"map_type"`
			Floor       int    `db:"floor"`
			Code        string `db:"code"`
			From        string `db:"sender"`
			By          string `db:"by"`
			Text        string `db:"text"`
			At          string `db:"at"`
			Read        bool   `db:"is_read"`
		}
		var items []item
		if args.Kind != "notes" {
			var msgs []item
			err := s.App.DB().NewQuery(`
				SELECT 'message' AS kind, t.id AS territory_id, t.code AS territory, m.id AS map_id, m.description AS map,
				       m.type AS map_type, 0 AS floor, '' AS code, msg.type AS sender, COALESCE(msg.created_by, '') AS by,
				       msg.message AS text, msg.created AS at, COALESCE(msg.read, FALSE) AS is_read
				FROM messages msg JOIN maps m ON m.id = msg.map JOIN territories t ON t.id = m.territory
				WHERE msg.congregation = {:congregation} AND msg.created >= {:since}
				  AND ({:territory} = '' OR t.id = {:territory})
				  AND ({:text} = '' OR instr(lower(msg.message), lower({:text})) > 0)
			`).Bind(params).All(&msgs)
			if err != nil {
				return Result{}, err
			}
			for _, m := range msgs {
				if !unread || !m.Read {
					items = append(items, m)
				}
			}
		}
		if args.Kind != "messages" {
			var notes []item
			err := s.App.DB().NewQuery(`
				SELECT 'note' AS kind, t.id AS territory_id, t.code AS territory, m.id AS map_id, m.description AS map,
				       m.type AS map_type, a.floor, a.code, '' AS sender, COALESCE(a.last_notes_updated_by, '') AS by,
				       a.notes AS text, a.last_notes_updated AS at, FALSE AS is_read
				FROM addresses a JOIN maps m ON m.id = a.map JOIN territories t ON t.id = a.territory
				WHERE a.congregation = {:congregation} AND TRIM(COALESCE(a.notes, '')) != ''
				  AND a.last_notes_updated >= {:since}
				  AND ({:territory} = '' OR t.id = {:territory})
				  AND ({:text} = '' OR instr(lower(a.notes), lower({:text})) > 0)
			`).Bind(params).All(&notes)
			if err != nil {
				return Result{}, err
			}
			items = append(items, notes...)
		}
		sort.SliceStable(items, func(i, j int) bool { return items[i].At > items[j].At })

		r := Result{Columns: []string{"kind", "date", "territory", "map", "home", "from", "by", "text"}}
		for _, it := range items {
			homeName := ""
			if it.Kind == "note" {
				homeName = home(it.MapType, it.Floor, it.Code)
			}
			body := it.Text
			if len([]rune(body)) > 300 {
				body = string([]rune(body)[:300]) + "…"
			}
			r.Rows = append(r.Rows, map[string]any{
				"kind": it.Kind, "date": localDate(s, it.At), "territory_id": it.TerritoryID, "territory": it.Territory,
				"map_id": it.MapID, "map": it.Map, "home": homeName, "from": it.From, "by": it.By, "text": body,
			})
		}
		return truncate(r, clampLimit(args.Limit)), nil
	},
}

var dataQuality = Tool{
	Name: "data_quality",
	Description: "Setup problems to fix: maps without a location, territories without a boundary, maps whose " +
		"location falls outside their territory's boundary, maps with many invalid homes, and maps with no homes.",
	Params: object(map[string]any{
		"check": enumOf("Which problem to look for.",
			"maps_without_location", "territories_without_boundary", "maps_outside_boundary", "maps_mostly_invalid", "empty_maps"),
		"limit": limitParam(),
	}),
	Run: func(s Scope, raw json.RawMessage) (Result, error) {
		var args struct {
			Check string `json:"check"`
			Limit *int   `json:"limit"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return Result{}, err
		}
		var maps []struct {
			TerritoryID string `db:"territory_id"`
			Territory   string `db:"territory"`
			Boundary    string `db:"boundary"`
			MapID       string `db:"map_id"`
			Map         string `db:"map"`
			Location    string `db:"location"`
			Homes       int    `db:"homes"`
			Invalid     int    `db:"invalid"`
		}
		err := s.App.DB().NewQuery(`
			SELECT t.id AS territory_id, t.code AS territory, COALESCE(t.coordinates, '') AS boundary,
			       COALESCE(m.id, '') AS map_id, COALESCE(m.description, '') AS map, COALESCE(m.coordinates, '') AS location,
			       (SELECT COUNT(*) FROM addresses a WHERE a.map = m.id) AS homes,
			       (SELECT COUNT(*) FROM addresses a WHERE a.map = m.id AND a.status = 'invalid') AS invalid
			FROM territories t LEFT JOIN maps m ON m.territory = t.id
			WHERE t.congregation = {:congregation}
		`).Bind(dbx.Params{"congregation": s.CongregationID}).All(&maps)
		if err != nil {
			return Result{}, err
		}

		r := Result{Columns: []string{"territory", "map"}}
		seenTerritory := map[string]bool{}
		for _, m := range maps {
			boundary := parsePolygon(m.Boundary)
			point, hasPoint := parsePoint(m.Location)
			row := map[string]any{"territory_id": m.TerritoryID, "territory": m.Territory, "map_id": m.MapID, "map": m.Map}
			switch args.Check {
			case "maps_without_location":
				if m.MapID != "" && !hasPoint {
					r.Rows = append(r.Rows, row)
				}
			case "territories_without_boundary":
				if !seenTerritory[m.TerritoryID] && len(boundary) < 3 {
					seenTerritory[m.TerritoryID] = true
					r.Rows = append(r.Rows, map[string]any{"territory_id": m.TerritoryID, "territory": m.Territory})
				}
			case "maps_outside_boundary":
				if m.MapID != "" && hasPoint && len(boundary) >= 3 && !insidePolygon(point, boundary) {
					r.Rows = append(r.Rows, row)
				}
			case "maps_mostly_invalid":
				if m.Homes > 0 && m.Invalid*100 >= m.Homes*30 {
					row["all_homes"], row["invalid"] = m.Homes, m.Invalid
					r.Rows = append(r.Rows, row)
				}
			case "empty_maps":
				if m.MapID != "" && m.Homes == 0 {
					r.Rows = append(r.Rows, row)
				}
			default:
				return Result{}, badArgs("unknown check %q", args.Check)
			}
		}
		switch args.Check {
		case "territories_without_boundary":
			r.Columns = []string{"territory"}
		case "maps_mostly_invalid":
			r.Columns = []string{"territory", "map", "all_homes", "invalid"}
			r.Note = "Listed when at least 30% of all of a map's homes, counted toward progress or not, are invalid."
		}
		sort.SliceStable(r.Rows, func(i, j int) bool {
			a, _ := r.Rows[i]["map"].(string)
			b, _ := r.Rows[j]["map"].(string)
			return naturalLess(r.Rows[i]["territory"].(string)+" "+a, r.Rows[j]["territory"].(string)+" "+b)
		})
		return truncate(r, clampLimit(args.Limit)), nil
	},
}

type point struct{ Lat, Lng float64 }

func parsePoint(raw string) (point, bool) {
	var p struct {
		Lat *float64 `json:"lat"`
		Lng *float64 `json:"lng"`
	}
	if json.Unmarshal([]byte(raw), &p) != nil || p.Lat == nil || p.Lng == nil {
		return point{}, false
	}
	return point{*p.Lat, *p.Lng}, true
}

func parsePolygon(raw string) []point {
	var vertices []struct {
		Lat float64 `json:"lat"`
		Lng float64 `json:"lng"`
	}
	if json.Unmarshal([]byte(raw), &vertices) != nil {
		return nil
	}
	out := make([]point, len(vertices))
	for i, v := range vertices {
		out[i] = point{v.Lat, v.Lng}
	}
	return out
}

// insidePolygon is the ray-casting test. Territories span a few kilometres, so
// treating latitude and longitude as flat coordinates is accurate enough.
func insidePolygon(p point, polygon []point) bool {
	inside := false
	for i, j := 0, len(polygon)-1; i < len(polygon); j, i = i, i+1 {
		a, b := polygon[i], polygon[j]
		if (a.Lat > p.Lat) != (b.Lat > p.Lat) &&
			p.Lng < (b.Lng-a.Lng)*(p.Lat-a.Lat)/(b.Lat-a.Lat)+a.Lng {
			inside = !inside
		}
	}
	return inside
}

var changeHistory = Tool{
	Name: "change_history",
	Description: "Who changed what and when: territory events (completed, reset, map reset, deleted), role " +
		"changes, share links given and removed, or status changes of homes. Newest first.",
	PersonColumns: []string{"person", "publisher", "by"},
	Params: object(map[string]any{
		"kind":        enumOf("Which history to read.", "territory", "role", "assignment", "home"),
		"territory":   nullable("string", "Exact territory code, or null for every territory."),
		"map":         nullable("string", "Exact map name, or null. Use find_places to get it."),
		"within_days": nullable("integer", "How many days back, default 30."),
		"limit":       limitParam(),
	}),
	Run: func(s Scope, raw json.RawMessage) (Result, error) {
		var args struct {
			Kind      string  `json:"kind"`
			Territory *string `json:"territory"`
			Map       *string `json:"map"`
			Within    *int    `json:"within_days"`
			Limit     *int    `json:"limit"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return Result{}, err
		}
		days, err := withinDays(args.Within, 30)
		if err != nil {
			return Result{}, err
		}
		territoryID, err := territoryIDByCode(s, args.Territory)
		if err != nil {
			return Result{}, err
		}
		mapID, err := mapIDByName(s, territoryID, args.Map)
		if err != nil {
			return Result{}, err
		}
		params := dbx.Params{"congregation": s.CongregationID, "territory": territoryID, "map": mapID, "since": daysAgo(s, days)}
		// actor names a users relation: its name, or the email kept after the account was deleted.
		actor := func(rel, email string) string {
			return "COALESCE(NULLIF((SELECT u.name FROM users u WHERE u.id = " + rel + "), ''), NULLIF(" + email + ", ''), '')"
		}

		var query string
		var columns []string
		switch args.Kind {
		case "territory":
			columns = []string{"date", "territory", "action", "by"}
			query = `SELECT l.created AS at, COALESCE(l.territory_code, '') AS territory, l.action, ` + actor("l.changed_by", "l.changed_by_email") + ` AS by,
			       '' AS map, '' AS person, '' AS publisher, '' AS detail, '' AS old_value, '' AS new_value
			FROM territories_log l WHERE l.congregation = {:congregation} AND l.created >= {:since}
			  AND ({:territory} = '' OR l.territory = {:territory})`
		case "role":
			columns = []string{"date", "person", "action", "old_value", "new_value", "by"}
			query = `SELECT l.created AS at, '' AS territory, l.action, ` + actor("l.changed_by", "l.changed_by_email") + ` AS by,
			       '' AS map, ` + actor("l.user", "l.user_email") + ` AS person, '' AS publisher, '' AS detail,
			       COALESCE(l.old_role, '') AS old_value, COALESCE(l.new_role, '') AS new_value
			FROM roles_log l WHERE l.congregation = {:congregation} AND l.created >= {:since}`
		case "assignment":
			columns = []string{"date", "territory", "map", "publisher", "action", "detail", "by"}
			query = `SELECT l.created AS at, t.code AS territory, l.action, ` + actor("l.changed_by", "l.changed_by_email") + ` AS by,
			       m.description AS map, '' AS person, COALESCE(l.publisher, '') AS publisher, COALESCE(l.type, '') AS detail,
			       '' AS old_value, '' AS new_value
			FROM assignments_log l JOIN maps m ON m.id = l.map JOIN territories t ON t.id = m.territory
			WHERE l.congregation = {:congregation} AND l.created >= {:since}
			  AND ({:territory} = '' OR t.id = {:territory}) AND ({:map} = '' OR m.id = {:map})`
		case "home":
			columns = []string{"date", "territory", "map", "detail", "old_value", "new_value", "by"}
			query = `SELECT l.created AS at, t.code AS territory, 'status' AS action, COALESCE(l.changed_by, '') AS by,
			       m.description AS map, '' AS person, '' AS publisher,
			       COALESCE(CASE m.type WHEN 'multi' THEN printf('#%02d-%s', a.floor, a.code) ELSE a.code END, '') AS detail,
			       COALESCE(l.old_status, '') AS old_value, COALESCE(l.new_status, '') AS new_value
			FROM addresses_log l JOIN maps m ON m.id = l.map JOIN territories t ON t.id = l.territory
			LEFT JOIN addresses a ON a.id = l.address
			WHERE l.congregation = {:congregation} AND l.created >= {:since}
			  AND ({:territory} = '' OR l.territory = {:territory}) AND ({:map} = '' OR l.map = {:map})`
		default:
			return Result{}, badArgs("kind must be territory, role, assignment or home")
		}
		var rows []struct {
			At        string `db:"at"`
			Territory string `db:"territory"`
			Action    string `db:"action"`
			By        string `db:"by"`
			Map       string `db:"map"`
			Person    string `db:"person"`
			Publisher string `db:"publisher"`
			Detail    string `db:"detail"`
			OldValue  string `db:"old_value"`
			NewValue  string `db:"new_value"`
		}
		if err := s.App.DB().NewQuery(query + ` ORDER BY at DESC LIMIT 500`).Bind(params).All(&rows); err != nil {
			return Result{}, err
		}
		r := Result{Columns: columns}
		for _, h := range rows {
			r.Rows = append(r.Rows, map[string]any{
				"date": localTime(s, h.At, dateTimeLayout), "territory": h.Territory, "action": h.Action, "by": h.By,
				"map": h.Map, "person": h.Person, "publisher": h.Publisher, "detail": h.Detail,
				"old_value": h.OldValue, "new_value": h.NewValue,
			})
		}
		return truncate(r, clampLimit(args.Limit)), nil
	},
}

// mapIDByName resolves an exact map name within the scope, and the territory
// when given. Two maps can share a name across territories, so an ambiguous name
// asks for the territory.
func mapIDByName(s Scope, territoryID string, name *string) (string, error) {
	if name == nil || strings.TrimSpace(*name) == "" {
		return "", nil
	}
	var ids []string
	// A name entered in several languages is stored as JSON, so match it quoted too.
	err := s.App.DB().NewQuery(`SELECT id FROM maps WHERE congregation = {:congregation}
		AND (lower(description) = lower({:name}) OR instr(lower(description), lower(json_quote({:name}))) > 0)
		AND ({:territory} = '' OR territory = {:territory})`).
		Bind(dbx.Params{"congregation": s.CongregationID, "name": strings.TrimSpace(*name), "territory": territoryID}).Column(&ids)
	if err != nil {
		return "", err
	}
	switch len(ids) {
	case 0:
		return "", badArgs("no map is named %q; use find_places to look it up", *name)
	case 1:
		return ids[0], nil
	default:
		return "", badArgs("several maps are named %q; give the territory code too", *name)
	}
}
