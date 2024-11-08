package handlers

import (
	"errors"
	"log"
	"strconv"

	"github.com/labstack/echo/v5"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// validateOptions validates a slice of options represented as maps.
// Each option must have the following keys with the specified types:
// - "is_deleted": bool (optional, defaults to false)
// - "is_default": bool (required)
// - "code": string (required)
// - "sequence": float64 (required)
//
// The function checks the following conditions:
// - Options with "is_deleted" set to true are ignored.
// - Each option must have a valid "is_default", "code", and "sequence".
// - There must be exactly one option with "is_default" set to true.
// - All "code" values must be unique among non-deleted options.
// - All "sequence" values must be unique among non-deleted options.
//
// Returns an error if any of the conditions are not met.
func validateOptions(options []interface{}) error {
	defaultCount := 0
	codeMap := make(map[string]bool)
	sequenceMap := make(map[int]bool)

	for _, option := range options {
		optionMap, ok := option.(map[string]interface{})
		if !ok {
			return errors.New("invalid option format")
		}

		isDeleted, _ := optionMap["is_deleted"].(bool)
		if isDeleted {
			continue
		}

		isDefault, ok := optionMap["is_default"].(bool)
		if !ok {
			return errors.New("invalid is_default format")
		}

		code, ok := optionMap["code"].(string)
		if !ok {
			return errors.New("invalid code format")
		}

		sequence, ok := optionMap["sequence"].(float64)
		if !ok {
			return errors.New("invalid sequence format")
		}

		if isDefault {
			defaultCount++
		}

		if _, exists := codeMap[code]; exists {
			return errors.New("duplicate code found: " + code)
		}
		codeMap[code] = true

		if _, exists := sequenceMap[int(sequence)]; exists {
			return errors.New("duplicate sequence found: " + strconv.Itoa(int(sequence)))
		}
		sequenceMap[int(sequence)] = true
	}

	if defaultCount != 1 {
		return errors.New("there must be exactly one default option")
	}

	return nil
}

// replaceAddressOptionsWithDefault replaces address options in the database with a default option for a given congregation.
// It finds all records with the old option ID and updates them to include the default option ID if it is not already present.
//
// Parameters:
//   - txDao: The database access object for performing transactions.
//   - oldOptionId: The ID of the old address option to be replaced.
//   - defaultOptionId: The ID of the default address option to be added.
//   - congregation: The congregation to which the address options belong.
//
// Returns:
//   - error: An error object if an error occurs during the operation, otherwise nil.
func replaceAddressOptionsWithDefault(txDao core.App, oldOptionId, defaultOptionId, congregation string) error {
	log.Printf("Replacing address options: oldOptionId=%s, defaultOptionId=%s, congregation=%s", oldOptionId, defaultOptionId, congregation)
	records, err := txDao.FindRecordsByFilter("addresses", "type ~ {:oldOptionId} && congregation = {:congregation}", "", 0, 0, dbx.Params{"oldOptionId": oldOptionId, "congregation": congregation})
	if err != nil {
		log.Printf("Error finding records: %v", err)
		return err
	}

	for _, record := range records {
		existingTypes, ok := record.Get("type").([]string)
		if !ok {
			continue
		}

		if !contains(existingTypes, defaultOptionId) {
			existingTypes = append(existingTypes, defaultOptionId)
			record.Set("type", existingTypes)
			if err := txDao.Save(record); err != nil {
				log.Printf("Error saving record: %v", err)
				return err
			}
		}
	}

	return nil
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// handleOptionDeletion handles the deletion of an option record.
// It first replaces address options with a default option, then deletes the specified option record.
//
// Parameters:
//   - txDao: The core.App instance used for database operations.
//   - id: The ID of the option record to be deleted.
//   - defaultId: The ID of the default option to replace the deleted option.
//   - congregation: The congregation associated with the option.
//
// Returns:
//   - error: An error if any operation fails, otherwise nil.
func handleOptionDeletion(txDao core.App, id, defaultId, congregation string) error {
	log.Printf("Handling option deletion: id=%s, defaultId=%s, congregation=%s", id, defaultId, congregation)
	if err := replaceAddressOptionsWithDefault(txDao, id, defaultId, congregation); err != nil {
		log.Printf("Error replacing address options: %v", err)
		return err
	}

	optionRecordToBeDeleted, err := txDao.FindRecordById("options", id)
	if err != nil {
		log.Printf("Error finding option record: %v", err)
		return err
	}

	if err := txDao.Delete(optionRecordToBeDeleted); err != nil {
		log.Printf("Error deleting option record: %v", err)
		return err
	}

	return nil
}

// HandleOptionUpdate processes the update of options for a given congregation.
// It validates the input data, updates existing options, creates new options,
// and handles the deletion of options within a database transaction.
//
// Parameters:
//   - c: A pointer to core.RequestEvent containing the request context.
//   - app: A pointer to pocketbase.PocketBase instance for database operations.
//
// Returns:
//   - error: An error if the operation fails, otherwise nil.
//
// The function performs the following steps:
//  1. Extracts and validates the request data.
//  2. Logs the start of the options update process.
//  3. Validates the options data.
//  4. Runs a database transaction to:
//     a. Update existing options or create new options if they are not marked as deleted.
//     b. Handle the deletion of options marked as deleted.
//  5. Logs the completion of the options update process.
//  6. Returns a JSON response indicating the success or failure of the operation.
func HandleOptionUpdate(c *core.RequestEvent, app *pocketbase.PocketBase) error {
	requestInfo, _ := c.RequestInfo()
	data := requestInfo.Body

	options, ok := data["options"].([]interface{})
	if !ok {
		return apis.NewBadRequestError("invalid options format", nil)
	}

	congregation, ok := data["congregation"].(string)
	if !ok {
		return apis.NewBadRequestError("invalid congregation format", nil)
	}

	log.Printf("Processing options update for congregation: %s", congregation)

	if err := validateOptions(options); err != nil {
		return apis.NewBadRequestError(err.Error(), nil)
	}

	var defaultOption string

	err := app.RunInTransaction(func(txApp core.App) error {
		for _, option := range options {
			optionMap, ok := option.(map[string]interface{})
			if !ok {
				return errors.New("invalid option format")
			}

			id, idExists := optionMap["id"].(string)
			isDeleted, _ := optionMap["is_deleted"].(bool)
			isDefault, _ := optionMap["is_default"].(bool)
			isCountable, _ := optionMap["is_countable"].(bool)
			code, _ := optionMap["code"].(string)
			description, _ := optionMap["description"].(string)
			sequence, _ := optionMap["sequence"].(float64)

			if !isDeleted {
				if idExists && id != "" {
					if isDefault {
						defaultOption = id
					}
					optionRecord, err := txApp.FindRecordById("options", id)
					if err != nil {
						return err
					}
					optionRecord.Set("is_default", isDefault)
					optionRecord.Set("is_countable", isCountable)
					optionRecord.Set("code", code)
					optionRecord.Set("description", description)
					optionRecord.Set("sequence", int(sequence))
					if err := txApp.Save(optionRecord); err != nil {
						return err
					}
				} else {
					collection, _ := txApp.FindCollectionByNameOrId("options")
					newOption := core.NewRecord(collection)
					newOption.Set("congregation", congregation)
					newOption.Set("is_default", isDefault)
					newOption.Set("is_countable", isCountable)
					newOption.Set("code", code)
					newOption.Set("description", description)
					newOption.Set("sequence", int(sequence))
					if err := txApp.Save(newOption); err != nil {
						return err
					}
				}
			}
		}

		for _, option := range options {
			optionMap, ok := option.(map[string]interface{})
			if !ok {
				return errors.New("invalid option format")
			}

			id, idExists := optionMap["id"].(string)
			isDeleted, _ := optionMap["is_deleted"].(bool)

			if isDeleted && idExists && id != "" {
				if err := handleOptionDeletion(txApp, id, defaultOption, congregation); err != nil {
					return err
				}
			}
		}

		return nil
	})

	if err != nil {
		return apis.NewApiError(500, "Error processing options", nil)
	}

	log.Printf("Options update completed for congregation: %s", congregation)
	return c.JSON(200, echo.Map{"message": "Options processed successfully"})
}
