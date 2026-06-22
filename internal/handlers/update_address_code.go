package handlers

import (
	"log"
	"net/http"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

type CodeSequenceUpdate struct {
	Code     string `json:"code"`
	Sequence int    `json:"sequence"`
}

type UpdateMapSequenceRequest struct {
	MapId string               `json:"map"`
	Codes []CodeSequenceUpdate `json:"codes"`
}

// countUniqueAddressCodes counts the number of distinct address codes in a map
func countUniqueAddressCodes(app core.App, mapId string) (int, error) {
	result := struct {
		Count int `db:"count"`
	}{}
	query := app.DB().NewQuery("SELECT COUNT(DISTINCT code) as count FROM addresses WHERE map = {:map}")
	err := query.Bind(dbx.Params{"map": mapId}).One(&result)
	return result.Count, err
}

// HandleMapUpdateSequence updates sequence numbers for multiple address codes within a map.
func HandleMapUpdateSequence(e *core.RequestEvent, app core.App) error {
	data := UpdateMapSequenceRequest{}
	if err := e.BindBody(&data); err != nil {
		return apis.NewBadRequestError("Invalid request body", nil)
	}

	if data.MapId == "" {
		return apis.NewBadRequestError("map is required", nil)
	}

	if len(data.Codes) == 0 {
		return apis.NewBadRequestError("codes array is required", nil)
	}

	mapData, err := fetchMapData(app, data.MapId)
	if err != nil {
		return apis.NewNotFoundError("Map not found", nil)
	}

	if !AuthorizeByRole(app, e.Auth.Id, mapData.GetString("congregation"), "administrator") {
		return apis.NewForbiddenError("Administrator access required", nil)
	}

	log.Println("Updating sequences for", len(data.Codes), "codes in map", data.MapId)

	err = app.RunInTransaction(func(txApp core.App) error {
		for _, codeSeq := range data.Codes {
			records, err := txApp.FindRecordsByFilter(
				"addresses",
				"code = {:code} && map = {:map}",
				"",
				0,
				0,
				map[string]any{
					"code": codeSeq.Code,
					"map":  data.MapId,
				},
			)
			if err != nil {
				return err
			}

			for _, record := range records {
				record.Set("sequence", codeSeq.Sequence)
				if err := txApp.Save(record); err != nil {
					return err
				}
			}
		}
		return nil
	})

	if err != nil {
		return newServerError(err)
	}

	return e.String(http.StatusOK, "Address sequences updated successfully")
}

// HandleMapDelete deletes all addresses for a given code and map, refusing to
// remove the last remaining code. Map aggregates are recalculated afterwards.
func HandleMapDelete(c *core.RequestEvent, app core.App) error {
	requestInfo, _ := c.RequestInfo()
	data := requestInfo.Body
	code := data["code"].(string)
	mapId := data["map"].(string)

	mapData, err := fetchMapData(app, mapId)
	if err != nil {
		return apis.NewNotFoundError("Map not found", nil)
	}

	if !AuthorizeByRole(app, c.Auth.Id, mapData.GetString("congregation"), "administrator") {
		return apis.NewForbiddenError("Administrator access required", nil)
	}

	log.Println("Deleting addresses for code", code, "in map", mapId)

	codeCount, err := countUniqueAddressCodes(app, mapId)
	if err != nil {
		return apis.NewNotFoundError("Error counting address codes", nil)
	}

	if codeCount <= 1 {
		return apis.NewBadRequestError("Cannot delete the last address code", nil)
	}

	addressRecords, err := fetchAddressesByCode(app, code, mapId)
	if err != nil {
		return apis.NewNotFoundError("Error fetching address", nil)
	}

	err = app.RunInTransaction(func(txApp core.App) error {
		for _, addressRecord := range addressRecords {
			if err := txApp.Delete(addressRecord); err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		return newServerError(err)
	}
	ProcessMapAggregates(mapId, app)

	return c.String(http.StatusOK, "Addresses code deleted successfully")
}
