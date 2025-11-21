package jobs

import (
	"ministry-mapper/internal/middleware"
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

	addTask := func(name, schedule, flagKey string, task func() error) {
		scheduler.MustAdd(name, schedule, func() {
			if enabled, _ := ldClient.BoolVariation(flagKey, universalContext, true); enabled {
				middleware.WithJobRecovery(name, task)
			}
		})
	}

	// Clean up expired or invalid territory assignments (every 5 minutes)
	addTask("cleanUpAssignments", "*/5 * * * *", "enable-assignments-cleanup", func() error {
		return assignmentsCleanup(app)
	})

	// Update territory statistics and aggregated data (every 10 minutes)
	addTask("updateTerritoryAggregates", "*/10 * * * *", "enable-territory-aggregations", func() error {
		return updateTerritoryAggregates(app, 10)
	})

	// Process pending messages in the message queue (every 30 minutes)
	addTask("processMessages", "*/30 * * * *", "enable-message-processing", func() error {
		return processMessages(app, 30)
	})

	// Process pending instructions for territory assignments (every 30 minutes)
	addTask("processInstructions", "*/30 * * * *", "enable-instruction-processing", func() error {
		return processInstructions(app, 30)
	})

	// Process updated notes for congregations (every hour)
	addTask("processNotes", "0 * * * *", "enable-note-processing", func() error {
		return ProcessNotes(app, 60)
	})

	// Generate an excel report of congregations (every month on the 1st at midnight)
	addTask("generateMonthlyReport", "0 0 1 * *", "enable-monthly-report", func() error {
		GenerateMonthlyReport(app)
		return nil
	})

	// Start the scheduler to begin processing all configured tasks
	scheduler.Start()
}
