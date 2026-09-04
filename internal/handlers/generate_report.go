package handlers

import (
	"fmt"
	"net/http"

	sentry "github.com/getsentry/sentry-go"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/security"
)

// ReportInflightKey is the app store key that marks a congregation's report as
// being generated. A full report for the largest congregation takes ~10s and
// several hundred MB, so the handler refuses a second request for the same
// congregation until the first finishes instead of running them side by side.
func ReportInflightKey(congregationId string) string {
	return "report_inflight:" + congregationId
}

type GenerateReportRequest struct {
	Congregation string `json:"congregation"`
}

// ReportGeneratorFn is the function signature for congregation report generation.
// Injected from the jobs package at registration time to avoid import cycles.
type ReportGeneratorFn func(app core.App, congregation *core.Record, recipient *core.Record) error

// HandleGenerateReport triggers an on-demand Excel report for a congregation.
// The authenticated user must have the administrator role for the specified congregation.
// The report is generated asynchronously and emailed only to the requesting user.
func HandleGenerateReport(c *core.RequestEvent, app core.App, generator ReportGeneratorFn) error {
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

	// GetOrSet is atomic, so of two concurrent requests exactly one sees its own
	// token come back and proceeds; the other gets the winner's token.
	inflightKey := ReportInflightKey(congregationID)
	token := security.RandomString(16)
	if app.Store().GetOrSet(inflightKey, func() any { return token }) != token {
		return apis.NewApiError(http.StatusConflict, "A report for this congregation is already being generated", nil)
	}

	go func() {
		defer app.Store().Remove(inflightKey)
		defer func() {
			if r := recover(); r != nil {
				sentry.CaptureException(fmt.Errorf("report generation panic: %v", r))
			}
		}()
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
