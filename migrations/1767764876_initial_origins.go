package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		// add up queries...

		collection, err := app.FindCollectionByNameOrId("congregations")
		if err != nil {
			return err
		}

		// Check if fields already exist (idempotent)
		if collection.Fields.GetByName("origin") != nil {
			collection.Fields.Add(&core.SelectField{
				Name:      "origin",
				MaxSelect: 1,
				Values: []string{
					"us",
					"cn",
					"in",
					"mx",
					"eg",
					"sa",
					"bd",
					"br",
					"id",
					"jp",
					"kr",
					"sg",
					"my",
				},
			})
		}

		if collection.Fields.GetByName("timezone") != nil {
			collection.Fields.Add(&core.SelectField{
				Name:      "timezone",
				MaxSelect: 1,
				Values: []string{
					"America/New_York",
					"America/Chicago",
					"America/Denver",
					"America/Los_Angeles",
					"America/Mexico_City",
					"America/Sao_Paulo",
					"Asia/Shanghai",
					"Asia/Kolkata",
					"Asia/Dhaka",
					"Asia/Jakarta",
					"Asia/Tokyo",
					"Asia/Seoul",
					"Asia/Singapore",
					"Asia/Kuala_Lumpur",
					"Asia/Riyadh",
					"Asia/Dubai",
					"Africa/Cairo",
					"Africa/Johannesburg",
					"Australia/Sydney",
					"Pacific/Auckland",
				},
			})
		}

		return app.Save(collection)
	}, func(app core.App) error {
		// add down queries...

		return nil
	})
}
