package assistant

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"
	"unicode/utf8"

	"ganigeorgiev.com/pocketbase/dbx"
)

// A Proposal is a change the assistant prepared but did not make. The panel
// shows it on a card; only the admin's Confirm runs it, in the browser,
// through the same calls the admin screens use. Names and emails are for the
// card; the model sees only the kind, the roles and the warnings.
type Proposal struct {
	ID string `json:"id"`
	// Kind is the tool's name without "propose_", or role_revoke.
	Kind string `json:"kind"`
	// The people it could apply to. A change or removal names one; a grant
	// lists every account the search found, for the admin to pick.
	Candidates []Candidate `json:"candidates"`
	From       string      `json:"from"` // current role; "" for a grant
	To         string      `json:"to"`   // new role; "" for a removal
	// Warnings the card shows: self (the admin's own role), removes_admin.
	Warnings []string `json:"warnings"`
	// Blocked explains why the change cannot be confirmed (last_admin).
	Blocked string `json:"blocked,omitempty"`

	// For changes to a map, territory or the congregation's records:
	// Target is what the change applies to; Value is what will be stored (a
	// name, a message, a publisher) and Previous what was stored, for the
	// re-check before confirming and for Undo.
	Target   *Target `json:"target,omitempty"`
	Value    string  `json:"value,omitempty"`
	Previous string  `json:"previous,omitempty"`
	// Share links: normal or personal, lasting Hours.
	LinkType string `json:"linkType,omitempty"`
	Hours    int    `json:"hours,omitempty"`
	// The records a change touches: messages, links or homes.
	Items []Item `json:"items,omitempty"`
	// What the card shows for changes to a map's structure or many homes:
	// how many homes a reset touches, the floor and units added, the
	// do-not-call cut-off.
	Data map[string]any `json:"data,omitempty"`
}

type Item struct {
	ID     string `json:"id"`
	Map    string `json:"map"`
	Label  string `json:"label"`
	Detail string `json:"detail,omitempty"`
}

type Target struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	TerritoryID string `json:"territoryId"`
}

func territoryTarget(id, code string) *Target {
	return &Target{ID: id, Name: code, TerritoryID: id}
}

type Candidate struct {
	UserID string `json:"userId"`
	RoleID string `json:"roleId,omitempty"`
	Name   string `json:"name"`
	Email  string `json:"email"`
	// For a change to someone's role: their current role, and the warnings
	// and block that apply to them, since same-named people can differ.
	Role     string   `json:"role,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
	Blocked  string   `json:"blocked,omitempty"`
}

const (
	maxCandidates      = 10
	minUserSearchRunes = 3
)

var assignableRoles = []string{"administrator", "conductor", "read_only"}

// Card is a prepared change as the question log keeps it, so a conversation
// reopened from history shows it again and, if it was never acted on, can still
// be confirmed: the whole proposal as the panel received it (confirming
// re-checks its record first), and what became of it. Outcome is empty until
// the asker acts on it; Person is the account a role change ended up
// applying to (cards logged before it held the name).
type Card struct {
	Proposal Proposal `json:"proposal"`
	Outcome  string   `json:"outcome"`
	Person   string   `json:"person,omitempty"`
	// Record is the share link a link card created, and Label the map a quick
	// link picked, so history can share the link again while it still works.
	Record string `json:"record,omitempty"`
	Label  string `json:"label,omitempty"`
}

// Cards turns an answer's proposals into the records history keeps.
func Cards(proposals []Proposal) []Card {
	cards := make([]Card, len(proposals))
	for i, p := range proposals {
		cards[i] = Card{Proposal: p}
	}
	return cards
}

func proposalID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// proposalView is the model's copy: no names, emails or ids.
func proposalView(p Proposal) string {
	view := map[string]any{
		"proposal":   "prepared; the admin must confirm it on the card, nothing has changed yet",
		"kind":       p.Kind,
		"from":       p.From,
		"to":         p.To,
		"candidates": len(p.Candidates),
		"warnings":   p.Warnings,
		"blocked":    p.Blocked,
	}
	if p.Target != nil {
		view["target"] = p.Target.Name
	}
	if len(p.Items) > 0 {
		view["items"] = len(p.Items)
	}
	// New maps: how many and how many homes, not their units.
	if maps, ok := p.Data["maps"].([]map[string]any); ok {
		view["maps"], view["homes"] = len(maps), p.Data["homes"]
	}
	if p.LinkType != "" {
		view["link_type"], view["hours"] = p.LinkType, p.Hours
	}
	out, _ := json.Marshal(view)
	return string(out)
}

var proposeRoleChange = Tool{
	Name: "propose_role_change",
	Description: "Prepare a change to the access of someone who already has a role in this congregation: a new " +
		"role, or role none to remove their access. Nothing changes until the asker confirms on a card. " +
		"Use it only when the admin asks for the change.",
	Params: object(map[string]any{
		"person": map[string]any{"type": "string", "description": "The person's token as shown in results, such as Person QXWB."},
		"role":   enumOf("The new role, or none to remove access.", slices.Concat(assignableRoles, []string{"none"})...),
	}),
	Run: func(s Scope, raw json.RawMessage) (Result, error) {
		var args struct {
			Person string `json:"person"`
			Role   string `json:"role"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return Result{}, err
		}
		var roles []struct {
			RoleID   string `db:"role_id"`
			UserID   string `db:"user_id"`
			Name     string `db:"name"`
			Email    string `db:"email"`
			Role     string `db:"role"`
			Disabled bool   `db:"disabled"`
		}
		err := s.App.DB().NewQuery(`
			SELECT r.id AS role_id, u.id AS user_id, COALESCE(u.name, '') AS name, u.email, r.role, u.disabled
			FROM roles r JOIN users u ON u.id = r.user
			WHERE r.congregation = {:congregation}
		`).Bind(dbx.Params{"congregation": s.CongregationID}).All(&roles)
		if err != nil {
			return Result{}, err
		}

		// The model refers to people by token; a token depends only on the
		// congregation and the name, so it can be recomputed here.
		people := newPseudonyms(s.CongregationID)
		want := strings.TrimSpace(args.Person)
		var matches []int
		for i, r := range roles {
			if people.token(r.Name) == want || (want != "" && strings.EqualFold(strings.TrimSpace(r.Name), want)) {
				matches = append(matches, i)
			}
		}
		if len(matches) == 0 {
			return Result{}, badArgs("nobody with access matches %q; use people_and_roles to find them", args.Person)
		}

		p := Proposal{To: args.Role}
		if args.Role == "none" {
			p.Kind, p.To = "role_revoke", ""
		}
		// Same-named people all share one token; the card makes the admin
		// pick, and each carries their own role, warnings and block.
		for _, i := range matches {
			r := roles[i]
			if r.Role == p.To {
				continue
			}
			c := Candidate{UserID: r.UserID, RoleID: r.RoleID, Name: r.Name, Email: r.Email, Role: r.Role}
			if r.UserID == s.UserID {
				c.Warnings = append(c.Warnings, "self")
			}
			if r.Role == "administrator" {
				c.Warnings = append(c.Warnings, "removes_admin")
				others := 0
				for _, other := range roles {
					if other.RoleID != r.RoleID && other.Role == "administrator" && !other.Disabled {
						others++
					}
				}
				if others == 0 {
					c.Blocked = "last_admin"
				}
			}
			p.Candidates = append(p.Candidates, c)
		}
		switch len(p.Candidates) {
		case 0:
			return Result{}, badArgs("that person already has the role %s", roles[matches[0]].Role)
		case 1:
			c := p.Candidates[0]
			p.From, p.Warnings, p.Blocked = c.Role, c.Warnings, c.Blocked
		}
		return Result{Proposal: &p}, nil
	},
}

var proposeRoleGrant = Tool{
	Name: "propose_role_grant",
	Description: "Prepare to give a role to someone who has an account in the app but no role in this " +
		"congregation, found by the start of their name or email. Nothing changes until the asker picks the " +
		"person and confirms on a card. Use it only when the admin asks to give someone access.",
	Params: object(map[string]any{
		"search": map[string]any{"type": "string", "description": "The start of the person's name or email, as the admin wrote it; at least 3 characters."},
		"role":   enumOf("The role to give.", assignableRoles...),
	}),
	Run: func(s Scope, raw json.RawMessage) (Result, error) {
		var args struct {
			Search string `json:"search"`
			Role   string `json:"role"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return Result{}, err
		}
		search := strings.TrimSpace(args.Search)
		if utf8.RuneCountInString(search) < minUserSearchRunes {
			return Result{}, badArgs("search must be at least %d characters", minUserSearchRunes)
		}
		// The same search as Invite User: the start of the name or the email.
		var users []struct {
			UserID  string `db:"user_id"`
			Name    string `db:"name"`
			Email   string `db:"email"`
			HasRole bool   `db:"has_role"`
		}
		err := s.App.DB().NewQuery(`
			SELECT u.id AS user_id, COALESCE(u.name, '') AS name, u.email,
			       EXISTS (SELECT 1 FROM roles r WHERE r.user = u.id AND r.congregation = {:congregation}) AS has_role
			FROM users u
			WHERE u.disabled = FALSE AND u.id != {:self}
			  AND (lower(u.email) LIKE lower({:prefix}) ESCAPE '\' OR lower(u.name) LIKE lower({:prefix}) ESCAPE '\')
			ORDER BY lower(u.name)
			LIMIT 50
		`).Bind(dbx.Params{"congregation": s.CongregationID, "self": s.UserID, "prefix": likePrefix(search)}).All(&users)
		if err != nil {
			return Result{}, err
		}
		p := Proposal{To: args.Role}
		alreadyIn := 0
		for _, u := range users {
			if u.HasRole {
				alreadyIn++
				continue
			}
			if len(p.Candidates) < maxCandidates {
				p.Candidates = append(p.Candidates, Candidate{UserID: u.UserID, Name: u.Name, Email: u.Email})
			}
		}
		if len(p.Candidates) == 0 {
			if alreadyIn > 0 {
				return Result{}, badArgs("everyone matching %q already has a role here; use propose_role_change to change it", search)
			}
			return Result{}, badArgs("no account matches %q; the person must sign up in the app first", search)
		}
		return Result{Proposal: &p}, nil
	},
}

// likePrefix escapes LIKE wildcards in what the admin typed and matches it as
// a prefix.
func likePrefix(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s) + "%"
}
