package jobs

import (
	"log"
	"ministry-mapper/internal/middleware"
	"os"
	"sync"
	"time"

	"github.com/launchdarkly/go-sdk-common/v4/ldcontext"
	ld "github.com/launchdarkly/go-server-sdk/v7"
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
func ConfigureScheduler(app core.App) {
	var ldClient *ld.LDClient
	if key := os.Getenv("LAUNCHDARKLY_SDK_KEY"); key != "" {
		var err error
		ldClient, err = ld.MakeClient(key, 5*time.Second)
		if err != nil {
			log.Printf("LaunchDarkly client initialisation error: %v", err)
		}
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
			// When LD is not configured, default all feature flags to enabled.
			enabled := true
			if ldClient != nil {
				enabled, _ = ldClient.BoolVariation(flagKey, universalContext, true)
			}
			if enabled {
				middleware.WithJobRecovery(name, task)
			}
		})
	}

	// Schedules are staggered so no two jobs fire at the same minute, and
	// daily/monthly jobs run at 18:00–19:00 UTC (02:00–03:00 SGT), clear of the
	// weekend 08:00–12:00 SGT peak field-service window.

	// Every 5 min: assignments expire and maps need freeing in real time.
	addTask("cleanUpAssignments", "1,6,11,16,21,26,31,36,41,46,51,56 * * * *", "enable-assignments-cleanup", func() error {
		return assignmentsCleanup(app)
	})

	// Every 30 min: publishers receive messages while actively working.
	addTask("processMessages", "8,38 * * * *", "enable-message-processing", func() error {
		return processMessages(app, 30)
	})

	// Every 30 min: publishers need territory instructions while working.
	addTask("processInstructions", "18,48 * * * *", "enable-instruction-processing", func() error {
		return processInstructions(app, 30)
	})

	// Hourly: notes are not time-critical but should stay reasonably fresh.
	addTask("processNotes", "28 * * * *", "enable-note-processing", func() error {
		return ProcessNotes(app, 60)
	})

	// Monthly on the 1st — at 18:00 UTC (02:00 SGT), deep off-peak.
	// Heavy job: reads all congregation data, builds Excel workbook, sends email
	// to all administrators.
	addTask("generateMonthlyReport", "0 18 1 * *", "enable-monthly-report", func() error {
		GenerateMonthlyReport(app, IsAISummaryEnabled())
		return nil
	})

	// Daily — at 18:00 UTC (02:00 SGT), deep off-peak.
	// Warns and disables users with no role assignment.
	addTask("processUnprovisionedUsers", "0 18 * * *", "enable-unprovisioned-user-processing", func() error {
		return processUnprovisionedUsers(app)
	})

	// Daily — at 18:30 UTC (02:30 SGT), 30 min after processUnprovisionedUsers.
	// Warns and disables inactive users.
	addTask("processInactiveUsers", "30 18 * * *", "enable-inactive-user-processing", func() error {
		return processInactiveUsers(app)
	})

	// Daily — at 19:00 UTC (03:00 SGT).
	// Digest email per congregation listing addresses added via the app
	// (source = "app") in the past 24 hours.
	addTask("processNewAddresses", "0 19 * * *", "enable-new-addresses-notification", func() error {
		return ProcessNewAddresses(app, time.Now().UTC().Add(-24*time.Hour))
	})

	scheduler.Start()
}
