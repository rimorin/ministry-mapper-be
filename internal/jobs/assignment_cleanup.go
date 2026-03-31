package jobs

import (
	"log"
	"time"

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

	// Fetch full records in one query — avoids a second FindRecordById per record inside the loop.
	assignments, err := app.FindRecordsByFilter(
		"assignments",
		"expiry_date < {:current_date}",
		"", 0, 0,
		map[string]any{"current_date": time.Now().UTC()},
	)
	if err != nil {
		log.Printf("Cleanup failed: %v", err)
		return err
	}

	if len(assignments) == 0 {
		log.Println("Completed: No expired assignments found")
		return nil
	}

	// Delete each record. Running inside a transaction keeps the deletions atomic
	// and each txApp.Delete call fires PocketBase hooks/realtime events as expected.
	err = app.RunInTransaction(func(txApp core.App) error {
		for _, assignment := range assignments {
			if err := txApp.Delete(assignment); err != nil {
				log.Printf("Error deleting assignment with ID: %s, %v", assignment.Id, err)
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
