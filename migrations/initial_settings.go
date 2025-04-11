package migrations

import (
	"os"
	"strconv"

	"github.com/getsentry/sentry-go"
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		settings := app.Settings()

		// Get app name and URL from environment variables
		appName := os.Getenv("PB_APP_NAME")
		if appName == "" {
			appName = "Ministry Mapper" // fallback value
		}

		appURL := os.Getenv("PB_APP_URL")
		if appURL == "" {
			appURL = "https://frontend-ministry-mapper-page.com" // fallback value
		}

		// Get SMTP settings from environment variables
		smtpHost := os.Getenv("PB_SMTP_HOST")
		if smtpHost == "" {
			smtpHost = "smtp.gmail.com" // fallback value
		}

		smtpPortStr := os.Getenv("PB_SMTP_PORT")
		smtpPort, err := strconv.Atoi(smtpPortStr)
		if err != nil {
			sentry.CaptureException(err)
			smtpPort = 587 // fallback value
		}

		smtpUsername := os.Getenv("PB_SMTP_USERNAME")
		if smtpUsername == "" {
			smtpUsername = "smtp_account@mailprovider.com" // fallback value
		}

		smtpPassword := os.Getenv("PB_SMTP_PASSWORD")
		if smtpPassword == "" {
			smtpPassword = "defaultpassword" // fallback value
		}

		smtpSenderAddress := os.Getenv("PB_SMTP_SENDER_ADDRESS")
		if smtpSenderAddress == "" {
			smtpSenderAddress = "support@ministry-mapper.com"
		}

		smtpSenderName := os.Getenv("PB_SMTP_SENDER_NAME")
		if smtpSenderName == "" {
			smtpSenderName = "MM Support"
		}

		hideControls := os.Getenv("PB_HIDE_CONTROLS")
		hideControlsBool := false
		if hideControls != "" {
			hideControlsBool, _ = strconv.ParseBool(hideControls)
		}

		rateLimitingEnabled := os.Getenv("PB_ENABLE_RATE_LIMITING")
		rateLimitingEnabledBool := false
		if rateLimitingEnabled != "" {
			rateLimitingEnabledBool, _ = strconv.ParseBool(rateLimitingEnabled)
		}

		settings.Meta.AppName = appName
		settings.Meta.AppURL = appURL
		settings.Meta.SenderAddress = smtpSenderAddress
		settings.Meta.SenderName = smtpSenderName
		settings.SMTP.Enabled = true
		settings.SMTP.Host = smtpHost
		settings.SMTP.Port = smtpPort
		settings.SMTP.Username = smtpUsername
		settings.SMTP.Password = smtpPassword
		settings.Meta.HideControls = hideControlsBool
		settings.RateLimits.Enabled = rateLimitingEnabledBool

		return app.Save(settings)
	}, nil)
}
