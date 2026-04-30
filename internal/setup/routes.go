package setup

import (
	"os"
	"strings"
	"time"

	"ministry-mapper/internal/handlers"
	"ministry-mapper/internal/jobs"
	"ministry-mapper/internal/middleware"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

func RegisterRoutes(app core.App) {
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

		// Custom endpoint: handles its own auth (supports link-id for publishers)
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

		return e.Next()
	})
}
