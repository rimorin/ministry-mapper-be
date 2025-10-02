package handlers

import (
	"net/http"

	"github.com/getsentry/sentry-go"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

type GetMapCodesRequest struct {
	MapId string `json:"map_id"`
}

func HandleGetMapCodes(c *core.RequestEvent, app *pocketbase.PocketBase) error {
	data := GetMapCodesRequest{}
	if err := c.BindBody(&data); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}

	if data.MapId == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "map_id is required"})
	}

	mapRecord, err := fetchMapData(app, data.MapId)
	if err != nil {
		sentry.CaptureException(err)
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Map not found"})
	}

	codeResults := []struct {
		Code string `db:"code"`
	}{}
	query := app.DB().NewQuery("SELECT DISTINCT code FROM addresses WHERE map = {:map_id} ORDER BY sequence, code")
	err = query.Bind(dbx.Params{"map_id": data.MapId}).All(&codeResults)
	if err != nil {
		sentry.CaptureException(err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to fetch map codes"})
	}

	// Extract codes from struct slice to string slice
	codes := make([]string, len(codeResults))
	for i, result := range codeResults {
		codes[i] = result.Code
	}

	response := map[string]interface{}{
		"codes": codes,
		"type":  mapRecord.Get("type"),
	}

	return c.JSON(http.StatusOK, response)
}
