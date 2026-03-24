package main

import (
	"fmt"
	"log"
	"ministry-mapper/internal/handlers"
	"ministry-mapper/internal/jobs"
	"ministry-mapper/internal/middleware"
	"os"
	"strings"
	"sync"
	"time"

	_ "ministry-mapper/migrations"

	"github.com/getsentry/sentry-go"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"
)

// aggregateDebouncer coalesces bursts of address saves (e.g. a map reset saving
// 836 records one-by-one) into a single ProcessMapAggregates call per map.
// Each mapID has its own independent timer so concurrent updates to different
// maps don't interfere with each other.
//
// Per-row app.Save() calls are kept intact so PocketBase realtime events still
// fire for every individual address change — the frontend sees each update.
// Only the expensive aggregate recalculation is debounced.
type aggregateDebouncer struct {
	mu      sync.Mutex
	pending map[string]*time.Timer
	delay   time.Duration
}

func newAggregateDebouncer(delay time.Duration) *aggregateDebouncer {
	return &aggregateDebouncer{
		pending: make(map[string]*time.Timer),
		delay:   delay,
	}
}

// schedule arms (or re-arms) the debounce timer for a mapID. Uses Stop() +
// new AfterFunc rather than Reset() to avoid a subtle edge case: if the timer
// has just fired but its goroutine hasn't yet removed the entry from pending,
// Reset() would re-arm the already-expired timer causing a duplicate run.
func (d *aggregateDebouncer) schedule(mapID string, app *pocketbase.PocketBase) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if timer, ok := d.pending[mapID]; ok {
		timer.Stop()
	}

	d.pending[mapID] = time.AfterFunc(d.delay, func() {
		d.mu.Lock()
		delete(d.pending, mapID)
		d.mu.Unlock()
		if err := handlers.ProcessMapAggregates(mapID, app, false); err != nil {
			log.Printf("aggregate debounce error for map %s: %v", mapID, err)
		}
	})
}

// cancel stops all pending timers. Call on application shutdown to prevent
// callbacks from firing against a stopping app.
func (d *aggregateDebouncer) cancel() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for mapID, timer := range d.pending {
		timer.Stop()
		delete(d.pending, mapID)
	}
}

func main() {
	// Coolify sets the SOURCE_COMMIT environment variable to the commit hash of the current build.
	buildVersion := os.Getenv("SOURCE_COMMIT")
	if buildVersion == "" {
		buildVersion = "development-build"
	}
	log.Printf("Starting Ministry Mapper build %s\n", buildVersion)
	sentryEnv := os.Getenv("SENTRY_ENV")
	if sentryEnv == "" {
		sentryEnv = "development"
	}

	sentryOptions := sentry.ClientOptions{
		Dsn:              os.Getenv("SENTRY_DSN"),
		Environment:      sentryEnv,
		Release:          buildVersion,
		AttachStacktrace: true,
		EnableLogs:       true,
		SampleRate:       1.0,
	}

	switch sentryEnv {
	case "production":
		sentryOptions.TracesSampleRate = 0.2
	case "staging":
		sentryOptions.TracesSampleRate = 0.5
	default:
		sentryOptions.Debug = true
	}

	err := sentry.Init(sentryOptions)
	if err != nil {
		log.Fatalf("sentry.Init: %s", err)
	}
	// Flush buffered events before the program terminates.
	// Set the timeout to the maximum duration the program can afford to wait.
	defer sentry.Flush(2 * time.Second)

	app := pocketbase.New()
	debouncer := newAggregateDebouncer(500 * time.Millisecond)
	defer debouncer.cancel()

	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		allowOrigins := os.Getenv("PB_ALLOW_ORIGINS")
		if allowOrigins == "" {
			allowOrigins = "*"
		}
		origins := strings.Split(allowOrigins, ",")

		e.Router.Bind(apis.CORS(apis.CORSConfig{
			AllowOrigins: origins,
		}))

		bindAuthenticatedRoute := func(path string, handler func(*core.RequestEvent) error) {
			e.Router.POST(path, middleware.WrapHandler(handler)).Bind(apis.RequireAuth())
		}

		bindAuthenticatedRoute("/map/codes", func(c *core.RequestEvent) error {
			return handlers.HandleGetMapCodes(c, app)
		})

		bindAuthenticatedRoute("/map/code/add", func(c *core.RequestEvent) error {
			return handlers.HandleMapAdd(c, app)
		})

		bindAuthenticatedRoute("/map/codes/update", func(c *core.RequestEvent) error {
			return handlers.HandleMapUpdateSequence(c, app)
		})

		bindAuthenticatedRoute("/map/code/delete", func(c *core.RequestEvent) error {
			return handlers.HandleMapDelete(c, app)
		})

		bindAuthenticatedRoute("/map/floor/add", func(c *core.RequestEvent) error {
			return handlers.HandleMapFloor(c, app)
		})

		bindAuthenticatedRoute("/map/floor/remove", func(c *core.RequestEvent) error {
			return handlers.HandleRemoveMapFloor(c, app)
		})

		bindAuthenticatedRoute("/map/reset", func(c *core.RequestEvent) error {
			return handlers.HandleResetMap(c, app)
		})

		bindAuthenticatedRoute("/territory/reset", func(c *core.RequestEvent) error {
			return handlers.HandleResetTerritory(c, app)
		})

		bindAuthenticatedRoute("/territory/link", func(c *core.RequestEvent) error {
			return handlers.HandleTerritoryQuicklink(c, app)
		})

		bindAuthenticatedRoute("/map/add", func(c *core.RequestEvent) error {
			return handlers.HandleNewMap(c, app)
		})

		bindAuthenticatedRoute("/map/territory/update", func(c *core.RequestEvent) error {
			return handlers.HandleMapTerritoryUpdate(c, app)
		})

		bindAuthenticatedRoute("/options/update", func(c *core.RequestEvent) error {
			return handlers.HandleOptionUpdate(c, app)
		})

		jobs.ConfigureScheduler(app)

		bindAuthenticatedRoute("/report/generate", func(c *core.RequestEvent) error {
			return handlers.HandleGenerateReport(c, app, jobs.GenerateAndSendCongregationReportToUser)
		})

		return e.Next()
	})

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

	app.OnRecordAfterUpdateSuccess("addresses").BindFunc(func(e *core.RecordEvent) error {
		// Only status and not_home_tries affect aggregates. Skip the debounce
		// entirely for notes edits, sequence reorders, and other field changes.
		oldStatus, _ := e.Record.Original().Get("status").(string)
		newStatus, _ := e.Record.Get("status").(string)
		oldTries, _ := e.Record.Original().Get("not_home_tries").(float64)
		newTries, _ := e.Record.Get("not_home_tries").(float64)
		if oldStatus == newStatus && oldTries == newTries {
			return e.Next()
		}

		mapId := e.Record.Get("map").(string)
		debouncer.schedule(mapId, app)
		return e.Next()
	})

	app.OnRecordAfterUpdateSuccess("addresses").BindFunc(func(e *core.RecordEvent) error {
		handlers.LogAddressStatusChange(e)
		return e.Next()
	})

	app.OnRecordAuthRequest("users").BindFunc(func(e *core.RecordAuthRequestEvent) error {
		e.Record.Set("last_login", time.Now())
		// Reset inactive warning timestamps so a returning user gets fresh warnings
		// if they become inactive again in the future.
		e.Record.Set("inactive_warning_sent_at", nil)
		e.Record.Set("inactive_final_warning_sent_at", nil)
		if err := e.App.SaveNoValidate(e.Record); err != nil {
			// Log but don't block login — last_login is non-critical metadata.
			// A transient DB error should not prevent a valid user from authenticating.
			log.Printf("warning: error saving last_login for user %s: %v", e.Record.Id, err)
		}
		return e.Next()
	})

	// This hook is executed before a new record is created in the "users" table
	app.OnRecordCreate("users").BindFunc(func(e *core.RecordEvent) error {
		email := e.Record.Get("email").(string)
		// Clean up and lower case the email
		email = strings.ToLower(strings.TrimSpace(email))
		e.Record.Set("email", email)
		e.Record.SetEmailVisibility(true)
		return e.Next()
	})

	// When a role is deleted, stamp unprovisioned_since if the user has no remaining roles.
	app.OnRecordAfterDeleteSuccess("roles").BindFunc(func(e *core.RecordEvent) error {
		handlers.HandleRoleDelete(e)
		return e.Next()
	})

	app.OnModelCreate(core.LogsTableName).BindFunc(func(e *core.ModelEvent) error {
		l := e.Model.(*core.Log)

		var entry sentry.LogEntry
		switch l.Level {
		case -4:
			entry = sentry.NewLogger(e.Context).Error()
		case -3:
			entry = sentry.NewLogger(e.Context).Warn()
		case -2:
			entry = sentry.NewLogger(e.Context).Info()
		case -1:
			entry = sentry.NewLogger(e.Context).Debug()
		default:
			entry = sentry.NewLogger(e.Context).Info()
		}

		entry = entry.WithCtx(e.Context).
			String("id", l.Id).
			Int("level", l.Level).
			String("created", l.Created.Time().Format(time.RFC3339))

		for key, value := range l.Data {
			entry = entry.String("data_"+key, fmt.Sprint(value))
		}

		entry.Emit(l.Message)
		return e.Next()
	})

	isGoRun := strings.HasPrefix(os.Args[0], os.TempDir())

	migratecmd.MustRegister(app, app.RootCmd, migratecmd.Config{
		Automigrate: isGoRun,
	})

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}
