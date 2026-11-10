package assistant

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"ganigeorgiev.com/pocketbase/dbx"
)

// The same bounds /map/add enforces, checked here first so the card never
// offers what the backend would refuse.
const (
	maxNewMaps       = 10
	maxMapFloors     = 100
	maxUnitsPerFloor = 500
	maxMapHomes      = 5000
	minStartFloor    = -10
	maxStartFloor    = 100
	maxCodeRunes     = 20
)

// unitPart is one piece of a map's unit list, in visiting order: a run of
// numbers, or units listed as written.
type unitPart struct {
	From  *string  `json:"from"`
	To    *string  `json:"to"`
	Only  *string  `json:"only"`
	Codes []string `json:"codes"`
}

// normalCode is how two unit codes compare: "05" and "5" are the same unit.
func normalCode(code string) string {
	c := strings.ToLower(strings.TrimSpace(code))
	if t := strings.TrimLeft(c, "0"); t != "" {
		return t
	}
	return c
}

// expandUnits turns the parts into the unit list, in order. Runs are expanded
// here rather than by the model, so a long run cannot lose or repeat a unit.
func expandUnits(parts []unitPart, skip []string) ([]string, error) {
	skipped := map[string]bool{}
	for _, s := range skip {
		skipped[normalCode(strings.TrimPrefix(s, "#"))] = true
	}
	var out []string
	add := func(code string) error {
		if skipped[normalCode(code)] {
			return nil
		}
		if !unitCodePattern.MatchString(code) || utf8.RuneCountInString(code) > maxCodeRunes {
			return badArgs("unit %q may only have letters, digits and hyphens", code)
		}
		if len(out) >= maxUnitsPerFloor {
			return badArgs("a map may have at most %d units on a floor", maxUnitsPerFloor)
		}
		out = append(out, code)
		return nil
	}
	for _, p := range parts {
		switch {
		case p.From != nil && p.To != nil:
			from, to := strings.TrimSpace(*p.From), strings.TrimSpace(*p.To)
			a, errA := strconv.Atoi(from)
			b, errB := strconv.Atoi(to)
			if errA != nil || errB != nil || a < 0 || b < 0 {
				return nil, badArgs("a run goes from one whole number to another, not %q to %q; list other units in codes", from, to)
			}
			// "01 to 08" keeps its zeros; "1 to 12" has none.
			width := 0
			if len(from) > 1 && from[0] == '0' || len(to) > 1 && to[0] == '0' {
				width = max(len(from), len(to))
			}
			only := "all"
			if p.Only != nil {
				only = *p.Only
			}
			step := 1
			if b < a {
				step = -1
			}
			if (b-a)*step >= 2*maxUnitsPerFloor {
				return nil, badArgs("a map may have at most %d units on a floor", maxUnitsPerFloor)
			}
			for n := a; ; n += step {
				if only == "all" || (only == "odd") == (n%2 == 1) {
					if err := add(fmt.Sprintf("%0*d", width, n)); err != nil {
						return nil, err
					}
				}
				if n == b {
					break
				}
			}
		case len(p.Codes) > 0:
			for _, c := range p.Codes {
				if err := add(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(c), "#"))); err != nil {
					return nil, err
				}
			}
		default:
			return nil, badArgs("each part of units needs from and to, or codes")
		}
	}
	seen := map[string]bool{}
	for _, c := range out {
		if seen[normalCode(c)] {
			return nil, badArgs("unit %s appears twice; each unit is listed once", c)
		}
		seen[normalCode(c)] = true
	}
	if len(out) == 0 {
		return nil, badArgs("the map needs at least one unit")
	}
	return out, nil
}

// newMapSpec is how the model describes one map to create.
type newMapSpec struct {
	Name        string     `json:"name"`
	Kind        string     `json:"kind"`
	LowestFloor *int       `json:"lowest_floor"`
	TopFloor    *int       `json:"top_floor"`
	Units       []unitPart `json:"units"`
	Skip        []string   `json:"skip"`
	Address     *string    `json:"address"`
}

// floorCount is how many floors run from lowest to top, with no floor 0.
func floorCount(lowest, top int) int {
	n := top - lowest + 1
	if lowest < 0 && top > 0 {
		n-- // floor 0 does not exist
	}
	return n
}

// planMaps checks each map and works out what /map/add will be asked for.
func planMaps(specs []newMapSpec, existing []string, locale string) ([]map[string]any, int, error) {
	if len(specs) == 0 {
		return nil, 0, badArgs("maps needs at least one map")
	}
	if len(specs) > maxNewMaps {
		return nil, 0, badArgs("at most %d maps at a time", maxNewMaps)
	}
	taken := map[string]bool{}
	for _, name := range existing {
		taken[strings.ToLower(strings.TrimSpace(resolveLocalized(name, locale)))] = true
	}
	plans := make([]map[string]any, 0, len(specs))
	total := 0
	for _, m := range specs {
		name := strings.TrimSpace(m.Name)
		if name == "" || utf8.RuneCountInString(name) > maxNameRunes {
			return nil, 0, badArgs("each map needs a name of 1 to %d characters", maxNameRunes)
		}
		if taken[strings.ToLower(name)] {
			return nil, 0, badArgs("the territory already has a map named %q", name)
		}
		taken[strings.ToLower(name)] = true

		units, err := expandUnits(m.Units, m.Skip)
		if err != nil {
			return nil, 0, err
		}
		plan := map[string]any{"name": name, "units": units}
		floors := 1
		switch m.Kind {
		case "houses":
			plan["type"] = "single"
			plan["lowest_floor"], plan["top_floor"] = 1, 1
		case "building":
			if m.LowestFloor == nil || m.TopFloor == nil {
				return nil, 0, badArgs("%s needs its lowest and top floors; ask the admin, never guess", name)
			}
			lowest, top := *m.LowestFloor, *m.TopFloor
			if lowest == 0 || top == 0 || lowest < minStartFloor || lowest > maxStartFloor || top < lowest {
				return nil, 0, badArgs("%s: floors run from the lowest to the top floor, between %d and %d, and there is no floor 0",
					name, minStartFloor, maxStartFloor)
			}
			floors = floorCount(lowest, top)
			if floors > maxMapFloors {
				return nil, 0, badArgs("%s: a map may have at most %d floors", name, maxMapFloors)
			}
			plan["type"] = "multi"
			plan["lowest_floor"], plan["top_floor"] = lowest, top
		default:
			return nil, 0, badArgs("kind must be building or houses")
		}
		homes := floors * len(units)
		if homes > maxMapHomes {
			return nil, 0, badArgs("%s would have %d homes; a map may have at most %d", name, homes, maxMapHomes)
		}
		plan["floors"], plan["homes"] = floors, homes
		address := name
		if m.Address != nil && strings.TrimSpace(*m.Address) != "" {
			address = strings.TrimSpace(*m.Address)
		}
		plan["address"] = address
		plans = append(plans, plan)
		total += homes
	}
	return plans, total, nil
}

func newMapsParam(description string) map[string]any {
	part := object(map[string]any{
		"from":  nullable("string", "First number of a run, as written, such as 671 or 01."),
		"to":    nullable("string", "Last number of a run; lower than from to count down."),
		"only":  nullableEnum("odd or even keeps only those numbers in the run; all keeps every number.", "all", "odd", "even"),
		"codes": map[string]any{"type": []string{"array", "null"}, "items": map[string]any{"type": "string"}, "description": "Units listed as written, such as 3A or 23B, when they are not a run."},
	})
	return map[string]any{
		"type":        "array",
		"description": description,
		"items": object(map[string]any{
			"name":         map[string]any{"type": "string", "description": "The map's name, such as Blk 412 or Jalan Bahagia."},
			"kind":         enumOf("building for a block with floors; houses for a street of houses.", "building", "houses"),
			"lowest_floor": nullable("integer", "A building's lowest floor with homes, often 2 above a void deck; null for houses."),
			"top_floor":    nullable("integer", "A building's top floor; null for houses."),
			"units": map[string]any{
				"type": "array", "items": part,
				"description": "The units on each floor, or the houses, in visiting order: runs such as 671 to 685 odd, " +
					"and codes for units like 3A.",
			},
			"skip":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Numbers to leave out of the runs, such as 4 or 13."},
			"address": nullable("string", "The address as the admin gave it, to find the map's location; the admin confirms it on a map."),
		}),
	}
}

func mapNames(s Scope, territoryID string) ([]string, error) {
	var names []string
	err := s.App.DB().NewQuery(`SELECT COALESCE(description, '') FROM maps WHERE territory = {:territory}`).
		Bind(dbx.Params{"territory": territoryID}).Column(&names)
	return names, err
}

var proposeTerritoryCreate = Tool{
	Name: "propose_territory_create",
	Description: "Prepare a new territory, with no maps or up to 10. Maps can be added later, so a territory " +
		"with only a code and name is fine. Nothing is created until the asker sets each map's location and " +
		"confirms on a card. Use it only when the admin asks to create a territory.",
	Params: object(map[string]any{
		"code": map[string]any{"type": "string", "description": "The new territory's code, such as T05."},
		"name": map[string]any{"type": "string", "description": "The territory's name, such as Bedok North."},
		"maps": newMapsParam("Maps to create with the territory, only those the admin described. Leave it " +
			"empty when they named none: maps can be added later, so do not ask for them."),
	}),
	Run: func(s Scope, raw json.RawMessage) (Result, error) {
		var args struct {
			Code string       `json:"code"`
			Name string       `json:"name"`
			Maps []newMapSpec `json:"maps"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return Result{}, err
		}
		code := strings.TrimSpace(args.Code)
		if code == "" || utf8.RuneCountInString(code) > maxCodeRunes {
			return Result{}, badArgs("code must be 1 to %d characters", maxCodeRunes)
		}
		name, err := newName(args.Name)
		if err != nil {
			return Result{}, badArgs("name must be 1 to %d characters", maxNameRunes)
		}
		switch _, _, err := territoryByCode(s, code); {
		case err == nil:
			return Result{}, badArgs("a territory with the code %s already exists; add maps to it with propose_maps_create", code)
		case !errors.As(err, new(argError)):
			return Result{}, err
		}
		p := Proposal{
			Target: &Target{Name: code}, Value: name,
			Data: map[string]any{"maps": []map[string]any{}, "homes": 0},
		}
		if len(args.Maps) > 0 {
			plans, total, err := planMaps(args.Maps, nil, s.Locale)
			if err != nil {
				return Result{}, err
			}
			p.Data = map[string]any{"maps": plans, "homes": total}
		}
		return Result{Proposal: &p}, nil
	},
}

var proposeMapsCreate = Tool{
	Name: "propose_maps_create",
	Description: "Prepare up to 10 new maps in an existing territory. Nothing is created until the asker sets each " +
		"map's location and confirms on a card. Use it only when the admin asks to add or create maps.",
	Params: object(map[string]any{
		"territory": map[string]any{"type": "string", "description": "The existing territory's code."},
		"maps": newMapsParam("The maps, each one building or one street of houses. Never guess floors or " +
			"units: ask the admin when they did not say."),
	}),
	Run: func(s Scope, raw json.RawMessage) (Result, error) {
		var args struct {
			Territory string       `json:"territory"`
			Maps      []newMapSpec `json:"maps"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return Result{}, err
		}
		territoryID, code, err := territoryByCode(s, args.Territory)
		if err != nil {
			return Result{}, err
		}
		existing, err := mapNames(s, territoryID)
		if err != nil {
			return Result{}, err
		}
		plans, total, err := planMaps(args.Maps, existing, s.Locale)
		if err != nil {
			return Result{}, err
		}
		return Result{Proposal: &Proposal{
			Target: territoryTarget(territoryID, code),
			Data:   map[string]any{"maps": plans, "homes": total},
		}}, nil
	},
}
