package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Returns the territories viewRule to a plain logged-in check; its congregation
// scope moved to the territories view and enrich hooks.
const (
	territoryViewRuleBasic  = `@request.auth.id != ""`
	territoryViewRuleJoined = `@request.auth.id != "" && congregation.roles_via_congregation.user ?= @request.auth.id`
)

func init() {
	m.Register(func(app core.App) error {
		return setTerritoriesViewRule(app, territoryViewRuleBasic)
	}, func(app core.App) error {
		return setTerritoriesViewRule(app, territoryViewRuleJoined)
	})
}

func setTerritoriesViewRule(app core.App, rule string) error {
	collection, err := app.FindCollectionByNameOrId("territories")
	if err != nil {
		return nil
	}
	if collection.ViewRule != nil && *collection.ViewRule == rule {
		return nil
	}
	collection.ViewRule = &rule
	return app.Save(collection)
}
