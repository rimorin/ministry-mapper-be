package handlers

import (
	"encoding/json"
	"net/http"
	"regexp"
	"slices"
	"unicode/utf8"

	"ganigeorgiev.com/pocketbase/dbx"
	"ganigeorgiev.com/pocketbase/pocketbase/apis"
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"ganigeorgiev.com/pocketbase/pocketbase/tools/types"
)

// The address routes save with SaveNoValidate, so values are checked here.
const (
	maxAddressNotesRunes = 5000
	maxNotHomeTries      = 99
	maxAddressOptions    = 30
	minAddressFloor      = minStartFloor
	maxAddressFloor      = maxStartFloor + maxMapFloors
)

var addressStatuses = []string{"not_done", "done", "not_home", "do_not_call", "invalid"}

// recordIDPattern is PocketBase's own id form, which the panel also gives new
// assistant conversations.
var recordIDPattern = regexp.MustCompile(`^[a-z0-9]{15}$`)

// 422, not 400: the app's offline queue retries a 400 and drops a 422.
func invalidValue(message string) error {
	return apis.NewApiError(http.StatusUnprocessableEntity, message, nil)
}

func validateAddressFields(status, notes string, tries int, dncTime string, coordinates json.RawMessage) error {
	if !slices.Contains(addressStatuses, status) {
		return invalidValue("Invalid status")
	}
	if utf8.RuneCountInString(notes) > maxAddressNotesRunes {
		return invalidValue("The note is too long")
	}
	if tries < 0 || tries > maxNotHomeTries {
		return invalidValue("Invalid not_home_tries")
	}
	if dncTime != "" {
		if parsed, err := types.ParseDateTime(dncTime); err != nil || parsed.IsZero() {
			return invalidValue("Invalid dnc_time")
		}
	}
	if len(coordinates) > 0 && string(coordinates) != "null" {
		var point struct {
			Lat *float64 `json:"lat"`
			Lng *float64 `json:"lng"`
		}
		if err := json.Unmarshal(coordinates, &point); err != nil || point.Lat == nil || point.Lng == nil ||
			*point.Lat < -90 || *point.Lat > 90 || *point.Lng < -180 || *point.Lng > 180 {
			return invalidValue("Invalid coordinates")
		}
	}
	return nil
}

// validateAddressOptions refuses options outside the congregation.
func validateAddressOptions(app core.App, congregation string, optionIds []string) error {
	if len(optionIds) == 0 {
		return nil
	}
	if len(optionIds) > maxAddressOptions {
		return invalidValue("Too many options")
	}
	unique := uniqueStrings(optionIds)
	placeholders := make([]any, len(unique))
	for i, id := range unique {
		placeholders[i] = id
	}
	var found int
	err := app.DB().Select("COUNT(*)").From("options").
		Where(dbx.NewExp("congregation = {:congregation}", dbx.Params{"congregation": congregation})).
		AndWhere(dbx.In("id", placeholders...)).
		Row(&found)
	if err != nil {
		return err
	}
	if found != len(unique) {
		return invalidValue("Invalid option")
	}
	return nil
}
