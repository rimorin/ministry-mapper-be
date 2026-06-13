package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Backfills the completed/total keys in maps.aggregates (used by
// ProcessTerritoryAggregates to derive territory progress without scanning
// addresses) and recomputes map and territory progress from them.
// Maps with no countable addresses are left untouched — they contribute
// nothing to the territory sums, matching the previous address-scan behavior.
func init() {
	m.Register(func(app core.App) error {
		queries := []string{
			`UPDATE maps SET
				aggregates = json_set(
					COALESCE(NULLIF(aggregates, ''), '{}'),
					'$.completed', agg.completed,
					'$.total', agg.total
				),
				progress = CAST(ROUND(agg.completed * 100.0 / agg.total) AS INTEGER)
			FROM (
				SELECT
					a.map AS map_id,
					SUM(CASE WHEN a.status = 'done' OR (a.status = 'not_home' AND c.max_tries > 0 AND a.not_home_tries >= c.max_tries) THEN 1 ELSE 0 END) AS completed,
					COUNT(*) AS total
				FROM addresses a
				LEFT JOIN congregations c ON a.congregation = c.id
				WHERE EXISTS (
					SELECT 1
					FROM address_options ao
					JOIN options o ON ao.option = o.id
					WHERE ao.address = a.id
					AND ao.map = a.map
					AND o.is_countable = TRUE
				)
				AND a.status IN ('done', 'not_done', 'not_home')
				GROUP BY a.map
			) AS agg
			WHERE maps.id = agg.map_id`,
			`UPDATE territories SET progress = COALESCE((
				SELECT CASE
					WHEN SUM(json_extract(COALESCE(NULLIF(m.aggregates, ''), '{}'), '$.total')) > 0
					THEN CAST(ROUND(SUM(json_extract(COALESCE(NULLIF(m.aggregates, ''), '{}'), '$.completed')) * 100.0 / SUM(json_extract(COALESCE(NULLIF(m.aggregates, ''), '{}'), '$.total'))) AS INTEGER)
					ELSE 0
				END
				FROM maps m
				WHERE m.territory = territories.id
			), 0)`,
		}

		for _, query := range queries {
			if _, err := app.DB().NewQuery(query).Execute(); err != nil {
				return err
			}
		}

		return nil
	}, func(app core.App) error {
		_, err := app.DB().NewQuery(
			`UPDATE maps SET aggregates = json_remove(aggregates, '$.completed', '$.total') WHERE aggregates != ''`,
		).Execute()
		return err
	})
}
