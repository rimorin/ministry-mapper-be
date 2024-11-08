package jobs

import (
	"os"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	ld "github.com/launchdarkly/go-server-sdk/v7"
	"github.com/pocketbase/pocketbase"
)

// ConfigureScheduler initializes and starts the application's background tasks
// using a cron-based scheduler. It sets up periodic tasks for territory management
// and message processing.
func ConfigureScheduler(app *pocketbase.PocketBase) {
	ldClient, _ := ld.MakeClient(os.Getenv("LAUNCHDARKLY_SDK_KEY"), 5*time.Second)
	universalContext := ldcontext.NewWithKind("environment", os.Getenv("LAUNCHDARKLY_CONTEXT_KEY"))
	scheduler := app.Cron()

	// Helper function to add tasks to the scheduler
	addTask := func(name, schedule, flagKey string, task func()) {
		scheduler.MustAdd(name, schedule, func() {
			enabled, err := ldClient.BoolVariation(flagKey, universalContext, true)
			if err == nil && enabled {
				task()
			}
		})
	}

	// Clean up expired or invalid territory assignments (every 5 minutes)
	addTask("cleanUpAssignments", "*/5 * * * *", "enable-assignments-cleanup", func() {
		assignmentsCleanup(app)
	})

	// Update territory statistics and aggregated data (every 10 minutes)
	addTask("updateTerritoryAggregates", "*/10 * * * *", "enable-territory-aggregations", func() {
		updateTerritoryAggregates(app, 10)
	})

	// Process pending messages in the message queue (every 30 minutes)
	addTask("processMessages", "*/30 * * * *", "enable-message-processing", func() {
		processMessages(app, 30)
	})

	// Process pending instructions for territory assignments (every 30 minutes)
	addTask("processInstructions", "*/30 * * * *", "enable-instruction-processing", func() {
		processInstructions(app, 30)
	})

	// Process updated notes for congregations (every hour)
	addTask("processNotes", "0 * * * *", "enable-note-processing", func() {
		ProcessNotes(app, 60)
	})

	// Start the scheduler to begin processing all configured tasks
	scheduler.Start()
}
