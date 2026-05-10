package handlers

import (
	"log"

	"github.com/getsentry/sentry-go"
	"github.com/pocketbase/pocketbase/core"
)

func LogAssignmentCreated(e *core.RecordRequestEvent) {
	writeAssignmentLog(e.App, e.Record, authID(e.Auth), "assigned")
}

func LogAssignmentDeleted(e *core.RecordRequestEvent) {
	writeAssignmentLog(e.App, e.Record, authID(e.Auth), "unassigned")
}

// LogAssignmentExpired logs an expiry-driven deletion triggered by the cleanup cron job.
// changed_by is empty since there is no authenticated user in this context.
func LogAssignmentExpired(app core.App, record *core.Record) {
	writeAssignmentLog(app, record, "", "expired")
}

func writeAssignmentLog(app core.App, record *core.Record, changedBy, action string) {
	collection, err := app.FindCollectionByNameOrId("assignments_log")
	if err != nil {
		sentry.CaptureException(err)
		log.Printf("Error finding assignments_log collection: %v", err)
		return
	}

	logRecord := core.NewRecord(collection)
	logRecord.Set("assignment", record.Id)
	logRecord.Set("congregation", record.Get("congregation"))
	logRecord.Set("map", record.Get("map"))
	logRecord.Set("user", record.Get("user"))
	logRecord.Set("publisher", record.Get("publisher"))
	logRecord.Set("type", record.Get("type"))
	logRecord.Set("action", action)
	logRecord.Set("expiry_date", record.Get("expiry_date"))
	logRecord.Set("changed_by", changedBy)

	if err := app.Save(logRecord); err != nil {
		sentry.CaptureException(err)
		log.Printf("Error saving assignment log: %v", err)
	}
}
