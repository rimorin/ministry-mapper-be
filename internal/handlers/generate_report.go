package handlers

import (
	"net/http"

	sentry "github.com/getsentry/sentry-go"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

type GenerateReportRequest struct {
	Congregation string `json:"congregation"`
}

// ReportGeneratorFn is the function signature for congregation report generation.
// Injected from the jobs package at registration time to avoid import cycles.
type ReportGeneratorFn func(app *pocketbase.PocketBase, congregation *core.Record, recipient *core.Record) error

// HandleGenerateReport triggers an on-demand Excel report for a congregation.
// The authenticated user must have the administrator role for the specified congregation.
// The report is generated asynchronously and emailed only to the requesting user.
func HandleGenerateReport(c *core.RequestEvent, app *pocketbase.PocketBase, generator ReportGeneratorFn) error {
	data := GenerateReportRequest{}
	if err := c.BindBody(&data); err != nil {
		return apis.NewBadRequestError("Invalid request body", nil)
	}

	if data.Congregation == "" {
		return apis.NewBadRequestError("congregation is required", nil)
	}

	_, err := app.FindFirstRecordByFilter(
		"roles",
		"user = {:user} && congregation = {:congregation} && role = 'administrator'",
		dbx.Params{
			"user":         c.Auth.Id,
			"congregation": data.Congregation,
		},
	)
	if err != nil {
		return apis.NewForbiddenError("Not an administrator for this congregation", nil)
	}

	congregation, err := app.FindRecordById("congregations", data.Congregation)
	if err != nil {
		return apis.NewNotFoundError("Congregation not found", nil)
	}

	congregationID := congregation.Id
	recipientID := c.Auth.Id

	go func() {
		cong, err := app.FindRecordById("congregations", congregationID)
		if err != nil {
			sentry.CaptureException(err)
			return
		}
		recipient, err := app.FindRecordById("users", recipientID)
		if err != nil {
			sentry.CaptureException(err)
			return
		}
		if err := generator(app, cong, recipient); err != nil {
			sentry.CaptureException(err)
		}
	}()

	return c.JSON(http.StatusAccepted, map[string]any{
		"message": "Report generation started. You will receive an email shortly.",
	})
}
