package jobs

import (
	"log"
	"time"

	"ministry-mapper/internal/handlers"

	"github.com/pocketbase/pocketbase/core"
)

// RunAssignmentsCleanup is the exported entry point used by tests.
func RunAssignmentsCleanup(app core.App) error {
	return assignmentsCleanup(app)
}

// assignmentsCleanup deletes all expired assignments within a transaction.
func assignmentsCleanup(app core.App) error {
	log.Println("Starting assignments cleanup")

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

	// txApp.Delete (rather than raw SQL) so hooks/realtime events fire per record.
	err = app.RunInTransaction(func(txApp core.App) error {
		for _, assignment := range assignments {
			handlers.LogAssignmentExpired(txApp, assignment)
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
