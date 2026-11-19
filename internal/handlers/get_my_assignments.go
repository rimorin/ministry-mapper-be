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
}

// HandleGetMyAssignments lists the caller's unexpired links: those given to
// their account, in every congregation.
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
	}
	err := app.DB().NewQuery(`
		SELECT a.id, COALESCE(a.type, '') AS type, a.expiry_date,
		       COALESCE(u.name, '')        AS assigner,
		       COALESCE(m.description, '') AS map_description,
		       COALESCE(t.code, '')        AS territory,
		       COALESCE(c.name, '')        AS congregation_name,
		       a.user = {:user}            AS own,
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
		}
	}
	return c.JSON(http.StatusOK, map[string]any{"assignments": assignments})
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
