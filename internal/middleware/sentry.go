package middleware

import (
	"fmt"
	"log"
	"runtime/debug"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
)

// WrapHandler wraps a PocketBase handler with Sentry error capture and panic recovery.
func WrapHandler(handler func(*core.RequestEvent) error) func(*core.RequestEvent) error {
	return func(c *core.RequestEvent) error {
		hub := sentry.CurrentHub().Clone()
		ctx := sentry.SetHubOnContext(c.Request.Context(), hub)
		c.Request = c.Request.WithContext(ctx)

		defer func() {
			if r := recover(); r != nil {
				stack := debug.Stack()
				log.Printf("PANIC RECOVERED: %v\n%s", r, stack)

				hub.WithScope(func(scope *sentry.Scope) {
					scope.SetLevel(sentry.LevelFatal)
					enrichScopeWithRequest(scope, c)
					scope.SetContext("panic", map[string]interface{}{
						"value":      fmt.Sprintf("%v", r),
						"stacktrace": string(stack),
					})
					hub.RecoverWithContext(ctx, r)
				})

				hub.Flush(2 * time.Second)
				c.JSON(500, map[string]interface{}{"error": "Internal server error"})
			}
		}()

		hub.ConfigureScope(func(scope *sentry.Scope) {
			enrichScopeWithRequest(scope, c)
			if c.Auth != nil {
				scope.SetUser(sentry.User{
					ID:       c.Auth.Id,
					Email:    c.Auth.GetString("email"),
					Username: c.Auth.GetString("name"),
				})
				scope.SetTag("user_id", c.Auth.Id)
			}
		})

		err := handler(c)
		if err != nil && !isBusinessError(err) {
			hub.WithScope(func(scope *sentry.Scope) {
				scope.SetLevel(sentry.LevelError)
				captureErr := err
				if c, ok := err.(causer); ok {
					captureErr = c.Cause()
				}
				scope.SetContext("error_details", map[string]interface{}{
					"message": captureErr.Error(),
					"type":    fmt.Sprintf("%T", captureErr),
				})
				hub.CaptureException(captureErr)
			})
		}

		return err
	}
}

// causer is implemented by serverError in the handlers package to expose the
// real underlying error through the generic HTTP wrapper.
type causer interface{ Cause() error }

// isBusinessError reports whether err is an expected 4xx client rejection.
// serverError wraps infra failures but is never itself a *router.ApiError,
// so this only matches ApiErrors returned directly by handlers.
func isBusinessError(err error) bool {
	apiErr, ok := err.(*router.ApiError)
	return ok && apiErr.Status < 500
}

func enrichScopeWithRequest(scope *sentry.Scope, c *core.RequestEvent) {
	req := c.Request
	scope.SetRequest(req)
	scope.SetTag("http_method", req.Method)
	scope.SetTag("http_path", req.URL.Path)
}

// WithJobRecovery wraps background job functions with panic recovery and error capture.
func WithJobRecovery(jobName string, fn func() error) {
	hub := sentry.CurrentHub().Clone()

	defer func() {
		if r := recover(); r != nil {
			stack := debug.Stack()
			log.Printf("PANIC in job '%s': %v\n%s", jobName, r, stack)

			hub.WithScope(func(scope *sentry.Scope) {
				scope.SetLevel(sentry.LevelFatal)
				scope.SetTag("job_name", jobName)
				scope.SetContext("panic", map[string]interface{}{
					"job":   jobName,
					"value": fmt.Sprintf("%v", r),
					"stack": string(stack),
				})
				hub.Recover(r)
			})

			hub.Flush(2 * time.Second)
		}
	}()

	hub.ConfigureScope(func(scope *sentry.Scope) {
		scope.SetTag("job_name", jobName)
	})

	if err := fn(); err != nil {
		log.Printf("Error in job '%s': %v", jobName, err)
		hub.WithScope(func(scope *sentry.Scope) {
			scope.SetLevel(sentry.LevelError)
			scope.SetContext("error", map[string]interface{}{
				"job":     jobName,
				"message": err.Error(),
			})
			hub.CaptureException(err)
		})
	}
}
