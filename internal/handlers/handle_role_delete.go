package handlers

import (
	"log"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// HandleRoleDelete stamps unprovisioned_since on a user when their last role is removed.
//
// This gives long-tenured users a fresh 7-day grace period instead of being immediately
// disabled based on their original account creation date. All unprovisioned warning
// timestamps are cleared so the notification sequence fires from scratch.
//
// It should be called from OnRecordAfterDeleteSuccess("roles").
func HandleRoleDelete(e *core.RecordEvent) {
	userID := e.Record.GetString("user")
	if userID == "" {
		return
	}

	var result struct {
		Count int `db:"cnt"`
	}
	err := e.App.DB().NewQuery(
		"SELECT COUNT(*) AS cnt FROM roles WHERE user = {:uid}",
	).Bind(map[string]any{"uid": userID}).One(&result)
	if err != nil {
		log.Printf("HandleRoleDelete: could not count remaining roles for user %s: %v", userID, err)
		return
	}

	if result.Count > 0 {
		return
	}

	user, err := e.App.FindRecordById("users", userID)
	if err != nil {
		log.Printf("HandleRoleDelete: could not find user %s: %v", userID, err)
		return
	}

	now := time.Now().UTC()
	user.Set("unprovisioned_since", now)
	// Clear warning timestamps so the fresh 7-day clock triggers new warnings.
	user.Set("unprovisioned_warning_sent_at", nil)
	user.Set("unprovisioned_final_warning_sent_at", nil)
	user.Set("admin_alerted_at", nil)

	if err := e.App.SaveNoValidate(user); err != nil {
		log.Printf("HandleRoleDelete: could not stamp unprovisioned_since for user %s: %v", userID, err)
		return
	}

	log.Printf("HandleRoleDelete: user %s has no remaining roles — unprovisioned_since set to %s", userID, now.Format(time.RFC3339))
}
