package main

import (
	"fmt"
	"log"
	"ministry-mapper/internal/handlers"
	"ministry-mapper/internal/jobs"
	"ministry-mapper/internal/middleware"
	"os"
	"strings"
	"time"

	_ "ministry-mapper/migrations"

	"github.com/getsentry/sentry-go"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"
)

func main() {
	buildVersion := os.Getenv("SOURCE_COMMIT")
	if buildVersion == "" {
		buildVersion = "development-build"
	}
	log.Printf("Starting Ministry Mapper build %s\n", buildVersion)

	initSentry(buildVersion)
	defer sentry.Flush(2 * time.Second)

	app := pocketbase.New()

	handlers.RegisterAuthHooks(app)
	registerRoutes(app)
	registerDomainHooks(app)
	registerSentryLogForwarding(app)

	isGoRun := strings.HasPrefix(os.Args[0], os.TempDir())
	migratecmd.MustRegister(app, app.RootCmd, migratecmd.Config{
		Automigrate: isGoRun,
	})

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}

func initSentry(buildVersion string) {
	sentryEnv := os.Getenv("SENTRY_ENV")
	if sentryEnv == "" {
		sentryEnv = "development"
	}

	opts := sentry.ClientOptions{
		Dsn:              os.Getenv("SENTRY_DSN"),
		Environment:      sentryEnv,
		Release:          buildVersion,
		AttachStacktrace: true,
		EnableLogs:       true,
		SampleRate:       1.0,
	}

	switch sentryEnv {
	case "production":
		opts.TracesSampleRate = 0.2
	case "staging":
		opts.TracesSampleRate = 0.5
	default:
		opts.Debug = true
	}

	if err := sentry.Init(opts); err != nil {
		log.Fatalf("sentry.Init: %s", err)
	}
}

func registerRoutes(app *pocketbase.PocketBase) {
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		allowOrigins := os.Getenv("PB_ALLOW_ORIGINS")
		if allowOrigins == "" {
			allowOrigins = "*"
		}
		e.Router.Bind(apis.CORS(apis.CORSConfig{
			AllowOrigins: strings.Split(allowOrigins, ","),
		}))

		authRoute := func(path string, handler func(*core.RequestEvent) error) {
			e.Router.POST(path, middleware.WrapHandler(handler)).Bind(apis.RequireAuth())
		}

		// Custom endpoint: handles its own auth (supports link-id)
		e.Router.POST("/map/addresses", middleware.WrapHandler(func(c *core.RequestEvent) error {
			return handlers.HandleGetMapAddresses(c, app)
		}))

		// Map operations
		authRoute("/map/codes", func(c *core.RequestEvent) error {
			return handlers.HandleGetMapCodes(c, app)
		})
		authRoute("/map/code/add", func(c *core.RequestEvent) error {
			return handlers.HandleMapAdd(c, app)
		})
		authRoute("/map/codes/update", func(c *core.RequestEvent) error {
			return handlers.HandleMapUpdateSequence(c, app)
		})
		authRoute("/map/code/delete", func(c *core.RequestEvent) error {
			return handlers.HandleMapDelete(c, app)
		})
		authRoute("/map/floor/add", func(c *core.RequestEvent) error {
			return handlers.HandleMapFloor(c, app)
		})
		authRoute("/map/floor/remove", func(c *core.RequestEvent) error {
			return handlers.HandleRemoveMapFloor(c, app)
		})
		authRoute("/map/reset", func(c *core.RequestEvent) error {
			return handlers.HandleResetMap(c, app)
		})
		authRoute("/map/add", func(c *core.RequestEvent) error {
			return handlers.HandleNewMap(c, app)
		})
		authRoute("/map/territory/update", func(c *core.RequestEvent) error {
			return handlers.HandleMapTerritoryUpdate(c, app)
		})

		// Territory operations
		authRoute("/territory/reset", func(c *core.RequestEvent) error {
			return handlers.HandleResetTerritory(c, app)
		})
		authRoute("/territory/delete", func(c *core.RequestEvent) error {
			return handlers.HandleDeleteTerritory(c, app)
		})
		authRoute("/territory/link", func(c *core.RequestEvent) error {
			return handlers.HandleTerritoryQuicklink(c, app)
		})

		// Options
		authRoute("/options/update", func(c *core.RequestEvent) error {
			return handlers.HandleOptionUpdate(c, app)
		})

		// Reports
		authRoute("/report/generate", func(c *core.RequestEvent) error {
			return handlers.HandleGenerateReport(c, app, jobs.GenerateAndSendCongregationReportToUser)
		})

		// Health check
		e.Router.GET("/api/db-health", func(c *core.RequestEvent) error {
			return handlers.HandleDBHealth(c, app)
		})

		// TEMPORARY: one-off endpoint to trigger new-address digest for historical records.
		// Remove after use.
		e.Router.POST("/api/admin/trigger-new-addresses", func(c *core.RequestEvent) error {
			sinceStr := c.Request.URL.Query().Get("since")
			since, err := time.Parse(time.RFC3339, sinceStr)
			if err != nil {
				return apis.NewBadRequestError("since param required, e.g. ?since=2026-04-01T00:00:00Z", nil)
			}
			return jobs.ProcessNewAddresses(app, since)
		}).Bind(apis.RequireSuperuserAuth())

		jobs.ConfigureScheduler(app)

		return e.Next()
	})
}

func registerDomainHooks(app *pocketbase.PocketBase) {
	// Track notes changes on address updates
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

	// Log address status changes for audit trail
	app.OnRecordAfterUpdateSuccess("addresses").BindFunc(func(e *core.RecordEvent) error {
		handlers.LogAddressStatusChange(e)
		return e.Next()
	})

	// Track last login and reset inactive warnings
	app.OnRecordAuthRequest("users").BindFunc(func(e *core.RecordAuthRequestEvent) error {
		e.Record.Set("last_login", time.Now())
		e.Record.Set("inactive_warning_sent_at", nil)
		e.Record.Set("inactive_final_warning_sent_at", nil)
		if err := e.App.SaveNoValidate(e.Record); err != nil {
			log.Printf("warning: error saving last_login for user %s: %v", e.Record.Id, err)
		}
		return e.Next()
	})

	// Normalize email on user creation
	app.OnRecordCreate("users").BindFunc(func(e *core.RecordEvent) error {
		email := e.Record.Get("email").(string)
		e.Record.Set("email", strings.ToLower(strings.TrimSpace(email)))
		e.Record.SetEmailVisibility(true)
		return e.Next()
	})

	// Stamp unprovisioned_since when a user's last role is deleted
	app.OnRecordAfterDeleteSuccess("roles").BindFunc(func(e *core.RecordEvent) error {
		handlers.HandleRoleDelete(e)
		return e.Next()
	})
}

func registerSentryLogForwarding(app *pocketbase.PocketBase) {
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
}
