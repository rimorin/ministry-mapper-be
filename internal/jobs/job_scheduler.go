package jobs

import (
	"log"
	"ministry-mapper/internal/middleware"
	"os"
	"sync"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
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

	// ---------------------------------------------------------------------------
	// SCHEDULE DESIGN NOTES (all times shown as UTC / SGT = UTC+8)
	//
	// Peak usage window: 08:00–12:00 SGT (00:00–04:00 UTC) on weekends.
	//
	// Rules applied:
	//   1. No two jobs fire at the same minute (prevents CPU pile-up).
	//   2. Frequent jobs (≤30 min) are offset from :00 to spread load across
	//      the hour — they cannot avoid peak hours since they run all day.
	//   3. Infrequent jobs (daily / monthly) that are NOT time-sensitive are
	//      moved to 02:00–02:30 SGT (18:00–18:30 UTC) — deep off-peak, well
	//      clear of the morning field-service window.
	// ---------------------------------------------------------------------------

	// Every 5 min — offset by 1 min so it never fires on the hour with other jobs.
	// Must run during peak: assignments expire and maps need freeing in real time.
	// SGT examples: :01, :06, :11, :16 ... (all day)
	addTask("cleanUpAssignments", "1,6,11,16,21,26,31,36,41,46,51,56 * * * *", "enable-assignments-cleanup", func() error {
		return assignmentsCleanup(app)
	})

	// Every 10 min — offset by 4 min.
	// Recalculates map and territory aggregates for any maps/territories with
	// address status changes in the past 11 minutes (11 > 10 to absorb
	// sub-second scheduler jitter without missing log entries). Uses
	// addresses_log as the change-detection source.
	// SGT examples: :04, :14, :24, :34, :44, :54 (all day)
	addTask("updateTerritoryAggregates", "4,14,24,34,44,54 * * * *", "enable-territory-aggregations", func() error {
		return updateTerritoryAggregates(app, 11)
	})

	// Every 30 min — at :08 and :38, 10 min after updateTerritoryAggregates.
	// Must run during peak: publishers receive messages while actively working.
	// SGT examples: :08, :38 (all day)
	addTask("processMessages", "8,38 * * * *", "enable-message-processing", func() error {
		return processMessages(app, 30)
	})

	// Every 30 min — at :18 and :48, spaced 10 min from processMessages.
	// Must run during peak: publishers need territory instructions while working.
	// SGT examples: :18, :48 (all day)
	addTask("processInstructions", "18,48 * * * *", "enable-instruction-processing", func() error {
		return processInstructions(app, 30)
	})

	// Every hour — at :28, midway between the other 30-min jobs.
	// Notes are not time-critical but run hourly to stay reasonably fresh.
	// SGT examples: :28 every hour
	addTask("processNotes", "28 * * * *", "enable-note-processing", func() error {
		return ProcessNotes(app, 60)
	})

	// Monthly on the 1st — at 18:00 UTC (02:00 SGT), deep off-peak.
	// Heavy job: reads all congregation data, builds Excel workbook, sends email
	// to all administrators. Previously ran at 00:00 UTC (08:00 SGT) which
	// collided with the peak field-service window on the 1st of each month.
	addTask("generateMonthlyReport", "0 18 1 * *", "enable-monthly-report", func() error {
		aiEnabled, _ := ldClient.BoolVariation("enable-report-ai-summary", universalContext, false)
		GenerateMonthlyReport(app, aiEnabled)
		return nil
	})

	// Daily — at 18:00 UTC (02:00 SGT), deep off-peak.
	// Warns and disables users with no role assignment.
	// Previously ran at 01:00 UTC (09:00 SGT), 1 hour before the peak window.
	// Aligns with NIST SP 800-53 AC-2 least-privilege deprovisioning.
	addTask("processUnprovisionedUsers", "0 18 * * *", "enable-unprovisioned-user-processing", func() error {
		return processUnprovisionedUsers(app)
	})

	// Daily — at 18:30 UTC (02:30 SGT), 30 min after processUnprovisionedUsers.
	// Warns and disables inactive users.
	// Previously ran at 01:30 UTC (09:30 SGT), 30 min before the peak window.
	// Aligns with NIST SP 800-53 AC-2(3) automatic disabling of inactive accounts.
	addTask("processInactiveUsers", "30 18 * * *", "enable-inactive-user-processing", func() error {
		return processInactiveUsers(app)
	})

	// Daily — at 19:00 UTC (03:00 SGT), staggered 30 min after processInactiveUsers.
	// Sends a digest email per congregation listing addresses added via the app
	// (source = "app") in the past 24 hours. Not time-sensitive; runs at off-peak.
	addTask("processNewAddresses", "0 19 * * *", "enable-new-addresses-notification", func() error {
		return ProcessNewAddresses(app, time.Now().UTC().Add(-24*time.Hour))
	})

	// Start the scheduler to begin processing all configured tasks
	scheduler.Start()
}
