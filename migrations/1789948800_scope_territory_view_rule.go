package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Puts the territory congregation check in the viewRule, not just the
// OnRecordViewRequest hook: expandFetch applies the rule but fires no request
// hooks, so the hook alone left ?expand=map.territory able to reach another
// congregation's territories.
//
// The old rule only checked that the caller was logged in — its other clauses
// inspected the query string's shape, which the view endpoint ignores when
// fetching by id — so it is replaced rather than extended.
// roles.idx_PUEoaq44d4 (user, congregation, role) covers the new lookup.
const (
	territoryViewRuleScoped = `@request.auth.id != "" && congregation.roles_via_congregation.user ?= @request.auth.id`

	territoryViewRuleOriginal = `@request.auth.id != "" && @request.query.filter:isset = true && @request.query.filter ~ "user=" && @request.query.fields:isset = true`
)

func init() {
	m.Register(func(app core.App) error {
		return setTerritoryViewRule(app, territoryViewRuleScoped)
	}, func(app core.App) error {
		return setTerritoryViewRule(app, territoryViewRuleOriginal)
	})
}

func setTerritoryViewRule(app core.App, rule string) error {
	collection, err := app.FindCollectionByNameOrId("territories")
	if err != nil {
		return fmt.Errorf("find territories: %w", err)
	}

	collection.ViewRule = &rule

	return app.Save(collection)
}
