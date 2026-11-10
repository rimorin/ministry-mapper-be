package assistant

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"ganigeorgiev.com/pocketbase/dbx"
)

// maxPinnedShown bounds the instructions a details card lists.
const maxPinnedShown = 5

var mapDetails = Tool{
	Name: "map_details",
	Description: "Everything about one map: its territory, floors and units, homes by status and progress, " +
		"household types, recent visits, when it was last reset, messages and pinned instructions, notes, share " +
		"links and whether it has a location. Use it when the admin asks about one map, such as \"show me Blk " +
		"412\" or \"details of 950A Marsiling Delta\". Counted live from the homes.",
	Params: object(map[string]any{
		"map":       mapArg,
		"territory": mapTerritoryArg,
	}),
	Run: func(s Scope, raw json.RawMessage) (Result, error) {
		var args struct {
			mapRef
		}
		if err := decodeArgs(raw, &args); err != nil {
			return Result{}, err
		}
		target, err := args.target(s)
		if err != nil {
			return Result{}, err
		}

		var m struct {
			Type          string `db:"type"`
			Location      string `db:"location"`
			ResetAt       string `db:"reset_at"`
			TerritoryCode string `db:"territory_code"`
			TerritoryName string `db:"territory_name"`
			MaxTries      int    `db:"max_tries"`
		}
		if err := s.App.DB().NewQuery(`
			SELECT COALESCE(m.type, '') AS type, COALESCE(m.coordinates, '') AS location,
			       COALESCE(m.reset_at, '') AS reset_at, t.code AS territory_code,
			       COALESCE(t.description, '') AS territory_name, COALESCE(c.max_tries, 0) AS max_tries
			FROM maps m JOIN territories t ON t.id = m.territory JOIN congregations c ON c.id = m.congregation
			WHERE m.id = {:id}`).Bind(dbx.Params{"id": target.ID}).One(&m); err != nil {
			return Result{}, err
		}

		// Counted as the app counts a map's progress: only homes with a
		// countable household type (processMapAggregates).
		var homes []struct {
			Floor     int    `db:"floor"`
			Code      string `db:"code"`
			Status    string `db:"status"`
			Tries     int    `db:"tries"`
			HasNotes  bool   `db:"has_notes"`
			Countable bool   `db:"countable"`
		}
		if err := s.App.DB().NewQuery(`
			SELECT COALESCE(a.floor, 0) AS floor, a.code, COALESCE(a.status, '') AS status,
			       COALESCE(a.not_home_tries, 0) AS tries, TRIM(COALESCE(a.notes, '')) != '' AS has_notes,
			       EXISTS (SELECT 1 FROM address_options ao JOIN options o ON o.id = ao.option
			               WHERE ao.address = a.id AND ao.map = a.map AND o.is_countable = TRUE) AS countable
			FROM addresses a WHERE a.map = {:map}`).Bind(dbx.Params{"map": target.ID}).All(&homes); err != nil {
			return Result{}, err
		}
		counts := map[string]int{}
		units := map[string]bool{}
		floors := map[int]bool{}
		atLimit, notCounted, withNotes := 0, 0, 0
		for _, h := range homes {
			floors[h.Floor] = true
			units[h.Code] = true
			if h.HasNotes {
				withNotes++
			}
			if !h.Countable {
				notCounted++
				continue
			}
			counts[h.Status]++
			if h.Status == "not_home" && m.MaxTries > 0 && h.Tries >= m.MaxTries {
				atLimit++
			}
		}
		counted := counts["done"] + counts["not_done"] + counts["not_home"]
		progress := 0
		if counted > 0 {
			progress = int(math.Round(float64(counts["done"]+atLimit) / float64(counted) * 100))
		}
		lowest, top := 0, 0
		for f := range floors {
			if lowest == 0 || f < lowest {
				lowest = f
			}
			if top == 0 || f > top {
				top = f
			}
		}
		floorText := ""
		if m.Type == "multi" && len(floors) > 0 {
			floorText = fmt.Sprintf("%d to %d", lowest, top)
		}

		var types []struct {
			Name  string `db:"name"`
			Homes int    `db:"homes"`
		}
		if err := s.App.DB().NewQuery(`
			SELECT COALESCE(o.description, '') AS name, COUNT(DISTINCT ao.address) AS homes
			FROM address_options ao JOIN options o ON o.id = ao.option
			WHERE ao.map = {:map} GROUP BY o.id ORDER BY homes DESC`).Bind(dbx.Params{"map": target.ID}).All(&types); err != nil {
			return Result{}, err
		}
		typeParts := make([]string, 0, len(types))
		for _, t := range types {
			typeParts = append(typeParts, fmt.Sprintf("%s %d", resolveLocalized(t.Name, s.Locale), t.Homes))
		}

		var activity struct {
			LastVisit string `db:"last_visit"`
			Recent    int    `db:"recent"`
		}
		if err := s.App.DB().NewQuery(`
			SELECT COALESCE(MAX(created), '') AS last_visit,
			       COALESCE(SUM(created >= {:since}), 0) AS recent
			FROM addresses_log WHERE map = {:map} AND new_status IN ` + coverageStatuses).
			Bind(dbx.Params{"map": target.ID, "since": daysAgo(s, 30)}).One(&activity); err != nil {
			return Result{}, err
		}

		var unread int
		if err := s.App.DB().NewQuery(`SELECT COUNT(*) FROM messages
			WHERE map = {:map} AND read = FALSE AND type != 'administrator'`).
			Bind(dbx.Params{"map": target.ID}).Row(&unread); err != nil {
			return Result{}, err
		}
		var pinned []string
		if err := s.App.DB().NewQuery(`SELECT COALESCE(message, '') FROM messages
			WHERE map = {:map} AND pinned = TRUE AND type = 'administrator' ORDER BY created DESC`).
			Bind(dbx.Params{"map": target.ID}).Column(&pinned); err != nil {
			return Result{}, err
		}

		var links []struct {
			Publisher string `db:"publisher" json:"publisher"`
			Expires   string `db:"expires" json:"expires"`
		}
		if err := s.App.DB().NewQuery(`SELECT COALESCE(publisher, '') AS publisher, expiry_date AS expires
			FROM assignments WHERE map = {:map} AND expiry_date > {:now} ORDER BY expiry_date`).
			Bind(dbx.Params{"map": target.ID, "now": dbTime(s.Now)}).All(&links); err != nil {
			return Result{}, err
		}

		row := map[string]any{
			"map_id": target.ID, "map": target.Name, "territory_id": target.TerritoryID,
			"territory": m.TerritoryCode, "territory_name": resolveLocalized(m.TerritoryName, s.Locale),
			"type": m.Type, "floors": floorText,
			"units_per_floor": len(units), "homes": len(homes),
			"progress": progress, "not_done": counts["not_done"], "done": counts["done"],
			"not_home": counts["not_home"], "at_try_limit": atLimit, "dnc": counts["do_not_call"],
			"invalid": counts["invalid"], "not_counted": notCounted, "household_types": strings.Join(typeParts, ", "),
			"last_visit": localDate(s, activity.LastVisit), "visits_30_days": activity.Recent,
			"last_reset": localDate(s, m.ResetAt), "unread_messages": unread, "pinned_instructions": len(pinned),
			"homes_with_notes": withNotes, "active_links": len(links), "has_location": false,
		}
		if p, ok := parsePoint(m.Location); ok {
			row["has_location"], row["lat"], row["lng"] = true, p.Lat, p.Lng
		}
		// For the card only; the model is told how many, not what or who.
		if len(pinned) > maxPinnedShown {
			pinned = pinned[:maxPinnedShown]
		}
		row["pinned"] = pinned
		if atLeast(s.Role, roleConductor) {
			for i := range links {
				links[i].Expires = localTime(s, links[i].Expires, dateTimeLayout)
			}
			row["links"] = links
		}
		return Result{
			Columns: []string{"territory", "map", "type", "floors", "units_per_floor", "homes", "progress", "not_done",
				"done", "not_home", "at_try_limit", "dnc", "invalid", "not_counted", "household_types", "last_visit",
				"visits_30_days", "last_reset", "unread_messages", "pinned_instructions", "homes_with_notes",
				"active_links", "has_location"},
			Rows: []map[string]any{row},
			Note: "homes counts every home; progress counts done and nobody-home homes at the try limit among homes " +
				"whose household type is counted. not_counted homes have only uncounted types.",
		}, nil
	},
}
