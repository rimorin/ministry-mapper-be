package handlers

import (
	"log"

	"github.com/getsentry/sentry-go"
	"github.com/pocketbase/pocketbase/core"
)

func LogRoleGranted(e *core.RecordRequestEvent) {
	writeRoleLog(e, "granted", "", e.Record.GetString("role"))
}

// LogRoleChanged logs a role level change. oldRole must be captured before
// calling e.Next() while Original() still holds the pre-update value.
func LogRoleChanged(e *core.RecordRequestEvent, oldRole string) {
	newRole := e.Record.GetString("role")
	if oldRole == newRole {
		return
	}
	writeRoleLog(e, "changed", oldRole, newRole)
}

func LogRoleRevoked(e *core.RecordRequestEvent) {
	writeRoleLog(e, "revoked", e.Record.GetString("role"), "")
}

func writeRoleLog(e *core.RecordRequestEvent, action, oldRole, newRole string) {
	collection, err := e.App.FindCollectionByNameOrId("roles_log")
	if err != nil {
		sentry.CaptureException(err)
		log.Printf("Error finding roles_log collection: %v", err)
		return
	}

	logRecord := core.NewRecord(collection)
	logRecord.Set("congregation", e.Record.Get("congregation"))
	logRecord.Set("user", e.Record.Get("user"))
	logRecord.Set("old_role", oldRole)
	logRecord.Set("new_role", newRole)
	logRecord.Set("action", action)
	logRecord.Set("changed_by", authID(e.Auth))

	if err := e.App.Save(logRecord); err != nil {
		sentry.CaptureException(err)
		log.Printf("Error saving role log: %v", err)
	}
}
