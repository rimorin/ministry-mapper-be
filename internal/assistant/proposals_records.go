package assistant

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	"ganigeorgiev.com/pocketbase/dbx"
)

const (
	maxNameRunes      = 200
	maxMessageRunes   = 1000
	maxPublisherRunes = 100
	maxLinkDays       = 365
	defaultLinkHours  = 24
)

// mapTarget finds a map by its exact name, in a territory when one is given.
func mapTarget(s Scope, name string, territory *string) (Target, string, error) {
	if strings.TrimSpace(name) == "" {
		return Target{}, "", badArgs("map is required; use find_places to look it up")
	}
	territoryID, err := territoryIDByCode(s, territory)
	if err != nil {
		return Target{}, "", err
	}
	id, err := mapIDByName(s, territoryID, &name)
	if err != nil {
		return Target{}, "", err
	}
	var row struct {
		Description string `db:"description"`
		Territory   string `db:"territory"`
	}
	if err := s.App.DB().NewQuery(`SELECT COALESCE(description, '') AS description, territory FROM maps WHERE id = {:id}`).
		Bind(dbx.Params{"id": id}).One(&row); err != nil {
		return Target{}, "", err
	}
	return Target{ID: id, Name: resolveLocalized(row.Description, s.Locale), TerritoryID: row.Territory},
		row.Description, nil
}

// mapRef is the map, and optionally its territory, a function's arguments name.
type mapRef struct {
	Map       string  `json:"map"`
	Territory *string `json:"territory"`
}

func (r mapRef) target(s Scope) (Target, error) {
	target, _, err := mapTarget(s, r.Map, r.Territory)
	return target, err
}

// renamed returns what to store for a new map name. A name kept in several
// languages changes only in the one the admin sees; the others stay.
func renamed(previous, newName, locale string) string {
	byLocale, ok := localizedNames(previous)
	if !ok {
		return newName
	}
	byLocale[localeKey(byLocale, locale)] = newName
	out, _ := json.Marshal(byLocale)
	return string(out)
}

func newName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" || utf8.RuneCountInString(name) > maxNameRunes {
		return "", badArgs("new_name must be 1 to %d characters", maxNameRunes)
	}
	return name, nil
}

var proposeRename = Tool{
	Name: "propose_rename",
	Description: "Prepare to rename a map, or to change a territory's name (not its code). Nothing changes until " +
		"the asker confirms on a card. Use it only when the admin asks to rename a map or a territory.",
	Params: object(map[string]any{
		"what":      enumOf("Whether a map or a territory is renamed.", "map", "territory"),
		"name":      map[string]any{"type": "string", "description": "The map's exact current name, or the territory's exact code. Use find_places first if unsure."},
		"territory": mapTerritoryArg,
		"new_name":  map[string]any{"type": "string", "description": "The new name, exactly as the admin wants it."},
	}),
	Run: func(s Scope, raw json.RawMessage) (Result, error) {
		var args struct {
			What      string  `json:"what"`
			Name      string  `json:"name"`
			Territory *string `json:"territory"`
			NewName   string  `json:"new_name"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return Result{}, err
		}
		name, err := newName(args.NewName)
		if err != nil {
			return Result{}, err
		}
		if args.What == "territory" {
			territoryID, code, err := territoryByCode(s, args.Name)
			if err != nil {
				return Result{}, err
			}
			var description string
			if err := s.App.DB().NewQuery(`SELECT COALESCE(description, '') FROM territories WHERE id = {:id}`).
				Bind(dbx.Params{"id": territoryID}).Row(&description); err != nil {
				return Result{}, err
			}
			if name == description {
				return Result{}, badArgs("the territory is already named %q", name)
			}
			return Result{Proposal: &Proposal{
				Kind:   "territory_rename",
				Target: territoryTarget(territoryID, code),
				From:   description, To: name, Value: name, Previous: description,
			}}, nil
		}
		target, previous, err := mapTarget(s, args.Name, args.Territory)
		if err != nil {
			return Result{}, err
		}
		if name == target.Name {
			return Result{}, badArgs("the map is already named %q", name)
		}
		return Result{Proposal: &Proposal{
			Kind:   "map_rename",
			Target: &target, From: target.Name, To: name,
			Value: renamed(previous, name, s.Locale), Previous: previous,
		}}, nil
	},
}

// namesByToken maps each person token back to the name it stands for. Ask
// loads it once per question; other callers query it here.
func namesByToken(s Scope) (map[string]string, error) {
	if s.knownTokens != nil {
		return s.knownTokens, nil
	}
	known, err := knownPeople(s)
	if err != nil {
		return nil, err
	}
	return tokensFor(s.CongregationID, known), nil
}

func tokensFor(congregationID string, known []string) map[string]string {
	people := newPseudonyms(congregationID)
	names := make(map[string]string, len(known))
	for _, k := range known {
		names[people.token(k)] = strings.TrimSpace(k)
	}
	return names
}

// RestoreNames turns the person tokens in logged answers back into names, to
// show a past conversation to the asker again, and returns the tokens they used
// with their names, so a continued conversation can send them back. What is
// logged keeps the tokens.
func RestoreNames(s Scope, answers []string) ([]string, map[string]string, error) {
	names, err := namesByToken(s)
	if err != nil {
		return nil, nil, err
	}
	used := map[string]string{}
	restored := make([]string, len(answers))
	for i, answer := range answers {
		restored[i] = replaceTokens(answer, names, used)
	}
	return restored, used, nil
}

// unmaskTokens puts back the names that person tokens stand for. The model
// writes a message or note with the tokens it was given, but what is stored
// must say what the admin typed.
func unmaskTokens(s Scope, text string) (string, error) {
	if !tokenPattern.MatchString(text) {
		return text, nil
	}
	names, err := namesByToken(s)
	if err != nil {
		return "", err
	}
	return replaceTokens(text, names, nil), nil
}

// publisherName turns the model's person token back into the name it stands
// for; a name the records do not know arrives as typed.
func publisherName(s Scope, raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if tokenPattern.MatchString(name) {
		names, err := namesByToken(s)
		if err != nil {
			return "", err
		}
		if known, ok := names[name]; ok {
			return known, nil
		}
		return "", badArgs("write the publisher's name as the admin gave it")
	}
	if name == "" || utf8.RuneCountInString(name) > maxPublisherRunes {
		return "", badArgs("publisher must be 1 to %d characters", maxPublisherRunes)
	}
	return name, nil
}

var proposeShareLink = Tool{
	Name: "propose_share_link",
	Description: "Prepare a share link that gives a publisher a map: normal for the congregation's usual hours, " +
		"or personal for a number of days. Nothing is created until the asker confirms on a card, which then " +
		"lets them share the link. Use it only when the admin asks to give someone a map.",
	Params: object(map[string]any{
		"map":       mapArg,
		"territory": mapTerritoryArg,
		"publisher": map[string]any{"type": "string", "description": "Who the link is for: their token, or the name as the admin wrote it."},
		"type":      enumOf("normal for a usual link, personal for personal territory.", "normal", "personal"),
		"days":      nullable("integer", "With personal: how many days the link lasts, 1 to 365."),
	}),
	Run: func(s Scope, raw json.RawMessage) (Result, error) {
		var args struct {
			mapRef
			Publisher string `json:"publisher"`
			Type      string `json:"type"`
			Days      *int   `json:"days"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return Result{}, err
		}
		publisher, err := publisherName(s, args.Publisher)
		if err != nil {
			return Result{}, err
		}
		target, err := args.target(s)
		if err != nil {
			return Result{}, err
		}

		hours := 0
		switch args.Type {
		case "personal":
			if args.Days == nil || *args.Days < 1 || *args.Days > maxLinkDays {
				return Result{}, badArgs("a personal link needs days between 1 and %d; ask the admin how long", maxLinkDays)
			}
			hours = *args.Days * 24
		case "normal":
			if err := s.App.DB().NewQuery(`SELECT COALESCE(expiry_hours, 0) FROM congregations WHERE id = {:id}`).
				Bind(dbx.Params{"id": s.CongregationID}).Row(&hours); err != nil {
				return Result{}, err
			}
			if hours <= 0 {
				hours = defaultLinkHours
			}
		default:
			return Result{}, badArgs("type must be normal or personal")
		}

		var warnings []string
		var active int
		if err := s.App.DB().NewQuery(`SELECT COUNT(*) FROM assignments WHERE map = {:map} AND expiry_date > {:now}`).
			Bind(dbx.Params{"map": target.ID, "now": dbTime(s.Now)}).Row(&active); err != nil {
			return Result{}, err
		}
		if active > 0 {
			warnings = append(warnings, "already_assigned")
		}
		return Result{Proposal: &Proposal{
			Warnings: warnings,
			Target:   &target, Value: publisher, LinkType: args.Type, Hours: hours,
		}}, nil
	},
}

var proposePinMessage = Tool{
	Name: "propose_pin_message",
	Description: "Prepare an instruction pinned on a map for publishers. Pinned instructions are emailed to the " +
		"congregation. Nothing is posted until the asker confirms on a card. Use it only when the admin asks to " +
		"pin or post an instruction.",
	Params: object(map[string]any{
		"map":       mapArg,
		"territory": mapTerritoryArg,
		"message":   map[string]any{"type": "string", "description": "The instruction, in the admin's words."},
	}),
	Run: func(s Scope, raw json.RawMessage) (Result, error) {
		var args struct {
			mapRef
			Message string `json:"message"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return Result{}, err
		}
		message, err := unmaskTokens(s, strings.TrimSpace(args.Message))
		if err != nil {
			return Result{}, err
		}
		if message == "" || utf8.RuneCountInString(message) > maxMessageRunes {
			return Result{}, badArgs("message must be 1 to %d characters", maxMessageRunes)
		}
		target, err := args.target(s)
		if err != nil {
			return Result{}, err
		}
		return Result{Proposal: &Proposal{
			Warnings: []string{"emails_everyone"},
			Target:   &target, Value: message,
		}}, nil
	},
}

var proposeReportEmail = Tool{
	Name: "propose_report_email",
	Description: "Prepare to email the admin the activity report (the same as Generate Report). Nothing is sent " +
		"until the asker confirms on a card. Use it only when the admin asks for the report.",
	Params: object(map[string]any{}),
	Run: func(s Scope, _ json.RawMessage) (Result, error) {
		return Result{Proposal: &Proposal{}}, nil
	},
}
