package jobs

import (
	"log"
	"ministry-mapper/internal/middleware"
	"os"
	"sync"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	ld "github.com/launchdarkly/go-server-sdk/v7"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

// sharedLDClient and sharedLDContext are initialised once in ConfigureScheduler
// and reused by IsAISummaryEnabled for on-demand report requests.
var (
	ldInitOnce      sync.Once
	sharedLDClient  *ld.LDClient
	sharedLDContext ldcontext.Context
)

// IsAISummaryEnabled returns true if the enable-report-ai-summary LaunchDarkly
// flag is on. Falls back to false if the LD client was not yet initialised.
func IsAISummaryEnabled() bool {
	ldInitOnce.Do(func() {}) // synchronise with ConfigureScheduler's writes
	if sharedLDClient == nil {
		return false
	}
	enabled, _ := sharedLDClient.BoolVariation("enable-report-ai-summary", sharedLDContext, false)
	return enabled
}

// ConfigureScheduler initializes and starts the application's background tasks
// using a cron-based scheduler. It sets up periodic tasks for territory management
// and message processing.
func ConfigureScheduler(app *pocketbase.PocketBase) {
	ldClient, err := ld.MakeClient(os.Getenv("LAUNCHDARKLY_SDK_KEY"), 5*time.Second)
	if err != nil {
		log.Printf("LaunchDarkly client initialisation error: %v", err)
	}
	universalContext := ldcontext.NewWithKind("environment", os.Getenv("LAUNCHDARKLY_CONTEXT_KEY"))

	ldInitOnce.Do(func() {
		sharedLDClient = ldClient
		sharedLDContext = universalContext
	})

	if ldClient != nil {
		app.OnTerminate().BindFunc(func(e *core.TerminateEvent) error {
			ldClient.Close()
			return e.Next()
		})
	}

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
		aiEnabled, _ := ldClient.BoolVariation("enable-report-ai-summary", universalContext, false)
		GenerateMonthlyReport(app, aiEnabled)
		return nil
	})

	// Start the scheduler to begin processing all configured tasks
	scheduler.Start()
}
