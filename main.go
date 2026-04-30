package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"ministry-mapper/internal/handlers"
	"ministry-mapper/internal/jobs"
	"ministry-mapper/internal/setup"
	_ "ministry-mapper/migrations"

	"github.com/getsentry/sentry-go"
	"github.com/pocketbase/pocketbase"
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
	setup.RegisterRoutes(app)
	setup.RegisterDomainHooks(app)
	jobs.ConfigureScheduler(app)
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

func registerSentryLogForwarding(app core.App) {
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
