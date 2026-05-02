package handlers

import (
	"log"

	"github.com/getsentry/sentry-go"
	"github.com/pocketbase/pocketbase/core"
)

// LogAddressStatusChange records a status change event in the addresses_log collection.
// It should be called from OnRecordAfterUpdateSuccess when the address status has changed,
// or when not_home_tries is incremented (status stays not_home but the aggregate bucket shifts).
func LogAddressStatusChange(e *core.RecordEvent) {
	oldStatus, _ := e.Record.Original().Get("status").(string)
	newStatus, _ := e.Record.Get("status").(string)

	if oldStatus == "" || newStatus == "" {
		return
	}

	oldTries := e.Record.Original().GetInt("not_home_tries")
	newTries := e.Record.GetInt("not_home_tries")

	statusUnchanged := oldStatus == newStatus
	triesUnchanged := oldTries == newTries
	if statusUnchanged && (triesUnchanged || newStatus != "not_home") {
		return
	}

	collection, err := e.App.FindCollectionByNameOrId("addresses_log")
	if err != nil {
		sentry.CaptureException(err)
		log.Printf("Error finding addresses_log collection: %v", err)
		return
	}

	logRecord := core.NewRecord(collection)
	logRecord.Set("address", e.Record.Id)
	logRecord.Set("congregation", e.Record.Get("congregation"))
	logRecord.Set("territory", e.Record.Get("territory"))
	logRecord.Set("map", e.Record.Get("map"))
	logRecord.Set("old_status", oldStatus)
	logRecord.Set("new_status", newStatus)
	logRecord.Set("changed_by", e.Record.Get("updated_by"))

	if err := e.App.Save(logRecord); err != nil {
		sentry.CaptureException(err)
		log.Printf("Error saving address log: %v", err)
	}
}
