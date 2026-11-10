package assistant

import (
	"encoding/json"
	"regexp"
	"slices"
	"sort"
	"strings"

	"ganigeorgiev.com/pocketbase/dbx"
)

// Bounds the backend and the settings screen already enforce.
const (
	minMaxTries, maxMaxTries       = 1, 4
	minExpiryHours, maxExpiryHours = 1, 24
	minThreshold, maxThreshold     = 1, 100
	maxNewUnits                    = 20
)

// The same rule /map/code/add applies.
var unitCodePattern = regexp.MustCompile(`^[a-zA-Z0-9-]+$`)

// resetCounts is what a reset turns back to not done, for the card.
type resetCounts struct {
	Done    int `db:"done"`
	NotHome int `db:"not_home"`
	Maps    int `db:"maps"`
}

func (c resetCounts) data() map[string]any {
	return map[string]any{"done": c.Done, "not_home": c.NotHome, "maps": c.Maps}
}

func countResettable(s Scope, column, id string) (resetCounts, error) {
	var c resetCounts
	err := s.App.DB().NewQuery(`
		SELECT COALESCE(SUM(status = 'done'), 0) AS done, COALESCE(SUM(status = 'not_home'), 0) AS not_home,
		       COUNT(DISTINCT map) AS maps
		FROM addresses WHERE ` + column + ` = {:id} AND status IN ('done', 'not_home')`).
		Bind(dbx.Params{"id": id}).One(&c)
	return c, err
}

var proposeMapReset = Tool{
	Name: "propose_map_reset",
	Description: "Prepare to reset a map, as Reset on the map does: homes done or not home go back to not done " +
		"and nobody-home tries are cleared; do-not-call and invalid homes stay. It cannot be undone. Nothing " +
		"changes until the asker confirms on a card. Use it only when the admin asks to reset a map.",
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
		counts, err := countResettable(s, "map", target.ID)
		if err != nil {
			return Result{}, err
		}
		if counts.Done+counts.NotHome == 0 {
			return Result{}, badArgs("%s has no homes done or not home, so there is nothing to reset", target.Name)
		}
		return Result{Proposal: &Proposal{
			Warnings: []string{"cannot_undo"},
			Target:   &target, Data: counts.data(),
		}}, nil
	},
}

var proposeTerritoryReset = Tool{
	Name: "propose_territory_reset",
	Description: "Prepare to reset every map in a territory, as Reset on the territory does, and start its next " +
		"round: homes done or not home go back to not done; do-not-call and invalid homes stay. It cannot be " +
		"undone. Nothing changes until the asker confirms on a card. Use it only when the admin asks to reset a " +
		"territory.",
	Params: object(map[string]any{
		"territory": map[string]any{"type": "string", "description": "The territory's exact code."},
	}),
	Run: func(s Scope, raw json.RawMessage) (Result, error) {
		var args struct {
			Territory string `json:"territory"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return Result{}, err
		}
		territoryID, code, err := territoryByCode(s, args.Territory)
		if err != nil {
			return Result{}, err
		}
		counts, err := countResettable(s, "territory", territoryID)
		if err != nil {
			return Result{}, err
		}
		if counts.Done+counts.NotHome == 0 {
			return Result{}, badArgs("%s has no homes done or not home, so there is nothing to reset", code)
		}
		return Result{Proposal: &Proposal{
			Warnings: []string{"cannot_undo", "starts_round"},

			Target: territoryTarget(territoryID, code),
			Data:   counts.data(),
		}}, nil
	},
}

// bulkHomes lists homes for a change to many at once, each with its full
// state, since /address/update overwrites every field.
func bulkHomes(s Scope, where string, params dbx.Params) ([]Item, bool, error) {
	var rows []struct {
		ID      string `db:"id"`
		Map     string `db:"map"`
		MapName string `db:"map_name"`
		MapType string `db:"map_type"`
		Floor   int    `db:"floor"`
		Code    string `db:"code"`
		Status  string `db:"status"`
		Notes   string `db:"notes"`
		Tries   int    `db:"tries"`
		DncTime string `db:"dnc_time"`
		Types   string `db:"types"`
	}
	params["congregation"] = s.CongregationID
	params["limit"] = maxItems + 1
	err := s.App.DB().NewQuery(`
		SELECT a.id, a.map, COALESCE(m.description, '') AS map_name, COALESCE(m.type, '') AS map_type,
		       COALESCE(a.floor, 0) AS floor, a.code, COALESCE(a.status, '') AS status,
		       COALESCE(a.notes, '') AS notes, COALESCE(a.not_home_tries, 0) AS tries,
		       COALESCE(a.dnc_time, '') AS dnc_time,
		       COALESCE((SELECT group_concat(ao.option) FROM address_options ao WHERE ao.address = a.id), '') AS types
		FROM addresses a JOIN maps m ON m.id = a.map JOIN congregations c ON c.id = a.congregation
		WHERE a.congregation = {:congregation} AND ` + where + `
		ORDER BY m.description, a.floor, a.sequence
		LIMIT {:limit}`).Bind(params).All(&rows)
	if err != nil {
		return nil, false, err
	}
	more := len(rows) > maxItems
	if more {
		rows = rows[:maxItems]
	}
	items := make([]Item, 0, len(rows))
	for _, r := range rows {
		types := []string{}
		if r.Types != "" {
			types = strings.Split(r.Types, ",")
		}
		state, _ := json.Marshal(homeState{
			Status: r.Status, Notes: r.Notes, Tries: r.Tries, DncTime: r.DncTime, Types: types, TypeNames: []string{},
		})
		items = append(items, Item{
			ID: r.ID, Map: r.Map, Detail: string(state),
			Label: resolveLocalized(r.MapName, s.Locale) + " " + home(r.MapType, r.Floor, r.Code),
		})
	}
	return items, more, nil
}

var proposeTriesReset = Tool{
	Name: "propose_tries_reset",
	Description: "Prepare to send homes that reached the nobody-home try limit back to not done with their tries " +
		"cleared, on one map, one territory or the whole congregation (up to 50 homes a card). Nothing changes " +
		"until the asker confirms on a card. Use it only when the admin asks to reset nobody-home homes, never " +
		"for a request to reset a map or a territory.",
	Params: object(map[string]any{
		"map":       nullable("string", "An exact map name, to reset only that map's homes."),
		"territory": nullable("string", "A territory code, to reset only that territory's homes."),
	}),
	Run: func(s Scope, raw json.RawMessage) (Result, error) {
		var args struct {
			Map       *string `json:"map"`
			Territory *string `json:"territory"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return Result{}, err
		}
		mapID, territoryID, err := placeScope(s, args.Map, args.Territory)
		if err != nil {
			return Result{}, err
		}
		items, more, err := bulkHomes(s, `a.status = 'not_home' AND c.max_tries > 0 AND a.not_home_tries >= c.max_tries
			AND ({:map} = '' OR a.map = {:map}) AND ({:territory} = '' OR m.territory = {:territory})`,
			dbx.Params{"map": mapID, "territory": territoryID})
		if err != nil {
			return Result{}, err
		}
		if len(items) == 0 {
			return Result{}, badArgs("no home there has reached the nobody-home try limit")
		}
		p := Proposal{
			Items: items, Value: `{"status":"not_done","not_home_tries":0}`,
		}
		if more {
			p.Warnings = append(p.Warnings, "more_homes")
		}
		return Result{Proposal: &p}, nil
	},
}

// Do not call and invalid say something about the home itself, so they are
// set one home at a time (propose_home_update), never in bulk.
var bulkStatuses = []string{"not_done", "done", "not_home"}

var proposeHomesStatus = Tool{
	Name: "propose_homes_status",
	Description: "Prepare to set the status of several homes on one map: every home, a range of floors, or some " +
		"units on each floor (up to 50 homes a card). Do-not-call and invalid homes stay as they are. Nothing " +
		"changes until the asker confirms on a card. Use it only when the admin asks to change several homes; " +
		"for one home use propose_home_update.",
	Params: object(map[string]any{
		"map":        mapArg,
		"territory":  mapTerritoryArg,
		"status":     enumOf("The new status.", bulkStatuses...),
		"from_floor": nullable("integer", "The lowest floor to change, or null for the lowest."),
		"to_floor":   nullable("integer", "The highest floor to change, or null for the highest."),
		"units": map[string]any{
			"type": []string{"array", "null"}, "items": map[string]any{"type": "string"},
			"description": "Only these unit numbers on each floor, such as [\"1\", \"2\"], or null for all.",
		},
	}),
	Run: func(s Scope, raw json.RawMessage) (Result, error) {
		var args struct {
			mapRef
			Status    string   `json:"status"`
			FromFloor *int     `json:"from_floor"`
			ToFloor   *int     `json:"to_floor"`
			Units     []string `json:"units"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return Result{}, err
		}
		if !slices.Contains(bulkStatuses, args.Status) {
			return Result{}, badArgs("status must be one of %s; set do not call or invalid one home at a time",
				strings.Join(bulkStatuses, ", "))
		}
		target, err := args.target(s)
		if err != nil {
			return Result{}, err
		}
		from, to := 0, 1<<30
		if args.FromFloor != nil {
			from = *args.FromFloor
		}
		if args.ToFloor != nil {
			to = *args.ToFloor
		}
		if from > to {
			return Result{}, badArgs("from_floor is above to_floor")
		}
		units := make([]string, 0, len(args.Units))
		for _, u := range args.Units {
			if _, code := parseUnit(u); code != "" {
				units = append(units, code)
			}
		}
		items, more, err := bulkHomes(s, `a.map = {:map} AND COALESCE(a.floor, 0) BETWEEN {:from} AND {:to}
			AND a.status NOT IN ('do_not_call', 'invalid', {:status})
			AND ({:units} = '' OR instr(',' || {:units} || ',', ',' || a.code || ',') > 0)`,
			dbx.Params{"map": target.ID, "from": from, "to": to, "status": args.Status, "units": strings.Join(units, ",")})
		if err != nil {
			return Result{}, err
		}
		if len(items) == 0 {
			return Result{}, badArgs("no home there would change: they are already %s, do not call or invalid", args.Status)
		}
		patch := map[string]any{"status": args.Status}
		if args.Status == "not_home" {
			patch["not_home_tries"] = 1 // the same start the status form gives a first nobody-home visit
		}
		value, _ := json.Marshal(patch)
		p := Proposal{Target: &target, To: args.Status, Items: items, Value: string(value)}
		if more {
			p.Warnings = append(p.Warnings, "more_homes")
		}
		return Result{Proposal: &p}, nil
	},
}

var proposeDncReturn = Tool{
	Name: "propose_dnc_return",
	Description: "Prepare to send do-not-call homes marked more than some months ago back to not done, so they " +
		"are visited again, on one map, one territory or the whole congregation (up to 50 homes a card). Nothing " +
		"changes until the asker confirms on a card. Use it only when the admin asks to return old do-not-call " +
		"homes.",
	Params: object(map[string]any{
		"older_than_months": map[string]any{"type": "integer", "description": "Only homes marked do not call at least this many months ago, 1 to 120."},
		"map":               nullable("string", "An exact map name, to return only that map's homes."),
		"territory":         nullable("string", "A territory code, to return only that territory's homes."),
	}),
	Run: func(s Scope, raw json.RawMessage) (Result, error) {
		var args struct {
			Months    int     `json:"older_than_months"`
			Map       *string `json:"map"`
			Territory *string `json:"territory"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return Result{}, err
		}
		if args.Months < 1 || args.Months > 120 {
			return Result{}, badArgs("older_than_months must be between 1 and 120; ask the admin how old")
		}
		mapID, territoryID, err := placeScope(s, args.Map, args.Territory)
		if err != nil {
			return Result{}, err
		}
		before := s.Now.AddDate(0, -args.Months, 0)
		items, more, err := bulkHomes(s, `a.status = 'do_not_call' AND COALESCE(a.dnc_time, '') != '' AND a.dnc_time < {:before}
			AND ({:map} = '' OR a.map = {:map}) AND ({:territory} = '' OR m.territory = {:territory})`,
			dbx.Params{"before": dbTime(before), "map": mapID, "territory": territoryID})
		if err != nil {
			return Result{}, err
		}
		if len(items) == 0 {
			return Result{}, badArgs("no home there was marked do not call more than %d months ago", args.Months)
		}
		p := Proposal{
			Warnings: []string{"leaves_do_not_call"},
			Items:    items, Value: `{"status":"not_done"}`, Data: map[string]any{"months": args.Months},
		}
		if more {
			p.Warnings = append(p.Warnings, "more_homes")
		}
		return Result{Proposal: &p}, nil
	},
}

// settingsState holds the settings the assistant may change.
type settingsState struct {
	MaxTries  int `db:"max_tries" json:"max_tries"`
	Expiry    int `db:"expiry_hours" json:"expiry_hours"`
	Threshold int `db:"completion_threshold" json:"completion_threshold"`
}

var proposeSettings = Tool{
	Name: "propose_settings",
	Description: "Prepare a change to the congregation's settings: the nobody-home try limit, a normal link's " +
		"hours, or the percentage at which a territory counts as completed. Nothing changes until the admin " +
		"confirms on a card. Use it only when the admin asks to change a setting.",
	Params: object(map[string]any{
		"max_tries":            nullable("integer", "Nobody-home tries before a home stops being visited, 1 to 4."),
		"expiry_hours":         nullable("integer", "How many hours a normal share link lasts, 1 to 24."),
		"completion_threshold": nullable("integer", "The progress percentage at which a territory counts as completed, 1 to 100."),
	}),
	Run: func(s Scope, raw json.RawMessage) (Result, error) {
		var args struct {
			MaxTries  *int `json:"max_tries"`
			Expiry    *int `json:"expiry_hours"`
			Threshold *int `json:"completion_threshold"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return Result{}, err
		}
		check := func(v *int, lo, hi int, name string) error {
			if v != nil && (*v < lo || *v > hi) {
				return badArgs("%s must be between %d and %d", name, lo, hi)
			}
			return nil
		}
		for _, err := range []error{
			check(args.MaxTries, minMaxTries, maxMaxTries, "max_tries"),
			check(args.Expiry, minExpiryHours, maxExpiryHours, "expiry_hours"),
			check(args.Threshold, minThreshold, maxThreshold, "completion_threshold"),
		} {
			if err != nil {
				return Result{}, err
			}
		}
		var before settingsState
		if err := s.App.DB().NewQuery(`SELECT COALESCE(max_tries, 0) AS max_tries, COALESCE(expiry_hours, 0) AS expiry_hours,
			COALESCE(completion_threshold, 0) AS completion_threshold FROM congregations WHERE id = {:id}`).
			Bind(dbx.Params{"id": s.CongregationID}).One(&before); err != nil {
			return Result{}, err
		}
		after := before
		if args.MaxTries != nil {
			after.MaxTries = *args.MaxTries
		}
		if args.Expiry != nil {
			after.Expiry = *args.Expiry
		}
		if args.Threshold != nil {
			after.Threshold = *args.Threshold
		}
		if after == before {
			return Result{}, badArgs("the settings already have those values")
		}
		previous, _ := json.Marshal(before)
		value, _ := json.Marshal(after)
		return Result{Proposal: &Proposal{
			Warnings: []string{"affects_everyone"},
			Previous: string(previous), Value: string(value),
		}}, nil
	},
}

var proposeMapMove = Tool{
	Name: "propose_map_move",
	Description: "Prepare to move a map, with all its homes, to another territory. Nothing changes until the " +
		"admin confirms on a card. Use it only when the admin asks to move a map.",
	Params: object(map[string]any{
		"map":          mapArg,
		"territory":    nullable("string", "The map's current territory code, when several maps share the name."),
		"to_territory": map[string]any{"type": "string", "description": "The code of the territory to move it to."},
	}),
	Run: func(s Scope, raw json.RawMessage) (Result, error) {
		var args struct {
			mapRef
			ToTerritory string `json:"to_territory"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return Result{}, err
		}
		target, err := args.target(s)
		if err != nil {
			return Result{}, err
		}
		toID, toCode, err := territoryByCode(s, args.ToTerritory)
		if err != nil {
			return Result{}, err
		}
		if toID == target.TerritoryID {
			return Result{}, badArgs("%s is already in %s", target.Name, args.ToTerritory)
		}
		var fromCode string
		if err := s.App.DB().NewQuery(`SELECT COALESCE((SELECT code FROM territories WHERE id = {:id}), '')`).
			Bind(dbx.Params{"id": target.TerritoryID}).Row(&fromCode); err != nil {
			return Result{}, err
		}
		return Result{Proposal: &Proposal{
			Target: &target, From: fromCode, To: toCode,
			Previous: target.TerritoryID, Value: toID,
		}}, nil
	},
}

var proposeFloorAdd = Tool{
	Name: "propose_floor_add",
	Description: "Prepare to add a floor to a building map, above the top floor or below the lowest, with the " +
		"same units as that floor. Nothing changes until the asker confirms on a card. Use it only when the " +
		"admin asks to add a floor.",
	Params: object(map[string]any{
		"map":       mapArg,
		"territory": mapTerritoryArg,
		"where":     enumOf("top adds a floor above the highest, bottom one below the lowest.", "top", "bottom"),
	}),
	Run: func(s Scope, raw json.RawMessage) (Result, error) {
		var args struct {
			mapRef
			Where string `json:"where"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return Result{}, err
		}
		target, err := args.target(s)
		if err != nil {
			return Result{}, err
		}
		var m struct {
			Type   string `db:"type"`
			Top    int    `db:"top"`
			Bottom int    `db:"bottom"`
		}
		if err := s.App.DB().NewQuery(`SELECT COALESCE(m.type, '') AS type, COALESCE(MAX(a.floor), 0) AS top,
			COALESCE(MIN(a.floor), 0) AS bottom FROM maps m LEFT JOIN addresses a ON a.map = m.id WHERE m.id = {:id}`).
			Bind(dbx.Params{"id": target.ID}).One(&m); err != nil {
			return Result{}, err
		}
		if m.Type != "multi" {
			return Result{}, badArgs("%s is a map of houses, which has no floors; add units instead", target.Name)
		}
		from, floor := m.Top, m.Top+1
		if args.Where == "bottom" {
			from, floor = m.Bottom, m.Bottom-1
			if floor == 0 {
				floor = -1 // the same skip /map/floor/add makes
			}
		} else if args.Where != "top" {
			return Result{}, badArgs("where must be top or bottom")
		}
		var units []string
		if err := s.App.DB().NewQuery(`SELECT code FROM addresses WHERE map = {:id} AND floor = {:floor} ORDER BY sequence`).
			Bind(dbx.Params{"id": target.ID, "floor": from}).Column(&units); err != nil {
			return Result{}, err
		}
		return Result{Proposal: &Proposal{
			Target: &target, To: args.Where,
			Data: map[string]any{"floor": floor, "units": units},
		}}, nil
	},
}

var proposeUnitAdd = Tool{
	Name: "propose_unit_add",
	Description: "Prepare to add units to a map; in a building each new unit is added on every floor. Nothing " +
		"changes until the asker confirms on a card. Use it only when the admin asks to add units or houses.",
	Params: object(map[string]any{
		"map":       mapArg,
		"territory": mapTerritoryArg,
		"units": map[string]any{
			"type": "array", "items": map[string]any{"type": "string"},
			"description": "The new unit numbers, such as 15 or 15A, as the admin wrote them.",
		},
	}),
	Run: func(s Scope, raw json.RawMessage) (Result, error) {
		var args struct {
			mapRef
			Units []string `json:"units"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return Result{}, err
		}
		target, err := args.target(s)
		if err != nil {
			return Result{}, err
		}
		var existing []string
		if err := s.App.DB().NewQuery(`SELECT DISTINCT code FROM addresses WHERE map = {:id}`).
			Bind(dbx.Params{"id": target.ID}).Column(&existing); err != nil {
			return Result{}, err
		}
		var floors int
		if err := s.App.DB().NewQuery(`SELECT COUNT(DISTINCT floor) FROM addresses WHERE map = {:id}`).
			Bind(dbx.Params{"id": target.ID}).Row(&floors); err != nil {
			return Result{}, err
		}
		var add, skipped []string
		seen := map[string]bool{}
		for _, raw := range args.Units {
			unit := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "#"))
			if !unitCodePattern.MatchString(unit) {
				return Result{}, badArgs("unit %q may only have letters, digits and hyphens", raw)
			}
			if seen[strings.ToLower(unit)] {
				continue
			}
			seen[strings.ToLower(unit)] = true
			if containsFold(existing, unit) {
				skipped = append(skipped, unit)
			} else {
				add = append(add, unit)
			}
		}
		if len(add) == 0 {
			if len(skipped) > 0 {
				return Result{}, badArgs("%s already has %s", target.Name, strings.Join(skipped, ", "))
			}
			return Result{}, badArgs("units is required")
		}
		if len(add) > maxNewUnits {
			return Result{}, badArgs("add at most %d units at a time", maxNewUnits)
		}
		sort.Strings(skipped)
		var warnings []string
		if len(skipped) > 0 {
			warnings = append(warnings, "some_exist")
		}
		if floors < 1 {
			floors = 1
		}
		return Result{Proposal: &Proposal{
			Warnings: warnings,
			Target:   &target,
			Data:     map[string]any{"units": add, "existing": skipped, "floors": floors},
		}}, nil
	},
}
