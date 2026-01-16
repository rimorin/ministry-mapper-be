package handlers

import (
	"fmt"
	"net/http"
	"regexp"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

var codeFormatRegex = regexp.MustCompile(`^[a-zA-Z0-9-]+$`)

// HandleMapAdd handles the addition of multiple address codes to a map.
// It performs the following steps:
// 1. Extracts the request information and retrieves the codes array and map ID from the request body.
// 2. Validates the codes array (non-empty, valid strings, no duplicates within request).
// 3. Checks each code against the database and separates valid codes from existing ones.
// 4. Fetches the floors associated with the map ID. If an error occurs, returns a not found error.
// 5. Fetches the maximum sequence number for the map ID. If an error occurs, returns a not found error.
// 6. Fetches the map data for the map ID. If an error occurs, returns a not found error.
// 7. Fetches the default congregation option for the map's congregation. If an error occurs, returns a not found error.
// 8. Runs a transaction to insert address records for each valid code across all floors.
//   - Sets the code, congregation, floor, map ID, type, status, territory, and sequence for each record.
//   - Increments sequence for each code added.
//   - If an error occurs during the transaction, returns the error.
//
// 9. Processes map aggregates for the map ID.
// 10. Returns detailed JSON response with counts of inserted/skipped codes.
//
// Parameters:
// - e: The request event containing the request information.
// - app: The PocketBase application instance.
//
// Returns:
// - An error if validation fails or database operations fail, otherwise a detailed JSON response.
func HandleMapAdd(e *core.RequestEvent, app *pocketbase.PocketBase) error {
	requestInfo, _ := e.RequestInfo()
	data := requestInfo.Body
	mapId := data["map"].(string)

	// Phase 1: STRICT Validation - Extract and validate codes array
	codesRaw, ok := data["codes"].([]interface{})
	if !ok || len(codesRaw) == 0 {
		return apis.NewBadRequestError("codes array is required and cannot be empty", nil)
	}

	// Convert to strings and check for duplicates within request
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

		// Validate format: alphanumeric + hyphen only
		if !codeFormatRegex.MatchString(code) {
			return apis.NewBadRequestError(
				fmt.Sprintf("Invalid code at index %d: '%s' must contain only alphanumeric characters and hyphens", i, code),
				nil,
			)
		}

		// Strict: No duplicates allowed in request
		if seen[code] {
			return apis.NewBadRequestError(
				fmt.Sprintf("Duplicate code in request: '%s'", code),
				nil,
			)
		}

		seen[code] = true
		codes = append(codes, code)
	}

	// Phase 2: LENIENT Processing - Check against database
	var validCodes []string
	var existingCodes []string

	for _, code := range codes {
		_, err := fetchAddressByCode(app, code, mapId)
		if err != nil {
			// Code doesn't exist - OK to add
			validCodes = append(validCodes, code)
		} else {
			// Code exists in DB - skip but don't fail
			existingCodes = append(existingCodes, code)
		}
	}

	// Early return if nothing to process
	if len(validCodes) == 0 {
		return e.JSON(http.StatusOK, map[string]interface{}{
			"success":        true,
			"codes_requested": len(codes),
			"codes_inserted":  0,
			"codes_skipped":   len(existingCodes),
			"addresses_created": 0,
			"existing_codes":  existingCodes,
			"message":        "All codes already exist",
		})
	}

	// Fetch map metadata (once for all codes)
	floors, err := fetchMapFloors(app, mapId)
	if err != nil {
		return apis.NewNotFoundError("Error fetching floors", nil)
	}

	sequence, err := fetchMapMaxSequence(app, mapId)
	if err != nil {
		return apis.NewNotFoundError("Error fetching sequence", nil)
	}

	mapData, err := fetchMapData(app, mapId)
	if err != nil {
		return apis.NewNotFoundError("Error fetching map data", nil)
	}

	defaultCode, err := fetchDefaultCongregationOption(app, mapData.Get("congregation").(string))
	if err != nil {
		return apis.NewNotFoundError("Error fetching default code", nil)
	}

	// Transaction: Insert all valid codes
	currentSequence := sequence
	err = app.RunInTransaction(func(txApp core.App) error {
		collection, err := txApp.FindCollectionByNameOrId("addresses")
		if err != nil {
			return err
		}

		for _, code := range validCodes {
			currentSequence++
			for _, floor := range floors {
				record := core.NewRecord(collection)
				record.Set("code", code)
				record.Set("congregation", mapData.Get("congregation"))
				record.Set("floor", floor)
				record.Set("map", mapId)
				record.Set("type", defaultCode.Id)
				record.Set("status", "not_done")
				record.Set("territory", mapData.Get("territory"))
				record.Set("sequence", currentSequence)

				if err := txApp.Save(record); err != nil {
					return err
				}
			}
		}
		return nil
	})

	if err != nil {
		return apis.NewNotFoundError("Error inserting records", nil)
	}

	// Process aggregates once for all changes
	ProcessMapAggregates(mapId, app)

	// Return detailed response
	totalInserted := len(validCodes) * len(floors)

	return e.JSON(http.StatusOK, map[string]interface{}{
		"success":         true,
		"codes_requested": len(codes),
		"codes_inserted":  len(validCodes),
		"codes_skipped":   len(existingCodes),
		"addresses_created": totalInserted,
		"existing_codes":  existingCodes,
		"message": fmt.Sprintf(
			"%d codes processed (%d addresses created), %d skipped (already exist)",
			len(validCodes),
			totalInserted,
			len(existingCodes),
		),
	})
}
