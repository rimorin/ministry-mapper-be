package main

import (
	"log"
	"ministry-mapper/internal/handlers"
	"ministry-mapper/internal/jobs"
	"os"
	"strings"
	"time"

	_ "ministry-mapper/migrations"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"
)

func main() {
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

		bindAuthenticatedRoute("/map/code/add", func(c *core.RequestEvent) error {
			return handlers.HandleMapAdd(c, app)
		})

		bindAuthenticatedRoute("/map/code/update", func(c *core.RequestEvent) error {
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

		bindAuthenticatedRoute("/map/add", func(c *core.RequestEvent) error {
			return handlers.HandleNewMap(c, app)
		})

		bindAuthenticatedRoute("/map/territory/update", func(c *core.RequestEvent) error {
			return handlers.HandleMapTerritoryUpdate(c, app)
		})

		bindAuthenticatedRoute("/options/update", func(c *core.RequestEvent) error {
			return handlers.HandleOptionUpdate(c, app)
		})

		e.Router.POST("/batch/addresses", func(c *core.RequestEvent) error {
			if err := handlers.CreateAddress(app, c); err != nil {
				return err
			}
			return e.Next()
		}).Bind(apis.RequireSuperuserAuth())

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
		log.Fatal(err)
	}
}
