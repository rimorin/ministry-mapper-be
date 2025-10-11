package handlers

import (
	"github.com/getsentry/sentry-go"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

// CreateAddress handles the creation of a new address record in the "addresses" collection.
// It retrieves the collection by name or ID, extracts the request body data, and sets the
// corresponding fields in the new record. The record is then saved with validation.
//
// Parameters:
// - app: A pointer to the PocketBase application instance.
// - event: A pointer to the RequestEvent containing the request information.
//
// Returns:
// - error: An error if the collection is not found or if there is an issue saving the record.
func CreateAddress(app *pocketbase.PocketBase, event *core.RequestEvent) error {
	collection, err := app.FindCollectionByNameOrId("addresses")
	if err != nil {
		sentry.CaptureException(err)
		return err
	}

	requestInfo, _ := event.RequestInfo()
	data := requestInfo.Body

	record := core.NewRecord(collection)

	fields := []string{
		"congregation",
		"territory",
		"map",
		"floor",
		"code",
		"type",
		"status",
		"sequence",
		"not_home_tries",
		"notes",
		"dnc_time",
		"coordinates",
	}

	for _, field := range fields {
		if val, ok := data[field]; ok {
			record.Set(field, val)
		}
	}

	return app.Save(record)
}
