package handlers

import (
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/labstack/echo/v5"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

var codePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func isValidCode(code string) bool {
	return codePattern.MatchString(code)
}

func validateOptionFormat(optionMap map[string]interface{}) error {
	isDeleted, _ := optionMap["is_deleted"].(bool)
	if isDeleted {
		return nil
	}

	code, ok := optionMap["code"].(string)
	if !ok {
		return errors.New("invalid code format")
	}

	code = strings.TrimSpace(code)
	if len(code) == 0 {
		return errors.New("code cannot be empty")
	}

	if len(code) > 50 {
		return errors.New("code cannot exceed 50 characters")
	}

	if !isValidCode(code) {
		return errors.New("code can only contain letters, numbers, underscores, and hyphens")
	}

	description, ok := optionMap["description"].(string)
	if ok && len(description) > 200 {
		return errors.New("description cannot exceed 200 characters")
	}

	sequence, ok := optionMap["sequence"].(float64)
	if !ok {
		return errors.New("invalid sequence format")
	}

	if sequence < 0 {
		return errors.New("sequence cannot be negative")
	}

	_, ok = optionMap["is_default"].(bool)
	if !ok {
		return errors.New("invalid is_default format")
	}

	return nil
}

func validateOptionsPayload(options []interface{}) error {
	defaultCount := 0
	codeMap := make(map[string]bool)
	sequenceMap := make(map[int]bool)

	for i, option := range options {
		optionMap, ok := option.(map[string]interface{})
		if !ok {
			return fmt.Errorf("invalid option format at index %d", i)
		}

		if err := validateOptionFormat(optionMap); err != nil {
			return fmt.Errorf("option at index %d: %w", i, err)
		}

		isDeleted, _ := optionMap["is_deleted"].(bool)
		if isDeleted {
			continue
		}

		isDefault, _ := optionMap["is_default"].(bool)
		code := strings.TrimSpace(optionMap["code"].(string))
		sequence := int(optionMap["sequence"].(float64))

		if isDefault {
			defaultCount++
		}

		if _, exists := codeMap[code]; exists {
			return fmt.Errorf("duplicate code in payload: %s", code)
		}
		codeMap[code] = true

		if _, exists := sequenceMap[sequence]; exists {
			return fmt.Errorf("duplicate sequence in payload: %d", sequence)
		}
		sequenceMap[sequence] = true
	}

	if defaultCount != 1 {
		return errors.New("exactly one option must be marked as default")
	}

	return nil
}

func verifyOptionOwnership(txApp core.App, optionId, congregation string) error {
	option, err := txApp.FindRecordById("options", optionId)
	if err != nil {
		return fmt.Errorf("option not found: %s", optionId)
	}

	optionCongregation := option.GetString("congregation")
	if optionCongregation != congregation {
		return fmt.Errorf("option %s does not belong to congregation %s", optionId, congregation)
	}

	return nil
}

func validateCodeUniqueness(txApp core.App, code, optionId, congregation string) error {
	return validateCodeUniquenessWithBatch(txApp, code, optionId, congregation, nil)
}

// Allows swapping codes between options in the same batch
func validateCodeUniquenessWithBatch(txApp core.App, code, optionId, congregation string, batchOptionIds map[string]bool) error {
	code = strings.TrimSpace(code)

	filter := "congregation = {:congregation} && code = {:code}"
	params := dbx.Params{"congregation": congregation, "code": code}

	if optionId != "" {
		filter += " && id != {:id}"
		params["id"] = optionId
	}

	existing, err := txApp.FindFirstRecordByFilter("options", filter, params)
	if err == nil && existing != nil {
		if batchOptionIds != nil && batchOptionIds[existing.Id] {
			return nil
		}
		return fmt.Errorf("code '%s' already exists for another option (id: %s)", code, existing.Id)
	}

	return nil
}

func validateSequenceUniqueness(txApp core.App, sequence int, optionId, congregation string) error {
	return validateSequenceUniquenessWithBatch(txApp, sequence, optionId, congregation, nil)
}

// Allows swapping sequences between options in the same batch
func validateSequenceUniquenessWithBatch(txApp core.App, sequence int, optionId, congregation string, batchOptionIds map[string]bool) error {
	filter := "congregation = {:congregation} && sequence = {:sequence}"
	params := dbx.Params{"congregation": congregation, "sequence": sequence}

	if optionId != "" {
		filter += " && id != {:id}"
		params["id"] = optionId
	}

	existing, err := txApp.FindFirstRecordByFilter("options", filter, params)
	if err == nil && existing != nil {
		if batchOptionIds != nil && batchOptionIds[existing.Id] {
			return nil
		}
		return fmt.Errorf("sequence %d already exists for another option (id: %s)", sequence, existing.Id)
	}

	return nil
}

func clearOldDefault(txApp core.App, congregation, excludeId string) error {
	filter := "congregation = {:congregation} && is_default = true"
	params := dbx.Params{"congregation": congregation}

	if excludeId != "" {
		filter += " && id != {:id}"
		params["id"] = excludeId
	}

	existingDefaults, err := txApp.FindRecordsByFilter("options", filter, "", 0, 0, params)
	if err != nil {
		return err
	}

	for _, record := range existingDefaults {
		record.Set("is_default", false)
		if err := txApp.Save(record); err != nil {
			return err
		}
	}

	return nil
}

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

// HandleOptionUpdate processes batch updates of congregation options within a transaction
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

	if len(strings.TrimSpace(congregation)) == 0 {
		return apis.NewBadRequestError("congregation cannot be empty", nil)
	}

	log.Printf("Processing options update for congregation: %s", congregation)

	if err := validateOptionsPayload(options); err != nil {
		return apis.NewBadRequestError(err.Error(), nil)
	}

	var defaultOption string

	err := app.RunInTransaction(func(txApp core.App) error {
		batchOptionIds := make(map[string]bool)
		for _, option := range options {
			optionMap := option.(map[string]interface{})
			if id, ok := optionMap["id"].(string); ok && id != "" {
				isDeleted, _ := optionMap["is_deleted"].(bool)
				if !isDeleted {
					batchOptionIds[id] = true
				}
			}
		}

		for _, option := range options {
			optionMap := option.(map[string]interface{})

			id, idExists := optionMap["id"].(string)
			isDeleted, _ := optionMap["is_deleted"].(bool)

			if isDeleted {
				continue
			}

			if idExists && id != "" {
				if err := verifyOptionOwnership(txApp, id, congregation); err != nil {
					return err
				}
			}

			isDefault, _ := optionMap["is_default"].(bool)
			isCountable, _ := optionMap["is_countable"].(bool)
			code := strings.TrimSpace(optionMap["code"].(string))
			description := strings.TrimSpace(optionMap["description"].(string))
			sequence := int(optionMap["sequence"].(float64))

			if err := validateCodeUniquenessWithBatch(txApp, code, id, congregation, batchOptionIds); err != nil {
				return err
			}

			if err := validateSequenceUniquenessWithBatch(txApp, sequence, id, congregation, batchOptionIds); err != nil {
				return err
			}

			if isDefault {
				if err := clearOldDefault(txApp, congregation, id); err != nil {
					return err
				}
			}

			if idExists && id != "" {
				optionRecord, err := txApp.FindRecordById("options", id)
				if err != nil {
					return err
				}

				oldCountable := optionRecord.GetBool("is_countable")
				if oldCountable && !isCountable {
					log.Printf("Warning: Option %s (code: %s) changed from countable to non-countable - this affects aggregates", id, code)
				}

				optionRecord.Set("is_default", isDefault)
				optionRecord.Set("is_countable", isCountable)
				optionRecord.Set("code", code)
				optionRecord.Set("description", description)
				optionRecord.Set("sequence", sequence)

				if err := txApp.Save(optionRecord); err != nil {
					return err
				}

				if isDefault {
					defaultOption = optionRecord.Id
				}
			} else {
				collection, err := txApp.FindCollectionByNameOrId("options")
				if err != nil {
					return err
				}

				newOption := core.NewRecord(collection)
				newOption.Set("congregation", congregation)
				newOption.Set("is_default", isDefault)
				newOption.Set("is_countable", isCountable)
				newOption.Set("code", code)
				newOption.Set("description", description)
				newOption.Set("sequence", sequence)

				if err := txApp.Save(newOption); err != nil {
					return err
				}

				if isDefault {
					defaultOption = newOption.Id
				}
			}
		}

		if defaultOption == "" {
			return errors.New("no default option was set")
		}

		for _, option := range options {
			optionMap := option.(map[string]interface{})

			id, idExists := optionMap["id"].(string)
			isDeleted, _ := optionMap["is_deleted"].(bool)

			if isDeleted && idExists && id != "" {
				if err := verifyOptionOwnership(txApp, id, congregation); err != nil {
					return err
				}

				if err := handleOptionDeletion(txApp, id, defaultOption, congregation); err != nil {
					return err
				}
			}
		}

		return nil
	})

	if err != nil {
		log.Printf("Error processing options for congregation %s: %v", congregation, err)
		return apis.NewApiError(500, "Error processing options: "+err.Error(), nil)
	}

	log.Printf("Options update completed for congregation: %s", congregation)
	return c.JSON(200, echo.Map{"message": "Options processed successfully"})
}
