package handlers

import (
	"log"
	"net/http"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// HandleMapUpdateSequence handles the update of the sequence number for a specific address code within a map.
// It retrieves the request information, extracts the code, map ID, and sequence number from the request body,
// and updates the sequence number for all address records with the specified code and map ID.
//
// Parameters:
//   - e: A pointer to the core.RequestEvent containing the request information.
//   - app: A pointer to the pocketbase.PocketBase application instance.
//
// Returns:
//   - error: An error if the update operation fails, otherwise nil.
//
// The function performs the following steps:
//  1. Retrieves the request information and extracts the code, map ID, and sequence number from the request body.
//  2. Logs the update operation details.
//  3. Fetches the existing address records by code and map ID.
//  4. Updates the sequence number for all fetched address records within a transaction.
//  5. Returns an appropriate response based on the success or failure of the update operation.
func HandleMapUpdateSequence(e *core.RequestEvent, app *pocketbase.PocketBase) error {
	requestInfo, _ := e.RequestInfo()
	data := requestInfo.Body
	code := data["code"].(string)
	mapId := data["map"].(string)
	sequence := int(data["sequence"].(float64))

	log.Println("Updating sequence for code", code, "in map", mapId, "to", sequence)

	// Fetch the existing address record by code and map ID
	addressRecords, err := fetchAddressesByCode(app, code, mapId)
	if err != nil {
		return apis.NewNotFoundError("Error fetching address", nil)
	}

	// update all addresses with the same code and map ID as transaction
	err = app.RunInTransaction(func(txApp core.App) error {
		for _, addressRecord := range addressRecords {
			addressRecord.Set("sequence", sequence)
			if err := txApp.Save(addressRecord); err != nil {
				return err
			}
		}
		return e.Next()
	})

	if err != nil {
		return apis.NewApiError(500, "Error updating address sequence", nil)
	}

	return e.String(http.StatusOK, "Address sequence updated successfully")
}

// HandleMapDelete handles the deletion of addresses associated with a specific code and map ID.
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
