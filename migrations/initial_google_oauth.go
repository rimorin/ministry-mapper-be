package migrations

import (
	"os"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}

		googleClientId := os.Getenv("GOOGLE_CLIENT_ID")
		googleClientSecret := os.Getenv("GOOGLE_CLIENT_SECRET")

		if googleClientId != "" && googleClientSecret != "" {
			// Check if Google OAuth is already configured
			googleExists := false
			for _, provider := range collection.OAuth2.Providers {
				if provider.Name == "google" {
					googleExists = true
					break
				}
			}

			// Only add if it doesn't already exist
			if !googleExists {
				collection.OAuth2.Providers = append(collection.OAuth2.Providers, core.OAuth2ProviderConfig{
					Name:         "google",
					ClientId:     googleClientId,
					ClientSecret: googleClientSecret,
				})
			}
		}

		return app.Save(collection)
	}, func(app core.App) error {
		// add down queries...

		return nil
	})
}
