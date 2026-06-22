package handlers

import (
	"fmt"
	"net/http"
	"regexp"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

var codeFormatRegex = regexp.MustCompile(`^[a-zA-Z0-9-]+$`)

// HandleMapAdd adds multiple address codes to a map, creating an address record
// per code on every existing floor. Codes already present are skipped, not rejected.
func HandleMapAdd(e *core.RequestEvent, app core.App) error {
	requestInfo, _ := e.RequestInfo()
	data := requestInfo.Body
	mapId := data["map"].(string)

	mapData, err := fetchMapData(app, mapId)
	if err != nil {
		return apis.NewNotFoundError("Error fetching map data", nil)
	}

	if !AuthorizeByRole(app, e.Auth.Id, mapData.GetString("congregation"), "administrator") {
		return apis.NewForbiddenError("Administrator access required", nil)
	}

	codesRaw, ok := data["codes"].([]interface{})
	if !ok || len(codesRaw) == 0 {
		return apis.NewBadRequestError("codes array is required and cannot be empty", nil)
	}

	var codes []string
	seen := make(map[string]bool)

	for i, codeRaw := range codesRaw {
		code, ok := codeRaw.(string)
		if !ok || code == "" {
			return apis.NewBadRequestError(
				fmt.Sprintf("Invalid code at index %d: must be non-empty string", i),
				nil,
			)
		}

		if !codeFormatRegex.MatchString(code) {
			return apis.NewBadRequestError(
				fmt.Sprintf("Invalid code at index %d: '%s' must contain only alphanumeric characters and hyphens", i, code),
				nil,
			)
		}

		if seen[code] {
			return apis.NewBadRequestError(
				fmt.Sprintf("Duplicate code in request: '%s'", code),
				nil,
			)
		}

		seen[code] = true
		codes = append(codes, code)
	}

	// Codes already in the DB are skipped, not rejected.
	var validCodes []string
	var existingCodes []string

	for _, code := range codes {
		if _, err := fetchAddressByCode(app, code, mapId); err != nil {
			validCodes = append(validCodes, code)
		} else {
			existingCodes = append(existingCodes, code)
		}
	}

	if len(validCodes) == 0 {
		return e.JSON(http.StatusOK, map[string]interface{}{
			"success":           true,
			"codes_requested":   len(codes),
			"codes_inserted":    0,
			"codes_skipped":     len(existingCodes),
			"addresses_created": 0,
			"existing_codes":    existingCodes,
			"message":           "All codes already exist",
		})
	}

	floors, err := fetchMapFloors(app, mapId)
	if err != nil {
		return apis.NewNotFoundError("Error fetching floors", nil)
	}

	sequence, err := fetchMapMaxSequence(app, mapId)
	if err != nil {
		return apis.NewNotFoundError("Error fetching sequence", nil)
	}

	defaultCode, err := fetchDefaultCongregationOption(app, mapData.GetString("congregation"))
	if err != nil {
		return apis.NewNotFoundError("Error fetching default code", nil)
	}

	currentSequence := sequence
	err = app.RunInTransaction(func(txApp core.App) error {
		collection, err := txApp.FindCollectionByNameOrId("addresses")
		if err != nil {
			return err
		}

		aoCollection, err := txApp.FindCachedCollectionByNameOrId("address_options")
		if err != nil {
			return err
		}

		for _, code := range validCodes {
			currentSequence++
			for _, floor := range floors {
				record := core.NewRecord(collection)
				record.Set("code", code)
				record.Set("congregation", mapData.GetString("congregation"))
				record.Set("floor", floor)
				record.Set("map", mapId)
				record.Set("status", "not_done")
				record.Set("territory", mapData.GetString("territory"))
				record.Set("sequence", currentSequence)
				record.Set("source", "admin")
				record.Set("created_by", e.Auth.Get("name").(string))

				if err := txApp.SaveNoValidate(record); err != nil {
					return err
				}
				aoRec := core.NewRecord(aoCollection)
				aoRec.Set("address", record.Id)
				aoRec.Set("option", defaultCode.Id)
				aoRec.Set("congregation", mapData.GetString("congregation"))
				aoRec.Set("map", mapId)
				if err := txApp.SaveNoValidate(aoRec); err != nil {
					return err
				}
			}
		}
		return nil
	})

	if err != nil {
		return newServerError(err)
	}

	ProcessMapAggregates(mapId, app)

	totalInserted := len(validCodes) * len(floors)

	return e.JSON(http.StatusOK, map[string]interface{}{
		"success":           true,
		"codes_requested":   len(codes),
		"codes_inserted":    len(validCodes),
		"codes_skipped":     len(existingCodes),
		"addresses_created": totalInserted,
		"existing_codes":    existingCodes,
		"message": fmt.Sprintf(
			"%d codes processed (%d addresses created), %d skipped (already exist)",
			len(validCodes),
			totalInserted,
			len(existingCodes),
		),
	})
}
