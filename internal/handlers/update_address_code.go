package handlers

import (
	"log"
	"net/http"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
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
func countUniqueAddressCodes(app *pocketbase.PocketBase, mapId string) (int, error) {
	result := struct {
		Count int `db:"count"`
	}{}
	query := app.DB().NewQuery("SELECT COUNT(DISTINCT code) as count FROM addresses WHERE map = {:map}")
	err := query.Bind(dbx.Params{"map": mapId}).One(&result)
	return result.Count, err
}

// HandleMapUpdateSequence handles the update of sequence numbers for multiple address codes within a map.
// It retrieves the request information, extracts the list of codes with their new sequences and map ID from the request body,
// and updates the sequence numbers using raw SQL for all address records with the specified codes and map ID.
//
// Parameters:
//   - e: A pointer to the core.RequestEvent containing the request information.
//   - app: A pointer to the pocketbase.PocketBase application instance.
//
// Returns:
//   - error: An error if the update operation fails, otherwise nil.
//
// The function performs the following steps:
//  1. Retrieves and validates the request body containing map ID and list of code/sequence pairs.
//  2. Updates the sequence numbers for each code within a transaction.
//  3. Returns an appropriate response based on the success or failure of the update operation.
func HandleMapUpdateSequence(e *core.RequestEvent, app *pocketbase.PocketBase) error {
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

	log.Println("Updating sequences for", len(data.Codes), "codes in map", data.MapId)

	// Update all addresses with the code/sequence pairs within a transaction
	err := app.RunInTransaction(func(txApp core.App) error {
		for _, codeSeq := range data.Codes {
			// Find all address records with matching code and map
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

			// Update sequence for each matching record
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
		return apis.NewApiError(500, "Error updating address sequences", nil)
	}

	return e.String(http.StatusOK, "Address sequences updated successfully")
}

// HandleMapDelete handles the deletion of addresses associated with a specific code and map ID.
// It ensures that there is more than one address code before allowing the deletion.
// It fetches the existing address records by code and map ID, and deletes them within a transaction.
// After successful deletion, it processes map aggregates.
//
// Parameters:
//   - c: The request event containing the request information.
//   - app: The PocketBase application instance.
//
// Returns:
//   - error: An error if the deletion process fails, otherwise nil.
func HandleMapDelete(c *core.RequestEvent, app *pocketbase.PocketBase) error {
	requestInfo, _ := c.RequestInfo()
	data := requestInfo.Body
	code := data["code"].(string)
	mapId := data["map"].(string)

	log.Println("Deleting addresses for code", code, "in map", mapId)

	// count address codes and ensure that there is more than one code
	codeCount, err := countUniqueAddressCodes(app, mapId)
	if err != nil {
		return apis.NewNotFoundError("Error counting address codes", nil)
	}

	if codeCount <= 1 {
		return apis.NewBadRequestError("Cannot delete the last address code", nil)
	}

	// Fetch the existing address record by code and map ID
	addressRecords, err := fetchAddressesByCode(app, code, mapId)
	if err != nil {
		return apis.NewNotFoundError("Error fetching address", nil)
	}

	// delete all addresses with the same code and map ID as transaction
	err = app.RunInTransaction(func(txApp core.App) error {
		for _, addressRecord := range addressRecords {
			if err := txApp.Delete(addressRecord); err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		return apis.NewApiError(500, "Error deleting address", nil)
	}
	ProcessMapAggregates(mapId, app)

	return c.String(http.StatusOK, "Addresses code deleted successfully")
}
