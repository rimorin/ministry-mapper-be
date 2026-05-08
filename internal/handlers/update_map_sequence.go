package handlers

import (
	"net/http"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

type UpdateTerritoryMapSequenceRequest struct {
	TerritoryId string   `json:"territory_id"`
	MapIds      []string `json:"map_ids"`
}

func HandleUpdateTerritoryMapSequence(e *core.RequestEvent, app core.App) error {
	data := UpdateTerritoryMapSequenceRequest{}
	if err := e.BindBody(&data); err != nil {
		return apis.NewBadRequestError("Invalid request body", nil)
	}
	if data.TerritoryId == "" {
		return apis.NewBadRequestError("territory_id is required", nil)
	}
	if len(data.MapIds) == 0 {
		return apis.NewBadRequestError("map_ids is required", nil)
	}

	congId := getTerritoryCongregation(app, data.TerritoryId)
	if congId == "" {
		return apis.NewNotFoundError("Territory not found", nil)
	}

	if !AuthorizeByRole(app, e.Auth.Id, congId, "administrator") {
		return apis.NewForbiddenError("Administrator access required", nil)
	}

	seen := make(map[string]struct{}, len(data.MapIds))
	for _, id := range data.MapIds {
		if _, dup := seen[id]; dup {
			return apis.NewBadRequestError("map_ids contains duplicate IDs", nil)
		}
		seen[id] = struct{}{}
	}

	records, err := app.FindAllRecords("maps", dbx.HashExp{"territory": data.TerritoryId})
	if err != nil {
		return apis.NewApiError(500, "Error fetching maps", nil)
	}

	if len(data.MapIds) != len(records) {
		return apis.NewBadRequestError("map_ids must include all maps in the territory", nil)
	}

	recordById := make(map[string]*core.Record, len(records))
	for _, r := range records {
		recordById[r.Id] = r
	}
	for _, id := range data.MapIds {
		if _, ok := recordById[id]; !ok {
			return apis.NewBadRequestError("map not found in territory", nil)
		}
	}

	err = app.RunInTransaction(func(txApp core.App) error {
		for i, id := range data.MapIds {
			rec := recordById[id]
			rec.Set("sequence", i+1)
			if err := txApp.SaveNoValidate(rec); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return apis.NewApiError(500, "Error updating map sequences", nil)
	}

	return e.String(http.StatusOK, "Map sequences updated")
}
