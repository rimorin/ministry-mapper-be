// Package links answers questions about share links that both the routes and
// the assistant ask.
package links

import (
	"encoding/json"
	"strings"

	"ganigeorgiev.com/pocketbase/dbx"
	"ganigeorgiev.com/pocketbase/pocketbase/core"
)

// Held is an open link that already carries someone a new link is for.
type Held struct {
	// Publisher is who the open link was made out to, as typed on it.
	Publisher string `json:"publisher"`
	// Map is the map's name as stored, which may hold several languages.
	Map string `json:"map"`
}

// HeldInTerritory lists the open normal links in a territory that already
// carry one of these people: by account where the link was given to accounts,
// and by name as written otherwise. A link names several people separated by
// commas, and each is compared on their own, ignoring case and spacing.
//
// It is for telling the conductor, not for refusing: a pair that finished its
// map early still holds that link when it asks for the next one.
func HeldInTerritory(app core.App, territoryID, congregationID string, names, accounts []string) ([]Held, error) {
	wanted := map[string]bool{}
	for _, name := range names {
		if n := normalName(name); n != "" {
			wanted[n] = true
		}
	}
	if len(wanted) == 0 && len(accounts) == 0 {
		return []Held{}, nil
	}
	var rows []struct {
		Publisher string `db:"publisher"`
		Assignees string `db:"assignees"`
		Map       string `db:"map"`
	}
	err := app.DB().NewQuery(`
		SELECT COALESCE(a.publisher, '') AS publisher, COALESCE(a.assignees, '') AS assignees,
		       COALESCE(m.description, '') AS map
		FROM assignments a JOIN maps m ON m.id = a.map
		WHERE m.territory = {:territory} AND a.congregation = {:congregation}
		  AND a.type = 'normal' AND a.expiry_date > datetime('now')
		ORDER BY a.created, a.id
	`).Bind(dbx.Params{"territory": territoryID, "congregation": congregationID}).All(&rows)
	if err != nil {
		return nil, err
	}
	held := []Held{}
	for _, row := range rows {
		if sharesPerson(row.Publisher, row.Assignees, wanted, accounts) {
			held = append(held, Held{Publisher: row.Publisher, Map: row.Map})
		}
	}
	return held, nil
}

func sharesPerson(publisher, assignees string, wanted map[string]bool, accounts []string) bool {
	for _, name := range strings.Split(publisher, ",") {
		if wanted[normalName(name)] {
			return true
		}
	}
	var onLink []string
	_ = json.Unmarshal([]byte(assignees), &onLink) // empty on a link given to nobody
	for _, id := range onLink {
		for _, account := range accounts {
			if id == account {
				return true
			}
		}
	}
	return false
}

func normalName(name string) string {
	return strings.ToLower(strings.Join(strings.Fields(name), " "))
}
