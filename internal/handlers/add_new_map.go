package handlers

import (
	"log"
	"regexp"
	"strings"

	"github.com/getsentry/sentry-go"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// HandleNewMap handles the creation of a new map record and associated address records.
// It performs validation on the input data, creates the map record, and then creates
// address records for each sequence element for each floor.
//
// Parameters:
//   - c: A pointer to a core.RequestEvent containing the request information.
//   - app: A pointer to a pocketbase.PocketBase instance.
//
// Returns:
//   - error: An error if any issues occur during the process, otherwise nil.
//
// The function performs the following steps:
//  1. Extracts and validates the input data from the request.
//  2. Validates the sequence format and map type.
//  3. Fetches the default congregation option.
//  4. Creates a new map record and saves it to the database.
//  5. Splits the sequence string into an array.
//  6. Creates address records for each sequence element for each floor.
//  7. Resets the map territory.
//
// Possible errors include invalid sequence format, invalid map type, error fetching
// default congregation option, and error saving map or address records.
func HandleNewMap(c *core.RequestEvent, app *pocketbase.PocketBase) error {
	requestInfo, _ := c.RequestInfo()
	data := requestInfo.Body
	code := data["code"].(string)
	territory := data["territory"].(string)
	mapType := data["type"].(string)
	floors := int(data["floors"].(float64))
	name := data["name"].(string)
	congregation := data["congregation"].(string)
	coordinates := data["coordinates"].(string)
	sequence := data["sequence"].(string)

	// validate sequence format
	if !isValidSequence(sequence) {
		log.Println("Invalid sequence format")
		return apis.NewBadRequestError("Invalid sequence format", nil)
	}

	// check if mapType is either "single" or "multi"
	if mapType != "single" && mapType != "multi" {
		log.Println("Invalid map type")
		return apis.NewBadRequestError("Invalid map type", nil)
	}

	// if mapType is "single", floors must be 1
	if mapType == "single" && floors != 1 {
		log.Println("Invalid number of floors for single map")
		return apis.NewBadRequestError("Invalid floor for single map", nil)
	}

	option, err := fetchDefaultCongregationOption(app, congregation)
	if err != nil {
		sentry.CaptureException(err)
		log.Println("Error fetching default congregation option:", err)
		return apis.NewNotFoundError("Error fetching default congregation option", nil)
	}

	// create a new map record
	collection, _ := app.FindCollectionByNameOrId("maps")
	mapRecord := core.NewRecord(collection)
	mapRecord.Set("code", code)
	mapRecord.Set("territory", territory)
	mapRecord.Set("type", mapType)
	mapRecord.Set("description", name)
	mapRecord.Set("congregation", congregation)
	mapRecord.Set("coordinates", coordinates)

	if err := app.Save(mapRecord); err != nil {
		log.Printf("Error creating map: %v", err)
		return err
	}

	log.Printf("Map created successfully with ID: %s", mapRecord.Id)

	// split the sequence string into an array
	sequenceArray := splitSequence(sequence)

	// for every floor, create an address record for each sequence element
	for i := 1; i <= floors; i++ {

		// create a new record for each sequence element
		for index, seq := range sequenceArray {
			// create a new address record
			address := createNewAddressRecord(app, seq, territory, option.Id, i, index, mapRecord.Id, congregation)
			if err := app.Save(address); err != nil {
				log.Println("Error saving address record:", err)
				return apis.NewBadRequestError("Error saving address record", nil)
			}
		}
	}
	ResetMapTerritory(mapRecord.Id, app)
	return c.JSON(200, mapRecord)
}

func createNewAddressRecord(app *pocketbase.PocketBase, code, territory, option string, floor int, sequence int, mapId string, congId string) *core.Record {
	collection, _ := app.FindCollectionByNameOrId("addresses")
	address := core.NewRecord(collection)
	address.Set("code", code)
	address.Set("territory", territory)
	address.Set("floor", floor)
	address.Set("sequence", sequence)
	address.Set("status", "not_done")
	address.Set("type", option)
	address.Set("map", mapId)
	address.Set("congregation", congId)
	return address
}

// splitSequence splits the sequence string into an array
func splitSequence(sequence string) []string {
	return strings.Split(sequence, ",")
}

// isValidSequence validates the sequence format
func isValidSequence(sequence string) bool {
	// check if sequence is empty
	if sequence == "" {
		return false
	}

	// check if sequence contains only alphanumeric characters and hyphens
	re := regexp.MustCompile(`^([a-zA-Z0-9-]+,?)*[a-zA-Z0-9-]+$`)
	return re.MatchString(sequence)
}
