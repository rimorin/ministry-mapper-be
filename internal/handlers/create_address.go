package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

type CreateAddressRequest struct {
	AddressId    string          `json:"address_id"`
	MapId        string          `json:"map_id"`
	Code         string          `json:"code"`
	Floor        int             `json:"floor"`
	Notes        string          `json:"notes"`
	Status       string          `json:"status"`
	NotHomeTries int             `json:"not_home_tries"`
	DncTime      string          `json:"dnc_time"`
	Coordinates  json.RawMessage `json:"coordinates"`
	UpdatedBy    string          `json:"updated_by"`
	AddOptionIds []string        `json:"add_option_ids"`
}

func HandleCreateAddress(c *core.RequestEvent, app core.App) error {
	var req CreateAddressRequest
	if err := c.BindBody(&req); err != nil {
		return apis.NewBadRequestError("Invalid request body", nil)
	}

	if req.MapId == "" || req.Code == "" {
		return apis.NewBadRequestError("map_id and code are required", nil)
	}

	if !codeFormatRegex.MatchString(req.Code) {
		return apis.NewBadRequestError("code must contain only alphanumeric characters and hyphens", nil)
	}

	if !AuthorizeMapAccess(c, app, req.MapId) {
		return apis.NewForbiddenError("Unauthorized", nil)
	}

	status := req.Status
	if status == "" {
		status = "not_done"
	}

	floor := req.Floor
	if floor == 0 {
		floor = 1
	}

	var newID string
	err := app.RunInTransaction(func(txApp core.App) error {
		mapRecord, err := txApp.FindRecordById("maps", req.MapId)
		if err != nil {
			return apis.NewNotFoundError("Map not found", nil)
		}

		existing, _ := txApp.FindFirstRecordByFilter(
			"addresses",
			"map = {:map} AND code = {:code} AND floor = {:floor}",
			dbx.Params{"map": req.MapId, "code": req.Code, "floor": floor},
		)
		if existing != nil {
			return apis.NewBadRequestError("Address code already exists on this floor", nil)
		}

		sequence, err := fetchMapMaxSequence(txApp, req.MapId)
		if err != nil {
			return err
		}

		col, err := txApp.FindCachedCollectionByNameOrId("addresses")
		if err != nil {
			return err
		}

		record := core.NewRecord(col)
		if req.AddressId != "" {
			record.Id = req.AddressId
		}
		record.Set("map", req.MapId)
		record.Set("code", req.Code)
		record.Set("floor", floor)
		record.Set("congregation", mapRecord.GetString("congregation"))
		record.Set("territory", mapRecord.GetString("territory"))
		record.Set("status", status)
		record.Set("not_home_tries", req.NotHomeTries)
		record.Set("notes", req.Notes)
		record.Set("dnc_time", req.DncTime)
		if len(req.Coordinates) > 0 && string(req.Coordinates) != "null" {
			record.Set("coordinates", req.Coordinates)
		}
		record.Set("updated_by", req.UpdatedBy)
		record.Set("created_by", req.UpdatedBy)
		record.Set("sequence", sequence+1)
		record.Set("source", "app")

		if err := txApp.SaveNoValidate(record); err != nil {
			return err
		}
		newID = record.Id

		if len(req.AddOptionIds) > 0 {
			aoCol, err := txApp.FindCachedCollectionByNameOrId("address_options")
			if err != nil {
				return err
			}
			for _, optId := range req.AddOptionIds {
				ao := core.NewRecord(aoCol)
				ao.Set("address", record.Id)
				ao.Set("option", optId)
				ao.Set("congregation", mapRecord.GetString("congregation"))
				ao.Set("map", req.MapId)
				if err := txApp.SaveNoValidate(ao); err != nil {
					var ve validation.Errors
					if !errors.As(err, &ve) {
						return err
					}
				}
			}
		}

		return nil
	})

	if err != nil {
		return apis.ToApiError(err)
	}

	ProcessMapAggregates(req.MapId, app)

	return c.JSON(http.StatusCreated, map[string]string{"id": newID})
}
