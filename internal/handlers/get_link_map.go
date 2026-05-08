package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

type linkMapOption struct {
	Id          string `db:"id"          json:"id"`
	Code        string `db:"code"         json:"code"`
	Description string `db:"description"  json:"description"`
	IsCountable bool   `db:"is_countable" json:"is_countable"`
	IsDefault   bool   `db:"is_default"   json:"is_default"`
	Sequence    int    `db:"sequence"     json:"sequence"`
}

type linkMapDetails struct {
	Id          string `json:"id"`
	Description any    `json:"description"`
	Type        string `json:"type"`
	Coordinates any    `json:"coordinates"`
	Progress    int    `json:"progress"`
	Aggregates  any    `json:"aggregates"`
	Territory   string `json:"territory"`
}

type linkMapCongregation struct {
	Id          string          `json:"id"`
	MaxTries    int             `json:"max_tries"`
	Origin      string          `json:"origin"`
	ExpiryHours int             `json:"expiry_hours"`
	Options     []linkMapOption `json:"options"`
}

type linkMapResponse struct {
	ExpiryDate        string              `json:"expiry_date"`
	Publisher         string              `json:"publisher"`
	Map               linkMapDetails      `json:"map"`
	Congregation      linkMapCongregation `json:"congregation"`
	Addresses         []addressResponse   `json:"addresses"`
	HasPinnedMessages bool                `json:"has_pinned_messages"`
}

func HandleGetLinkMap(c *core.RequestEvent, app core.App) error {
	linkId := c.Request.Header.Get("link-id")
	if linkId == "" {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
	}

	// Single query: auth check + map + congregation (3 PK lookups via JOINs).
	// A matching row means the link-id is valid and non-expired.
	var row struct {
		// assignment
		Map          string `db:"map"`
		Publisher    string `db:"publisher"`
		Congregation string `db:"congregation"`
		ExpiryDate   string `db:"expiry_date"`
		// map
		Description string `db:"description"`
		Type        string `db:"type"`
		Coordinates string `db:"coordinates"`
		Progress    int    `db:"progress"`
		Aggregates  string `db:"aggregates"`
		Territory   string `db:"territory"`
		// congregation
		MaxTries    int    `db:"max_tries"`
		Origin      string `db:"origin"`
		ExpiryHours int    `db:"expiry_hours"`
	}
	if err := app.DB().NewQuery(`
		SELECT a.map, a.publisher, a.congregation, a.expiry_date,
		       COALESCE(m.description, '')  AS description,
		       COALESCE(m.type, '')         AS type,
		       COALESCE(m.coordinates, '')  AS coordinates,
		       COALESCE(m.progress, 0)      AS progress,
		       COALESCE(m.aggregates, '')   AS aggregates,
		       COALESCE(m.territory, '')    AS territory,
		       COALESCE(c.max_tries, 1)     AS max_tries,
		       COALESCE(c.origin, '')       AS origin,
		       COALESCE(c.expiry_hours, 24) AS expiry_hours
		FROM assignments a
		JOIN maps m ON m.id = a.map
		JOIN congregations c ON c.id = a.congregation
		WHERE a.id = {:linkId} AND a.expiry_date > datetime('now')
		LIMIT 1
	`).Bind(dbx.Params{"linkId": linkId}).One(&row); err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
	}

	// Remaining 4 queries are independent — run them concurrently.
	// SQLite WAL mode allows concurrent readers across the connection pool.
	type optsResult struct {
		v   []linkMapOption
		err error
	}
	type addrsResult struct {
		v   []addressRow
		err error
	}
	type addrOptsResult struct {
		v   []addressOption
		err error
	}
	type pinnedResult struct {
		v bool
	}

	optsCh := make(chan optsResult, 1)
	addrCh := make(chan addrsResult, 1)
	addrOptsCh := make(chan addrOptsResult, 1)
	pinnedCh := make(chan pinnedResult, 1)

	congParams := dbx.Params{"congregation": row.Congregation}
	mapParams := dbx.Params{"map": row.Map}

	go func() {
		var v []linkMapOption
		err := app.DB().NewQuery(`
			SELECT id, code, description, is_countable, is_default, sequence
			FROM options
			WHERE congregation = {:congregation}
			ORDER BY sequence ASC
		`).Bind(congParams).All(&v)
		if v == nil {
			v = []linkMapOption{}
		}
		optsCh <- optsResult{v, err}
	}()

	go func() {
		var v []addressRow
		err := app.DB().NewQuery(`
			SELECT id, code, floor, sequence, status, notes, not_home_tries, dnc_time,
			       COALESCE(coordinates, '') AS coordinates, updated, updated_by
			FROM addresses
			WHERE map = {:map}
		`).Bind(mapParams).All(&v)
		addrCh <- addrsResult{v, err}
	}()

	go func() {
		var v []addressOption
		err := app.DB().NewQuery(`
			SELECT id, address, option
			FROM address_options
			WHERE map = {:map}
		`).Bind(mapParams).All(&v)
		addrOptsCh <- addrOptsResult{v, err}
	}()

	go func() {
		var check struct {
			V int `db:"v"`
		}
		err := app.DB().NewQuery(`
			SELECT 1 AS v FROM messages
			WHERE map = {:map} AND type = 'administrator' AND pinned = 1
			LIMIT 1
		`).Bind(mapParams).One(&check)
		pinnedCh <- pinnedResult{err == nil}
	}()

	optsRes := <-optsCh
	addrRes := <-addrCh
	addrOptsRes := <-addrOptsCh
	pinnedRes := <-pinnedCh

	if optsRes.err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to fetch options"})
	}
	if addrRes.err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to fetch addresses"})
	}
	if addrOptsRes.err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to fetch address options"})
	}

	// Build address response
	optionMap := make(map[string][]addressOption, len(addrOptsRes.v))
	for _, opt := range addrOptsRes.v {
		optionMap[opt.Address] = append(optionMap[opt.Address], opt)
	}

	addressResult := make([]addressResponse, len(addrRes.v))
	for i, addr := range addrRes.v {
		opts := optionMap[addr.Id]
		if opts == nil {
			opts = []addressOption{}
		}
		var coords any
		if addr.Coordinates != "" {
			coords = json.RawMessage(addr.Coordinates)
		}
		addressResult[i] = addressResponse{
			Id:           addr.Id,
			Code:         addr.Code,
			Floor:        addr.Floor,
			Sequence:     addr.Sequence,
			Status:       addr.Status,
			Notes:        addr.Notes,
			NotHomeTries: addr.NotHomeTries,
			DncTime:      addr.DncTime,
			Coordinates:  coords,
			Updated:      addr.Updated,
			UpdatedBy:    addr.UpdatedBy,
			Options:      opts,
		}
	}

	// description may be a plain string (older records) or a JSON object (localized).
	// Use json.Valid to distinguish: raw pass-through for JSON, string encoding for plain text.
	var mapDescription any
	if row.Description != "" {
		raw := []byte(row.Description)
		if json.Valid(raw) {
			mapDescription = json.RawMessage(raw)
		} else {
			mapDescription = row.Description
		}
	}
	var mapCoordinates any
	if row.Coordinates != "" {
		mapCoordinates = json.RawMessage(row.Coordinates)
	}
	var mapAggregates any
	if row.Aggregates != "" {
		mapAggregates = json.RawMessage(row.Aggregates)
	}

	return c.JSON(http.StatusOK, linkMapResponse{
		ExpiryDate: row.ExpiryDate,
		Publisher:  row.Publisher,
		Map: linkMapDetails{
			Id:          row.Map,
			Description: mapDescription,
			Type:        row.Type,
			Coordinates: mapCoordinates,
			Progress:    row.Progress,
			Aggregates:  mapAggregates,
			Territory:   row.Territory,
		},
		Congregation: linkMapCongregation{
			Id:          row.Congregation,
			MaxTries:    row.MaxTries,
			Origin:      row.Origin,
			ExpiryHours: row.ExpiryHours,
			Options:     optsRes.v,
		},
		Addresses:         addressResult,
		HasPinnedMessages: pinnedRes.v,
	})
}
