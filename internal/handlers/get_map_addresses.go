package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

type GetMapAddressesRequest struct {
	MapId string `json:"map_id"`
}

type addressOption struct {
	Id      string `db:"id" json:"aoId"`
	Address string `db:"address" json:"-"`
	Option  string `db:"option" json:"id"`
}

type addressRow struct {
	Id           string `db:"id" json:"id"`
	Code         string `db:"code" json:"code"`
	Floor        int    `db:"floor" json:"floor"`
	Sequence     int    `db:"sequence" json:"sequence"`
	Status       string `db:"status" json:"status"`
	Notes        string `db:"notes" json:"notes"`
	NotHomeTries int    `db:"not_home_tries" json:"not_home_tries"`
	DncTime      string `db:"dnc_time" json:"dnc_time"`
	Coordinates  string `db:"coordinates" json:"coordinates"`
	Updated      string `db:"updated" json:"updated"`
	UpdatedBy    string `db:"updated_by" json:"updated_by"`
}

type addressResponse struct {
	Id           string          `json:"id"`
	Code         string          `json:"code"`
	Floor        int             `json:"floor"`
	Sequence     int             `json:"sequence"`
	Status       string          `json:"status"`
	Notes        string          `json:"notes"`
	NotHomeTries int             `json:"not_home_tries"`
	DncTime      string          `json:"dnc_time"`
	Coordinates  any             `json:"coordinates"`
	Updated      string          `json:"updated"`
	UpdatedBy    string          `json:"updated_by"`
	Options      []addressOption `json:"options"`
}

func HandleGetMapAddresses(c *core.RequestEvent, app core.App) error {
	data := GetMapAddressesRequest{}
	if err := c.BindBody(&data); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}

	if data.MapId == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "map_id is required"})
	}

	if _, err := fetchMapData(app, data.MapId); err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Map not found"})
	}

	if !AuthorizeMapAccess(c, app, data.MapId) {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Unauthorized"})
	}

	params := dbx.Params{"map": data.MapId}

	// Query 1: addresses (~14ms on heaviest map, index-covered by idx_7CBdHug)
	var addresses []addressRow
	err := app.DB().NewQuery(`
		SELECT id, code, floor, sequence, status, notes, not_home_tries, dnc_time,
		       COALESCE(coordinates, '') as coordinates, updated, updated_by
		FROM addresses
		WHERE map = {:map}
	`).Bind(params).All(&addresses)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to fetch addresses"})
	}

	// Query 2: address_options (~6ms, index-covered by idx_SDhkFBbBup)
	var options []addressOption
	err = app.DB().NewQuery(`
		SELECT id, address, option
		FROM address_options
		WHERE map = {:map}
	`).Bind(params).All(&options)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to fetch address options"})
	}

	optionMap := make(map[string][]addressOption, len(options))
	for _, opt := range options {
		optionMap[opt.Address] = append(optionMap[opt.Address], opt)
	}

	result := make([]addressResponse, len(addresses))
	for i, addr := range addresses {
		opts := optionMap[addr.Id]
		if opts == nil {
			opts = []addressOption{}
		}

		// Parse coordinates JSON; empty/null sent as null.
		var coords any
		if addr.Coordinates != "" {
			coords = json.RawMessage(addr.Coordinates)
		}

		result[i] = addressResponse{
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

	return c.JSON(http.StatusOK, result)
}
