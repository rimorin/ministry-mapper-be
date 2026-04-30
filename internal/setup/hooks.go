package setup

import (
	"log"
	"strings"
	"time"

	"ministry-mapper/internal/handlers"

	"github.com/pocketbase/pocketbase/core"
)

func RegisterDomainHooks(app core.App) {
	// Track notes changes on address updates
	app.OnRecordUpdate("addresses").BindFunc(func(e *core.RecordEvent) error {
		originalNotes := e.Record.Original().Get("notes").(string)
		newNotes := e.Record.Get("notes").(string)
		updatedBy := e.Record.Get("updated_by").(string)

		if originalNotes != newNotes {
			e.Record.Set("last_notes_updated", time.Now())
			e.Record.Set("last_notes_updated_by", updatedBy)
		}

		return e.Next()
	})

	// Log address status changes for audit trail
	app.OnRecordAfterUpdateSuccess("addresses").BindFunc(func(e *core.RecordEvent) error {
		handlers.LogAddressStatusChange(e)
		return e.Next()
	})

	// Track last login and reset inactive warnings
	app.OnRecordAuthRequest("users").BindFunc(func(e *core.RecordAuthRequestEvent) error {
		e.Record.Set("last_login", time.Now())
		e.Record.Set("inactive_warning_sent_at", nil)
		e.Record.Set("inactive_final_warning_sent_at", nil)
		if err := e.App.SaveNoValidate(e.Record); err != nil {
			log.Printf("warning: error saving last_login for user %s: %v", e.Record.Id, err)
		}
		return e.Next()
	})

	// Normalize email on user creation
	app.OnRecordCreate("users").BindFunc(func(e *core.RecordEvent) error {
		email := e.Record.Get("email").(string)
		e.Record.Set("email", strings.ToLower(strings.TrimSpace(email)))
		e.Record.SetEmailVisibility(true)
		return e.Next()
	})

	// Stamp unprovisioned_since when a user's last role is deleted
	app.OnRecordAfterDeleteSuccess("roles").BindFunc(func(e *core.RecordEvent) error {
		handlers.HandleRoleDelete(e)
		return e.Next()
	})
}
