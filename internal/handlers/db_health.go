package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

func healthResponse(code int, message string) map[string]any {
	return map[string]any{
		"code":    code,
		"message": message,
		"data":    map[string]any{},
	}
}

// HandleDBHealth runs a SQLite quick_check and returns 200 if the database is
// healthy, or 503 if the check fails. Intended for cloud platform health probes
// (no authentication required).
//
// Uses PRAGMA quick_check rather than SELECT 1 because simple queries succeed
// even on a corrupted database — index corruption is only caught by a structural
// scan. quick_check detects corrupted pages, missing index entries, and malformed
// records without the full cost of integrity_check.
//
// A 10-second timeout is enforced so the endpoint always responds promptly even
// if SQLite is stuck scanning a corrupted page.
func HandleDBHealth(e *core.RequestEvent, app *pocketbase.PocketBase) error {
	var result struct {
		Result string `db:"quick_check"`
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := app.DB().NewQuery("PRAGMA quick_check").WithContext(ctx).One(&result); err != nil {
		sentry.CaptureException(fmt.Errorf("db health check failed: %w", err))
		return e.JSON(http.StatusServiceUnavailable, healthResponse(http.StatusServiceUnavailable, "Database health check failed."))
	}

	if result.Result != "ok" {
		sentry.CaptureException(fmt.Errorf("db integrity issue: %s", result.Result))
		return e.JSON(http.StatusServiceUnavailable, healthResponse(http.StatusServiceUnavailable, result.Result))
	}

	return e.JSON(http.StatusOK, healthResponse(http.StatusOK, "Database is healthy."))
}
