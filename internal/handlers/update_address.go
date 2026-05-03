package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

type UpdateAddressRequest struct {
	AddressId    string          `json:"address_id"`
	MapId        string          `json:"map_id"`
	Notes        string          `json:"notes"`
	Status       string          `json:"status"`
	NotHomeTries int             `json:"not_home_tries"`
	DncTime      string          `json:"dnc_time"`
	Coordinates  json.RawMessage `json:"coordinates"` // null | {"lat": ..., "lng": ...}
	UpdatedBy    string          `json:"updated_by"`
	DeleteAoIds  []string        `json:"delete_ao_ids"`
	AddOptionIds []string        `json:"add_option_ids"`
}

func HandleUpdateAddress(c *core.RequestEvent, app core.App) error {
	var req UpdateAddressRequest
	if err := c.BindBody(&req); err != nil {
		return apis.NewBadRequestError("Invalid request body", nil)
	}

	if req.AddressId == "" || req.MapId == "" {
		return apis.NewBadRequestError("address_id and map_id are required", nil)
	}

	if !AuthorizeMapAccess(c, app, req.MapId) {
		return apis.NewForbiddenError("Unauthorized", nil)
	}

	err := app.RunInTransaction(func(txApp core.App) error {
		address, err := txApp.FindRecordById("addresses", req.AddressId)
		if err != nil {
			return apis.NewNotFoundError("Address not found", nil)
		}
		if address.GetString("map") != req.MapId {
			return apis.NewForbiddenError("Address does not belong to the specified map", nil)
		}

		congregation := address.GetString("congregation")

		for _, aoId := range req.DeleteAoIds {
			ao, err := txApp.FindRecordById("address_options", aoId)
			if err != nil {
				continue // already gone — treat as success
			}
			if ao.GetString("address") != req.AddressId || ao.GetString("map") != req.MapId {
				return apis.NewForbiddenError("The address_option does not belong to this address", nil)
			}
			if err := txApp.Delete(ao); err != nil {
				return err
			}
		}

		if len(req.AddOptionIds) > 0 {
			aoCol, err := txApp.FindCachedCollectionByNameOrId("address_options")
			if err != nil {
				return err
			}
			for _, optId := range req.AddOptionIds {
				ao := core.NewRecord(aoCol)
				ao.Set("address", req.AddressId)
				ao.Set("option", optId)
				ao.Set("congregation", congregation)
				ao.Set("map", req.MapId)
				if err := txApp.SaveNoValidate(ao); err != nil {
					// PocketBase converts UNIQUE violations to validation.Errors — option already exists, skip.
					var ve validation.Errors
					if !errors.As(err, &ve) {
						return err
					}
				}
			}
		}

		address.Set("notes", req.Notes)
		address.Set("status", req.Status)
		address.Set("not_home_tries", req.NotHomeTries)
		address.Set("dnc_time", req.DncTime)
		if len(req.Coordinates) == 0 || string(req.Coordinates) == "null" {
			address.Set("coordinates", nil)
		} else {
			address.Set("coordinates", req.Coordinates)
		}
		address.Set("updated_by", req.UpdatedBy)

		return txApp.SaveNoValidate(address)
	})

	if err != nil {
		return apis.ToApiError(err)
	}

	return c.NoContent(http.StatusNoContent)
}
