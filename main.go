package main

import (
	"log"
	"ministry-mapper/internal/handlers"
	"ministry-mapper/internal/jobs"
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
	// Coolify sets the SOURCE_COMMIT environment variable to the commit hash of the current build.
	buildVersion := os.Getenv("SOURCE_COMMIT")
	if buildVersion == "" {
		buildVersion = "development-build"
	}
	log.Printf("Starting Ministry Mapper build %s\n", buildVersion)
	sentryEnv := os.Getenv("SENTRY_ENV")

	err := sentry.Init(sentry.ClientOptions{
		Dsn:         os.Getenv("SENTRY_DSN"),
		Environment: sentryEnv,
		Release:     buildVersion,
	})
	if err != nil {
		log.Fatalf("sentry.Init: %s", err)
	}
	// Flush buffered events before the program terminates.
	// Set the timeout to the maximum duration the program can afford to wait.
	defer sentry.Flush(2 * time.Second)

	app := pocketbase.New()

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
			e.Router.POST(path, handler).Bind(apis.RequireAuth())
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
		mapId := e.Record.Get("map").(string)
		handlers.ProcessMapAggregates(mapId, app, false)
		return e.Next()
	})

	app.OnRecordAuthRequest("users").BindFunc(func(e *core.RecordAuthRequestEvent) error {
		e.Record.Set("last_login", time.Now())
		if err := e.App.SaveNoValidate(e.Record); err != nil {
			log.Printf("error saving last login: %v", err)
			return err
		}
		return e.Next()
	})

	// This hook is executed before a new record is created in the "users" table
	app.OnRecordCreate("users").BindFunc(func(e *core.RecordEvent) error {
		email := e.Record.Get("email").(string)
		// Clean up and lower case the email
		email = strings.ToLower(strings.TrimSpace(email))
		e.Record.Set("email", email)
		return e.Next()
	})

	isGoRun := strings.HasPrefix(os.Args[0], os.TempDir())

	migratecmd.MustRegister(app, app.RootCmd, migratecmd.Config{
		Automigrate: isGoRun,
	})

	if err := app.Start(); err != nil {
		sentry.CaptureException(err)
		log.Fatal(err)
	}
}
