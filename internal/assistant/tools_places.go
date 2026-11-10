package assistant

import (
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"ganigeorgiev.com/pocketbase/dbx"
)

// coverageStatuses are the status changes that mean a publisher called at a home.
const coverageStatuses = "('done', 'not_home', 'do_not_call')"

var findPlaces = Tool{
	Name: "find_places",
	Description: "Find territories and maps whose code or name contains the text. Use it whenever the admin " +
		"names a place, such as a block, street or territory, and you do not have its exact territory code " +
		"or map name. If nothing matches, try a shorter fragment, such as just the block number.",
	Params: object(map[string]any{
		"text": map[string]any{"type": "string", "description": "Part of a territory code, territory name or map name."},
	}),
	Run: func(s Scope, raw json.RawMessage) (Result, error) {
		var args struct {
			Text string `json:"text"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return Result{}, err
		}
		text := strings.TrimSpace(args.Text)
		if text == "" {
			return Result{}, badArgs("text must not be empty")
		}

		var rows []struct {
			Kind                 string `db:"kind"`
			TerritoryID          string `db:"territory_id"`
			Territory            string `db:"territory"`
			TerritoryDescription string `db:"territory_description"`
			MapID                string `db:"map_id"`
			Map                  string `db:"map"`
			MapType              string `db:"map_type"`
		}
		// A congregation has at most a few hundred places, so they are matched
		// here, where a near miss such as "Marsiling Dr" for "Marsiling Drive" can be caught.
		err := s.App.DB().NewQuery(`
			SELECT 'territory' AS kind, t.id AS territory_id, t.code AS territory,
			       COALESCE(t.description, '') AS territory_description, '' AS map_id, '' AS map, '' AS map_type
			FROM territories t WHERE t.congregation = {:congregation}
			UNION ALL
			SELECT 'map', t.id, t.code, COALESCE(t.description, ''), m.id, COALESCE(m.description, ''), COALESCE(m.type, '')
			FROM maps m JOIN territories t ON t.id = m.territory
			WHERE m.congregation = {:congregation}
		`).Bind(dbx.Params{"congregation": s.CongregationID}).All(&rows)
		if err != nil {
			return Result{}, err
		}

		query := placeWords(text)
		type hit struct {
			row  map[string]any
			rank int // 0 the exact name, 1 a match, 2 and up a near miss by its typos
		}
		var hits []hit
		for _, row := range rows {
			var names, extra []string
			if row.Kind == "territory" {
				names = append([]string{row.Territory}, nameVariants(row.TerritoryDescription)...)
			} else {
				names, extra = nameVariants(row.Map), placeWords(row.Territory)
			}
			rank := -1
			for _, name := range names {
				r := -1
				switch {
				case eqFold(name, text):
					r = 0
				case strings.Contains(strings.ToLower(name), strings.ToLower(text)):
					r = 1
				default:
					if typos, ok := placeMatch(query, placeWords(name), extra); ok {
						r = 1 + typos
					}
				}
				if r >= 0 && (rank < 0 || r < rank) {
					rank = r
				}
			}
			if rank < 0 {
				continue
			}
			hits = append(hits, hit{rank: rank, row: map[string]any{
				"kind": row.Kind, "territory_id": row.TerritoryID, "territory": row.Territory,
				"territory_description": row.TerritoryDescription, "map_id": row.MapID,
				"map": row.Map, "map_type": row.MapType,
			}})
		}
		// Near misses only when nothing matches, closest first after any match.
		sort.SliceStable(hits, func(i, j int) bool { return hits[i].rank < hits[j].rank })
		r := Result{Columns: []string{"kind", "territory", "territory_description", "map", "map_type"}}
		for _, h := range hits {
			if h.rank >= 2 && hits[0].rank < 2 {
				break
			}
			r.Rows = append(r.Rows, h.row)
		}
		if len(hits) > 0 && hits[0].rank >= 2 {
			r.Note = "Nothing matches exactly; these are the closest names. Check with the admin before using one."
		}
		return truncate(r, 15), nil
	},
}

// nameVariants is every language a name is kept in, or the name itself.
func nameVariants(stored string) []string {
	byLocale, ok := localizedNames(stored)
	if !ok {
		return []string{stored}
	}
	names := make([]string, 0, len(byLocale))
	for _, name := range byLocale {
		names = append(names, name)
	}
	return names
}

// territoryByCode resolves an exact territory code within the scope to its id
// and the code as stored. An unknown code is an argument error, so the model
// can look the place up first.
func territoryByCode(s Scope, code string) (id, stored string, err error) {
	var row struct {
		ID   string `db:"id"`
		Code string `db:"code"`
	}
	err = s.App.DB().NewQuery(`SELECT id, code FROM territories WHERE congregation = {:congregation} AND lower(code) = lower({:code}) LIMIT 1`).
		Bind(dbx.Params{"congregation": s.CongregationID, "code": strings.TrimSpace(code)}).One(&row)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", badArgs("no territory has the code %q; use find_places to look it up", code)
	}
	return row.ID, row.Code, err
}

// territoryIDByCode is territoryByCode for an optional filter: no code, no filter.
func territoryIDByCode(s Scope, code *string) (string, error) {
	if code == nil || strings.TrimSpace(*code) == "" {
		return "", nil
	}
	id, _, err := territoryByCode(s, *code)
	return id, err
}
