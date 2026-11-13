package assistant

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"ganigeorgiev.com/pocketbase/dbx"
)

const (
	// maxItems bounds the records one card changes.
	maxItems = 50
	// farMeters is how far a map's new location may be from its old one before
	// the card warns that it may be the wrong map.
	farMeters    = 500
	maxNoteRunes = 1000
)

var homeStatuses = []string{"not_done", "done", "not_home", "do_not_call", "invalid"}

// placeScope narrows a query to a map or a territory, whichever was named.
func placeScope(s Scope, mapName, territory *string) (mapID, territoryID string, err error) {
	territoryID, err = territoryIDByCode(s, territory)
	if err != nil {
		return "", "", err
	}
	mapID, err = mapIDByName(s, territoryID, mapName)
	return mapID, territoryID, err
}

var proposeMarkRead = Tool{
	Name: "propose_mark_read",
	Description: "Prepare to mark unread messages from publishers as read, on one map, one territory or the " +
		"whole congregation. Nothing changes until the asker confirms on a card. Use it only when the admin " +
		"asks to mark messages or feedback read.",
	Params: object(map[string]any{
		"map":       nullable("string", "An exact map name, to mark only that map's messages."),
		"territory": nullable("string", "A territory code, to mark only that territory's messages."),
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
		var rows []struct {
			ID   string `db:"id"`
			Map  string `db:"map"`
			Name string `db:"name"`
		}
		err = s.App.DB().NewQuery(`
			SELECT msg.id, msg.map, COALESCE(m.description, '') AS name
			FROM messages msg JOIN maps m ON m.id = msg.map
			WHERE msg.congregation = {:congregation} AND msg.read = FALSE AND msg.type != 'administrator'
			  AND ({:map} = '' OR msg.map = {:map}) AND ({:territory} = '' OR m.territory = {:territory})
			ORDER BY msg.created
			LIMIT {:limit}`).
			Bind(dbx.Params{"congregation": s.CongregationID, "map": mapID, "territory": territoryID, "limit": maxItems + 1}).
			All(&rows)
		if err != nil {
			return Result{}, err
		}
		if len(rows) == 0 {
			return Result{}, badArgs("there are no unread messages there")
		}
		p := Proposal{}
		if len(rows) > maxItems {
			rows = rows[:maxItems]
			p.Warnings = append(p.Warnings, "more_unread")
		}
		for _, r := range rows {
			p.Items = append(p.Items, Item{ID: r.ID, Map: r.Map, Label: resolveLocalized(r.Name, s.Locale)})
		}
		return Result{Proposal: &p}, nil
	},
}

var proposeUnpinMessage = Tool{
	Name: "propose_unpin_message",
	Description: "Prepare to unpin the instructions pinned on a map, so publishers stop seeing them. Nothing " +
		"changes until the asker confirms on a card. Use it only when the admin asks to unpin or take down an " +
		"instruction.",
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
		var rows []struct {
			ID      string `db:"id"`
			Message string `db:"message"`
		}
		err = s.App.DB().NewQuery(`SELECT id, COALESCE(message, '') AS message FROM messages
			WHERE map = {:map} AND pinned = TRUE AND type = 'administrator' ORDER BY created`).
			Bind(dbx.Params{"map": target.ID}).All(&rows)
		if err != nil {
			return Result{}, err
		}
		if len(rows) == 0 {
			return Result{}, badArgs("%s has no pinned instruction", target.Name)
		}
		p := Proposal{Target: &target}
		for _, r := range rows {
			p.Items = append(p.Items, Item{ID: r.ID, Map: target.ID, Label: r.Message})
		}
		return Result{Proposal: &p}, nil
	},
}

var proposeMapLocation = Tool{
	Name: "propose_map_location",
	Description: "Prepare to set a map's location to where the admin is standing now. Call it even if no " +
		"location has been shared: the call is what lets the admin share it. Nothing changes until the admin " +
		"confirms on a card. Use it only when the admin asks to set a map's location to theirs.",
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
		if s.Origin == nil {
			return Result{}, needLocationError{}
		}
		var location string
		if err := s.App.DB().NewQuery(`SELECT COALESCE(coordinates, '') FROM maps WHERE id = {:id}`).
			Bind(dbx.Params{"id": target.ID}).Row(&location); err != nil {
			return Result{}, err
		}
		p := Proposal{
			Target: &target, Previous: location,
		}
		if old, ok := parsePoint(location); ok && distanceMeters(old, *s.Origin) > farMeters {
			p.Warnings = append(p.Warnings, "moves_far")
		}
		return Result{Proposal: &p}, nil
	},
}

var proposeLinkRevoke = Tool{
	Name: "propose_link_revoke",
	Description: "Prepare to take back a map: delete its active share links, or only a given publisher's. " +
		"Nothing changes until the asker confirms on a card. Use it only when the admin asks to take back a " +
		"map or stop a link.",
	Params: object(map[string]any{
		"map":       mapArg,
		"territory": mapTerritoryArg,
		"publisher": nullable("string", "Only this publisher's links: their token, or the name as the admin wrote it."),
	}),
	Run: func(s Scope, raw json.RawMessage) (Result, error) {
		var args struct {
			mapRef
			Publisher *string `json:"publisher"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return Result{}, err
		}
		target, err := args.target(s)
		if err != nil {
			return Result{}, err
		}
		publisher := ""
		if args.Publisher != nil && strings.TrimSpace(*args.Publisher) != "" {
			if publisher, err = publisherName(s, *args.Publisher); err != nil {
				return Result{}, err
			}
		}
		var links []struct {
			ID        string `db:"id"`
			Publisher string `db:"publisher"`
			Expiry    string `db:"expiry"`
		}
		err = s.App.DB().NewQuery(`SELECT id, COALESCE(publisher, '') AS publisher, expiry_date AS expiry FROM assignments
			WHERE map = {:map} AND expiry_date > {:now}
			  AND ({:publisher} = '' OR lower(trim(publisher)) = lower({:publisher}))
			ORDER BY expiry_date`).
			Bind(dbx.Params{"map": target.ID, "now": dbTime(s.Now), "publisher": publisher}).All(&links)
		if err != nil {
			return Result{}, err
		}
		if len(links) == 0 {
			if publisher != "" {
				return Result{}, badArgs("%s has no active link from that publisher", target.Name)
			}
			return Result{}, badArgs("%s has no active link", target.Name)
		}
		p := Proposal{
			Warnings: []string{"stops_now"},
			Target:   &target,
		}
		for _, l := range links {
			p.Items = append(p.Items, Item{ID: l.ID, Map: target.ID, Label: l.Publisher, Detail: l.Expiry})
		}
		return Result{Proposal: &p}, nil
	},
}

var proposeQuickLink = Tool{
	Name: "propose_quick_link",
	Description: "Prepare a share link for the best map in a territory near where the admin is standing, as " +
		"Quick Link does: the app picks the map when the asker confirms. Call it even if no location has been " +
		"shared: the call is what lets the admin share it. Use it only when the admin asks for a map near them " +
		"for a publisher.",
	Params: object(map[string]any{
		"territory": map[string]any{"type": "string", "description": "The territory's exact code."},
		"publisher": map[string]any{"type": "string", "description": "Who the link is for: their token, or the name as the admin wrote it."},
	}),
	Run: func(s Scope, raw json.RawMessage) (Result, error) {
		var args struct {
			Territory string `json:"territory"`
			Publisher string `json:"publisher"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return Result{}, err
		}
		territoryID, code, err := territoryByCode(s, args.Territory)
		if err != nil {
			return Result{}, err
		}
		publisher, err := publisherName(s, args.Publisher)
		if err != nil {
			return Result{}, err
		}
		if s.Origin == nil {
			return Result{}, needLocationError{}
		}
		return Result{Proposal: &Proposal{
			Target: territoryTarget(territoryID, code),
			Value:  publisher,
		}}, nil
	},
}

// homeState is a home's fields that a home update can change, as the card
// compares them before confirming and restores them on Undo.
type homeState struct {
	Status    string   `json:"status"`
	Notes     string   `json:"notes"`
	Tries     int      `json:"not_home_tries"`
	DncTime   string   `json:"dnc_time"`
	Types     []string `json:"types"`
	TypeNames []string `json:"type_names"`
}

// parseUnit reads "#05-12", "05-12" or "12" into a floor (0 when not given)
// and a code.
func parseUnit(unit string) (int, string) {
	u := strings.TrimPrefix(strings.TrimSpace(unit), "#")
	if before, after, ok := strings.Cut(u, "-"); ok {
		if floor, err := strconv.Atoi(strings.TrimSpace(before)); err == nil {
			return floor, strings.TrimSpace(after)
		}
	}
	return 0, u
}

var proposeHomeUpdate = Tool{
	Name: "propose_home_update",
	Description: "Prepare a change to one home on a map: its status, its note, or its household type. Nothing " +
		"changes until the asker confirms on a card. Use it only when the admin asks to change a home.",
	Params: object(map[string]any{
		"map":            mapArg,
		"territory":      mapTerritoryArg,
		"unit":           map[string]any{"type": "string", "description": "The home as the admin wrote it, such as #05-12 or 23."},
		"status":         nullableEnum("The new status, if it changes.", homeStatuses...),
		"notes_action":   nullableEnum("add appends to the note, replace overwrites it, clear empties it.", "add", "replace", "clear"),
		"notes":          nullable("string", "With add or replace: the note text, in the admin's words."),
		"household_type": nullable("string", "The new household type, by name, replacing the current ones."),
	}),
	Run: func(s Scope, raw json.RawMessage) (Result, error) {
		var args struct {
			mapRef
			Unit          string  `json:"unit"`
			Status        *string `json:"status"`
			NotesAction   *string `json:"notes_action"`
			Notes         *string `json:"notes"`
			HouseholdType *string `json:"household_type"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return Result{}, err
		}
		if args.Status == nil && args.NotesAction == nil && args.HouseholdType == nil {
			return Result{}, badArgs("say what changes: status, notes_action or household_type")
		}
		target, err := args.target(s)
		if err != nil {
			return Result{}, err
		}

		floor, code := parseUnit(args.Unit)
		if code == "" {
			return Result{}, badArgs("unit is required, such as #05-12")
		}
		var homes []struct {
			ID      string `db:"id"`
			Code    string `db:"code"`
			Floor   int    `db:"floor"`
			MapType string `db:"map_type"`
			Status  string `db:"status"`
			Notes   string `db:"notes"`
			Tries   int    `db:"tries"`
			DncTime string `db:"dnc_time"`
		}
		err = s.App.DB().NewQuery(`
			SELECT a.id, a.code, COALESCE(a.floor, 0) AS floor, COALESCE(m.type, '') AS map_type,
			       COALESCE(a.status, '') AS status, COALESCE(a.notes, '') AS notes,
			       COALESCE(a.not_home_tries, 0) AS tries, COALESCE(a.dnc_time, '') AS dnc_time
			FROM addresses a JOIN maps m ON m.id = a.map
			WHERE a.map = {:map} AND ({:floor} = 0 OR a.floor = {:floor})`).
			Bind(dbx.Params{"map": target.ID, "floor": floor}).All(&homes)
		if err != nil {
			return Result{}, err
		}
		match := -1
		for i, h := range homes {
			if normalCode(h.Code) != normalCode(code) {
				continue
			}
			if match >= 0 {
				return Result{}, badArgs("%s has %s on several floors; give the floor, such as #05-%s", target.Name, code, code)
			}
			match = i
		}
		if match < 0 {
			return Result{}, badArgs("%s has no home %s", target.Name, args.Unit)
		}
		h := homes[match]

		var options []struct {
			ID          string `db:"id"`
			Code        string `db:"code"`
			Description string `db:"description"`
			Current     bool   `db:"current"`
		}
		err = s.App.DB().NewQuery(`
			SELECT o.id, COALESCE(o.code, '') AS code, COALESCE(o.description, '') AS description,
			       EXISTS (SELECT 1 FROM address_options ao WHERE ao.address = {:address} AND ao.option = o.id) AS current
			FROM options o WHERE o.congregation = {:congregation} ORDER BY o.sequence`).
			Bind(dbx.Params{"address": h.ID, "congregation": s.CongregationID}).All(&options)
		if err != nil {
			return Result{}, err
		}

		before := homeState{Status: h.Status, Notes: h.Notes, Tries: h.Tries, DncTime: h.DncTime, Types: []string{}, TypeNames: []string{}}
		for _, o := range options {
			if o.Current {
				before.Types = append(before.Types, o.ID)
				before.TypeNames = append(before.TypeNames, resolveLocalized(o.Description, s.Locale))
			}
		}
		after := before

		if args.Status != nil && *args.Status != h.Status {
			after.Status = *args.Status
			switch after.Status {
			case "not_home":
				// The same start the status form gives a first nobody-home visit.
				after.Tries = 1
			case "do_not_call":
				after.DncTime = s.Now.UTC().Format(time.RFC3339)
			}
		}
		if args.NotesAction != nil {
			text := ""
			if args.Notes != nil {
				if text, err = unmaskTokens(s, strings.TrimSpace(*args.Notes)); err != nil {
					return Result{}, err
				}
			}
			switch *args.NotesAction {
			case "add":
				if text == "" {
					return Result{}, badArgs("notes is required with add")
				}
				if after.Notes == "" {
					after.Notes = text
				} else {
					after.Notes += "\n" + text
				}
			case "replace":
				if text == "" {
					return Result{}, badArgs("notes is required with replace; use clear to empty the note")
				}
				after.Notes = text
			case "clear":
				after.Notes = ""
			}
			if utf8.RuneCountInString(after.Notes) > maxNoteRunes {
				return Result{}, badArgs("the note would be over %d characters", maxNoteRunes)
			}
		}
		if args.HouseholdType != nil {
			want := strings.TrimSpace(*args.HouseholdType)
			found := false
			for _, o := range options {
				name := resolveLocalized(o.Description, s.Locale)
				if eqFold(name, want) || eqFold(o.Code, want) ||
					slices.ContainsFunc(nameVariants(o.Description), func(v string) bool { return eqFold(v, want) }) {
					after.Types, after.TypeNames = []string{o.ID}, []string{name}
					found = true
					break
				}
			}
			if !found {
				return Result{}, badArgs("no household type is named %q; use congregation_settings to list them", want)
			}
		}

		if after.Status == before.Status && after.Notes == before.Notes && sameSet(after.Types, before.Types) {
			return Result{}, badArgs("that home already has those details")
		}
		var warnings []string
		if before.Status == "do_not_call" && after.Status != "do_not_call" {
			warnings = append(warnings, "leaves_do_not_call")
		}
		label := home(h.MapType, h.Floor, h.Code)
		if floor > 0 && h.MapType != "multi" {
			// The admin had to name the floor, so the card does too.
			label = fmt.Sprintf("#%02d-%s", h.Floor, h.Code)
		}
		previous, _ := json.Marshal(before)
		value, _ := json.Marshal(after)
		return Result{Proposal: &Proposal{
			Warnings: warnings,
			Target:   &target, From: before.Status, To: after.Status,
			Previous: string(previous), Value: string(value),
			Items: []Item{{ID: h.ID, Map: target.ID, Label: label}},
		}}, nil
	},
}

func sameSet(a, b []string) bool {
	return slices.Equal(slices.Sorted(slices.Values(a)), slices.Sorted(slices.Values(b)))
}
