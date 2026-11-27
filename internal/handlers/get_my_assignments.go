package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"ganigeorgiev.com/pocketbase/dbx"
	"ganigeorgiev.com/pocketbase/pocketbase/core"
)

type myAssignment struct {
	Id               string `json:"id"`
	Type             string `json:"type"`
	ExpiryDate       string `json:"expiry_date"`
	Assigner         string `json:"assigner"`
	MapDescription   any    `json:"map_description"`
	Territory        string `json:"territory"`
	CongregationName string `json:"congregation_name"`
	// The other accounts on the same link, by name, so partners can find
	// each other. Names only, and only for a link the caller holds.
	Partners []string `json:"partners"`
	// The caller made this link and put themself on it, so it is not news to them.
	Own bool `json:"own"`
	// Where the map stands, so the list can say which one still needs work.
	// Counts only. Total is 0 while the map's homes have not been counted,
	// and the others then say nothing.
	NotDone  int `json:"not_done"`
	NotHome  int `json:"not_home"`
	Total    int `json:"total"`
	Progress int `json:"progress"`
	// Pinned administrator messages on the map.
	Pinned int `json:"pinned"`
}

// HandleGetMyAssignments lists the caller's unexpired links: those given to
// their account, in every congregation, and the congregations and groups they
// are a publisher in.
//
// It is a route rather than a list of the assignments collection because a
// publisher holds no staff role, so the map and congregation names an expand
// would carry are hidden from them.
func HandleGetMyAssignments(c *core.RequestEvent, app core.App) error {
	var rows []struct {
		Id               string `db:"id"`
		Type             string `db:"type"`
		ExpiryDate       string `db:"expiry_date"`
		Assigner         string `db:"assigner"`
		MapDescription   string `db:"map_description"`
		Territory        string `db:"territory"`
		CongregationName string `db:"congregation_name"`
		Partners         string `db:"partners"`
		Own              bool   `db:"own"`
		NotDone          int    `db:"not_done"`
		NotHome          int    `db:"not_home"`
		Total            int    `db:"total"`
		Progress         int    `db:"progress"`
		Pinned           int    `db:"pinned"`
	}
	err := app.DB().NewQuery(`
		SELECT a.id, COALESCE(a.type, '') AS type, a.expiry_date,
		       COALESCE(u.name, '')        AS assigner,
		       COALESCE(m.description, '') AS map_description,
		       COALESCE(t.code, '')        AS territory,
		       COALESCE(c.name, '')        AS congregation_name,
		       a.user = {:user}            AS own,
		       COALESCE(m.progress, 0)     AS progress,
		       COALESCE(CASE WHEN json_valid(m.aggregates) THEN json_extract(m.aggregates, '$.notDone') END, 0) AS not_done,
		       COALESCE(CASE WHEN json_valid(m.aggregates) THEN json_extract(m.aggregates, '$.notHome') END, 0) AS not_home,
		       COALESCE(CASE WHEN json_valid(m.aggregates) THEN json_extract(m.aggregates, '$.total') END, 0)   AS total,
		       (SELECT COUNT(*) FROM messages msg
		        WHERE msg.map = m.id AND msg.type = 'administrator' AND msg.pinned = 1) AS pinned,
		       COALESCE((
		         SELECT json_group_array(p.name)
		         FROM json_each(a.assignees) held
		         JOIN users p ON p.id = held.value
		         WHERE held.value != {:user} AND p.name != ''
		       ), '[]') AS partners
		FROM assignments a
		JOIN maps m ON m.id = a.map
		JOIN congregations c ON c.id = a.congregation
		LEFT JOIN territories t ON t.id = m.territory
		LEFT JOIN users u ON u.id = a.user
		WHERE a.expiry_date > datetime('now')
		  AND EXISTS (SELECT 1 FROM json_each(a.assignees) WHERE value = {:user})
		ORDER BY a.expiry_date
	`).Bind(dbx.Params{"user": c.Auth.Id}).All(&rows)
	if err != nil {
		return newServerError(err)
	}

	assignments := make([]myAssignment, len(rows))
	for i, row := range rows {
		partners := []string{}
		if err := json.Unmarshal([]byte(row.Partners), &partners); err != nil {
			return newServerError(err)
		}
		assignments[i] = myAssignment{
			Id:               row.Id,
			Type:             row.Type,
			ExpiryDate:       row.ExpiryDate,
			Assigner:         row.Assigner,
			MapDescription:   localizedText(row.MapDescription),
			Territory:        row.Territory,
			CongregationName: row.CongregationName,
			Partners:         partners,
			Own:              row.Own,
			NotDone:          row.NotDone,
			NotHome:          row.NotHome,
			Total:            row.Total,
			Progress:         row.Progress,
			Pinned:           row.Pinned,
		}
	}

	// A publisher's role row carries no congregation name they may read, and
	// with no link in hand "My maps" says nothing of where they belong. Names
	// only, and only where the caller is a publisher: staff see their
	// congregations in the shell.
	congregations := []string{}
	err = app.DB().NewQuery(`
		SELECT c.name
		FROM roles r
		JOIN congregations c ON c.id = r.congregation
		WHERE r.user = {:user} AND r.role = 'publisher' AND c.name != ''
		ORDER BY c.name
	`).Bind(dbx.Params{"user": c.Auth.Id}).Column(&congregations)
	if err != nil {
		return newServerError(err)
	}

	// Only staff can read groups, so a publisher learns its own groups' names
	// here, for feature flags that target a group.
	groups := []string{}
	err = app.DB().NewQuery(`
		SELECT DISTINCT g.name
		FROM groups g
		JOIN roles r ON r.congregation = g.congregation
		WHERE r.user = {:user} AND r.role = 'publisher'
		  AND EXISTS (SELECT 1 FROM json_each(g.members) WHERE value = {:user})
		ORDER BY g.name
	`).Bind(dbx.Params{"user": c.Auth.Id}).Column(&groups)
	if err != nil {
		return newServerError(err)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"assignments":   assignments,
		"congregations": congregations,
		"groups":        groups,
	})
}

// localizedText passes a stored description through as it is kept: a JSON
// object of translations, or a plain string. Only an object goes out raw:
// json.Valid on its own is too loose, as a numeric name like "713" is a valid
// bare JSON number and would go out unquoted.
func localizedText(stored string) any {
	raw := []byte(stored)
	if strings.HasPrefix(stored, "{") && json.Valid(raw) {
		return json.RawMessage(raw)
	}
	return stored
}
