package jobs

import (
	"log"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

// assignmentsCleanup removes expired assignments from the database.
// It fetches all assignments that have an expiry date earlier than the current date,
// and deletes them within a transaction. If no expired assignments are found, it logs
// a message and returns without making any changes.
//
// Parameters:
//   - app: A pointer to the PocketBase application instance.
//
// Returns:
//   - error: An error if the cleanup process fails, otherwise nil.
func assignmentsCleanup(app *pocketbase.PocketBase) error {
	log.Println("Starting assignments cleanup")

	// Fetch all assignments that have expired
	type AssignmentData struct {
		ID string `db:"id"`
	}
	assignments := []AssignmentData{}
	err := app.DB().Select("assignments.id").From("assignments").Where(dbx.NewExp("expiry_date < {:current_date}", dbx.Params{"current_date": time.Now().UTC()})).All(&assignments)
	if err != nil {
		log.Printf("Cleanup failed: %v", err)
		return err
	}

	// If no expired assignments found, return
	if len(assignments) == 0 {
		log.Println("Completed: No expired assignments found")
		return nil
	}

	// Delete all expired assignments
	err = app.RunInTransaction(func(txApp core.App) error {
		for _, assignment_id := range assignments {
			assignment, _ := txApp.FindRecordById("assignments", assignment_id.ID)
			if err := txApp.Delete(assignment); err != nil {
				log.Printf("Error deleting assignment with ID: %s, %v", assignment_id.ID, err)
				return err
			}
		}
		return nil
	})

	if err != nil {
		log.Printf("Cleanup failed: %v", err)
		return err
	}

	log.Printf("Assignments cleanup completed: %d assignments removed", len(assignments))
	return nil
}
