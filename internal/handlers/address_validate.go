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

// The address routes are open to anyone holding a share link and save without
// PocketBase's own field validation, so what a home may hold is checked here.
// The limits are well past anything the app sends.
const (
	// maxAddressNotesRunes bounds a home's note: it travels in every map
	// load, the notes digest and the report.
	maxAddressNotesRunes = 5000
	// maxNotHomeTries is far above any congregation's try limit.
	maxNotHomeTries = 99
	// maxAddressOptions bounds the household types changed in one request.
	maxAddressOptions = 30
	// A home's floor, within what a map can be built with.
	minAddressFloor = minStartFloor
	maxAddressFloor = maxStartFloor + maxMapFloors
)

// addressStatuses are the values of the addresses.status field.
var addressStatuses = []string{"not_done", "done", "not_home", "do_not_call", "invalid"}

// A record id as PocketBase and the app generate them.
var recordIDPattern = regexp.MustCompile(`^[a-z0-9]{15}$`)

// invalidValue refuses a value with 422 rather than 400: the app's offline
// queue retries a 400 and drops a 422, and a value that is wrong now will be
// wrong on every retry.
func invalidValue(message string) error {
	return apis.NewApiError(http.StatusUnprocessableEntity, message, nil)
}

// validateAddressFields checks what a request would store on a home.
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

// validateAddressOptions checks that the household types named are the
// congregation's own: a foreign or made-up id would otherwise be stored on
// the home.
func validateAddressOptions(app core.App, congregation string, optionIds []string) error {
	if len(optionIds) == 0 {
		return nil
	}
	if len(optionIds) > maxAddressOptions {
		return invalidValue("Too many options")
	}
	params := dbx.Params{"congregation": congregation}
	unique := uniqueStrings(optionIds)
	placeholders := make([]any, len(unique))
	for i, id := range unique {
		placeholders[i] = id
	}
	var found int
	err := app.DB().Select("COUNT(*)").From("options").
		Where(dbx.NewExp("congregation = {:congregation}", params)).
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
