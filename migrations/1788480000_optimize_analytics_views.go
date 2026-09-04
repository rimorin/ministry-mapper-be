package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// The report summary filters these views by congregation, but their original
// definitions forced SQLite to materialise the whole view before applying that
// filter: ROW_NUMBER() OVER() blocks WHERE-clause push-down entirely, and a
// GROUP BY that omits `congregation` prevents pushing the filter into the
// aggregate. Against a 1.1M-address database each query took 0.6-0.9s; with
// these definitions plus an addresses_log index they take 0.01-0.15s.
//
// Semantics are unchanged except for the `id` column of the two views that used
// ROW_NUMBER(): those ids were never stable between queries, and they are now a
// deterministic key (address id for analytics_not_home, a composite of the
// grouping columns for analytics_daily_status). PocketBase's view parser only
// accepts plain identifiers or parenthesised expressions as columns, hence the
// parentheses around the composite id.
type analyticsViewChange struct {
	collection string
	before     string
	after      string
}

var analyticsViewChanges = []analyticsViewChange{
	{
		collection: "analytics_daily_status",
		before: `SELECT
     (ROW_NUMBER() OVER()) AS id,
     strftime('%Y-%m-%d', created) AS day,
     congregation,
     territory,
     new_status,
     COUNT(*) AS change_count
   FROM addresses_log
   GROUP BY day, congregation, territory, new_status`,
		after: `SELECT
     (strftime('%Y-%m-%d', created) || '_' || COALESCE(congregation, '') || '_' || COALESCE(territory, '') || '_' || COALESCE(new_status, '')) AS id,
     strftime('%Y-%m-%d', created) AS day,
     congregation,
     territory,
     new_status,
     COUNT(*) AS change_count
   FROM addresses_log
   GROUP BY day, congregation, territory, new_status`,
	},
	{
		collection: "analytics_territories",
		before: `SELECT
     t.id,
     t.code,
     t.description,
     t.congregation,
     t.progress,
     c.name AS congregation_name,
     COUNT(a.id) AS total_addresses,
     SUM(CASE WHEN a.status = 'done' THEN 1 ELSE 0 END) AS done,
     SUM(CASE WHEN a.status = 'not_done' THEN 1 ELSE 0 END) AS not_done,
     SUM(CASE WHEN a.status = 'not_home' THEN 1 ELSE 0 END) AS not_home,
     SUM(CASE WHEN a.status = 'do_not_call' THEN 1 ELSE 0 END) AS dnc,
     SUM(CASE WHEN a.status = 'invalid' THEN 1 ELSE 0 END) AS invalid
   FROM territories t
   LEFT JOIN addresses a ON a.territory = t.id
   LEFT JOIN congregations c ON t.congregation = c.id
   GROUP BY t.id;`,
		after: `SELECT
     t.id,
     t.code,
     t.description,
     t.congregation,
     t.progress,
     c.name AS congregation_name,
     COUNT(a.id) AS total_addresses,
     SUM(CASE WHEN a.status = 'done' THEN 1 ELSE 0 END) AS done,
     SUM(CASE WHEN a.status = 'not_done' THEN 1 ELSE 0 END) AS not_done,
     SUM(CASE WHEN a.status = 'not_home' THEN 1 ELSE 0 END) AS not_home,
     SUM(CASE WHEN a.status = 'do_not_call' THEN 1 ELSE 0 END) AS dnc,
     SUM(CASE WHEN a.status = 'invalid' THEN 1 ELSE 0 END) AS invalid
   FROM territories t
   LEFT JOIN addresses a ON a.territory = t.id
   LEFT JOIN congregations c ON t.congregation = c.id
   GROUP BY t.id, t.congregation;`,
	},
	{
		collection: "analytics_not_home",
		before: `SELECT
     (ROW_NUMBER() OVER()) AS id,
     a.congregation,
     a.territory,
     a.map,
     a.not_home_tries,
     c.max_tries,
     IIF(a.not_home_tries >= c.max_tries, 'maxed_out', 'retrying') AS retry_status,
     a.updated
   FROM addresses a
   JOIN congregations c ON a.congregation = c.id
   WHERE a.status = 'not_home'`,
		after: `SELECT
     a.id,
     a.congregation,
     a.territory,
     a.map,
     a.not_home_tries,
     c.max_tries,
     IIF(a.not_home_tries >= c.max_tries, 'maxed_out', 'retrying') AS retry_status,
     a.updated
   FROM addresses a
   JOIN congregations c ON a.congregation = c.id
   WHERE a.status = 'not_home'`,
	},
}

const addressesLogCongregationIndex = "idx_addresses_log_congregation_created"

func init() {
	m.Register(func(app core.App) error {
		for _, change := range analyticsViewChanges {
			if err := setViewQuery(app, change.collection, change.after); err != nil {
				return err
			}
		}

		logs, err := app.FindCollectionByNameOrId("addresses_log")
		if err != nil {
			return nil
		}
		if logs.GetIndex(addressesLogCongregationIndex) != "" {
			return nil
		}
		logs.AddIndex(addressesLogCongregationIndex, false, "`congregation`, `created`", "")
		return app.Save(logs)
	}, func(app core.App) error {
		for _, change := range analyticsViewChanges {
			if err := setViewQuery(app, change.collection, change.before); err != nil {
				return err
			}
		}

		logs, err := app.FindCollectionByNameOrId("addresses_log")
		if err != nil {
			return nil
		}
		if logs.GetIndex(addressesLogCongregationIndex) == "" {
			return nil
		}
		logs.RemoveIndex(addressesLogCongregationIndex)
		return app.Save(logs)
	})
}

// setViewQuery replaces a view collection's query, skipping collections that do
// not exist or already have the wanted query so the migration is idempotent.
func setViewQuery(app core.App, name, query string) error {
	collection, err := app.FindCollectionByNameOrId(name)
	if err != nil {
		return nil
	}
	if collection.ViewQuery == query {
		return nil
	}
	collection.ViewQuery = query
	return app.Save(collection)
}
