package handlers

import (
	"fmt"
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

// fetchDistinctMapCodes returns every address code in a map.
func fetchDistinctMapCodes(app core.App, mapId string) ([]string, error) {
	rows := []struct {
		Code string `db:"code"`
	}{}
	err := app.DB().NewQuery("SELECT DISTINCT code FROM addresses WHERE map = {:map}").
		Bind(dbx.Params{"map": mapId}).All(&rows)
	if err != nil {
		return nil, err
	}

	codes := make([]string, len(rows))
	for i, row := range rows {
		codes[i] = row.Code
	}

	return codes, nil
}

// validateSequencePayload checks the request in isolation and returns the set
// of codes it carries. Two codes claiming the same sequence is the collision
// this whole guard exists to stop.
func validateSequencePayload(codes []CodeSequenceUpdate) (map[string]bool, error) {
	seenCode := make(map[string]bool, len(codes))
	seenSequence := make(map[int]string, len(codes))

	for _, entry := range codes {
		if entry.Code == "" {
			return nil, apis.NewBadRequestError("Code cannot be empty", nil)
		}
		if seenCode[entry.Code] {
			return nil, apis.NewBadRequestError(
				fmt.Sprintf("Duplicate code in request: '%s'", entry.Code), nil)
		}
		if other, taken := seenSequence[entry.Sequence]; taken {
			return nil, apis.NewBadRequestError(
				fmt.Sprintf("Codes '%s' and '%s' both claim sequence %d", other, entry.Code, entry.Sequence), nil)
		}
		seenCode[entry.Code] = true
		seenSequence[entry.Sequence] = entry.Code
	}

	return seenCode, nil
}

// validateSequenceUpdate enforces the invariant that a map holds one distinct
// sequence per code. The map table aligns each floor's cells to a single row of
// column headers by sequence, and the Excel report keys its grid on sequence —
// so a repeated value puts cells under the wrong header and drops a column from
// the export entirely.
//
// A partial payload is rejected for the same reason: the codes left out keep
// their old sequences, which are then free to collide with the ones being
// rewritten. Requiring the full set mirrors /maps/sequence, which likewise
// insists on every map id in the territory.
func validateSequenceUpdate(app core.App, data UpdateMapSequenceRequest) error {
	seenCode, err := validateSequencePayload(data.Codes)
	if err != nil {
		return err
	}

	existing, err := fetchDistinctMapCodes(app, data.MapId)
	if err != nil {
		return newServerError(err)
	}

	if len(data.Codes) != len(existing) {
		return apis.NewBadRequestError(
			fmt.Sprintf("Request must cover all %d codes in the map, got %d", len(existing), len(data.Codes)), nil)
	}

	for _, code := range existing {
		if !seenCode[code] {
			return apis.NewBadRequestError(
				fmt.Sprintf("Code '%s' is missing from the request", code), nil)
		}
	}

	return nil
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

	if err := validateSequenceUpdate(app, data); err != nil {
		return err
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
type DeleteAddressCodeRequest struct {
	Code string `json:"code"`
	Map  string `json:"map"`
}

func HandleMapDelete(c *core.RequestEvent, app core.App) error {
	data := DeleteAddressCodeRequest{}
	if err := c.BindBody(&data); err != nil {
		return apis.NewBadRequestError("Invalid request body", nil)
	}
	if data.Map == "" {
		return apis.NewBadRequestError("map is required", nil)
	}
	if data.Code == "" {
		return apis.NewBadRequestError("code is required", nil)
	}

	code := data.Code
	mapId := data.Map

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
