package handlers

import (
	"ganigeorgiev.com/pocketbase/dbx"
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"ganigeorgiev.com/pocketbase/pocketbase/tools/routine"
	"ganigeorgiev.com/pocketbase/pocketbase/tools/types"
)

// DefaultCompletionThreshold is the progress a territory needs to count as
// completed when the congregation has not chosen its own. Below 100 because
// territories rarely reach it: not-homes short of max tries, and homes that
// were missed or can't be reached.
const DefaultCompletionThreshold = 90

// A territory's coverage is tracked in rounds for the S-13 record. A round starts
// at round_started_at (creation, a territory reset, or a map reset after the
// previous round completed) and is completed once last_completed_at is set at or
// after that. Every start and completion is also written to territories_log,
// which the report reads; the two territory fields only hold the current round.
//
// round_started_at is empty for territories that predate this tracking. Their
// first reset starts a round; until then completion is judged on progress and
// activity alone.

func completionThreshold(congregation *core.Record) int {
	if threshold := congregation.GetInt("completion_threshold"); threshold > 0 {
		return threshold
	}
	return DefaultCompletionThreshold
}

func roundOpen(territory *core.Record) bool {
	completed := territory.GetDateTime("last_completed_at")
	if completed.IsZero() {
		return true
	}
	started := territory.GetDateTime("round_started_at")
	return !started.IsZero() && completed.Before(started)
}

// withTerritoryLock runs fn under the lock ProcessTerritoryAggregates holds, so a
// round can't start between a completion check reading the round and recording
// its result. fn may open a transaction, but the lock must not be taken inside
// one: the completion check opens its own while holding it.
func withTerritoryLock(app core.App, territoryID string, fn func() error) error {
	job := aggregateJobFor(app, "territory:"+territoryID)
	job.run.Lock()
	defer job.run.Unlock()
	return fn()
}

// checkTerritoryCompletion completes the territory's open round once its progress
// reaches the congregation's threshold. Callers hold the territory lock.
//
// Progress alone is not enough. Every map with countable addresses must have been
// reset since the round started (or created after), so a congregation resetting
// map by map does not complete on the maps it has not reset yet. And something
// must have been worked since the round started, so the stale progress left
// right after a reset, before the maps are recounted, can't complete it.
//
// The completion is dated by the last work in the round rather than now: when a
// settings change or an address removal is what crosses the threshold, the
// coverage itself finished earlier.
func checkTerritoryCompletion(app core.App, territory *core.Record, progress int) error {
	if !roundOpen(territory) {
		return nil
	}

	congregation, err := app.FindRecordById("congregations", territory.GetString("congregation"))
	if err != nil {
		return err
	}
	threshold := completionThreshold(congregation)
	if progress < threshold {
		return nil
	}

	// Dates are stored as sortable text, and an empty start compares below
	// every date, so territories without a round skip the freshness check.
	started := territory.GetString("round_started_at")
	params := dbx.Params{"territory": territory.Id, "started": started}

	if started != "" {
		var unreset int
		err := app.DB().NewQuery(`
			SELECT COUNT(*) FROM maps
			WHERE territory = {:territory}
			AND created < {:started}
			AND COALESCE(reset_at, '') < {:started}
			AND COALESCE(json_extract(NULLIF(aggregates, ''), '$.total'), 0) > 0
		`).Bind(params).Row(&unreset)
		if err != nil {
			return err
		}
		if unreset > 0 {
			return nil
		}
	}

	activity := struct {
		Last   string `db:"last"`
		Worked int    `db:"worked"`
	}{}
	err = app.DB().NewQuery(`
		SELECT
			COALESCE(MAX(created), '') AS last,
			COALESCE(SUM(CASE WHEN old_status = 'not_done' THEN 1 ELSE 0 END), 0) AS worked
		FROM addresses_log
		WHERE territory = {:territory}
		AND new_status IN ('done', 'not_home')
		AND created >= {:started}
	`).Bind(params).One(&activity)
	if err != nil {
		return err
	}
	if activity.Worked == 0 {
		return nil
	}
	completedAt, err := types.ParseDateTime(activity.Last)
	if err != nil {
		return err
	}

	return app.RunInTransaction(func(txApp core.App) error {
		current, err := txApp.FindRecordById("territories", territory.Id)
		if err != nil {
			return err
		}
		if !roundOpen(current) || current.GetString("round_started_at") != started {
			return nil
		}
		current.Set("last_completed_at", completedAt)
		if err := txApp.SaveNoValidate(current.IgnoreUnchangedFields(true)); err != nil {
			return err
		}
		return logTerritoryEvent(txApp, current, "completed", map[string]any{
			"threshold": threshold,
			"progress":  progress,
		}, "", completedAt)
	})
}

// startTerritoryRound records a territory reset: a new round starts now and every
// map counts as reset in it. Runs inside the reset's transaction, under the
// territory lock.
func startTerritoryRound(txApp core.App, territory *core.Record, actor string) error {
	now := types.NowDateTime()
	if _, err := txApp.DB().NewQuery(
		"UPDATE maps SET reset_at = {:now} WHERE territory = {:territory}",
	).Bind(dbx.Params{"now": now.String(), "territory": territory.Id}).Execute(); err != nil {
		return err
	}
	territory.Set("round_started_at", now)
	if err := txApp.SaveNoValidate(territory.IgnoreUnchangedFields(true)); err != nil {
		return err
	}
	return logTerritoryEvent(txApp, territory, "reset", nil, actor, now)
}

// recordMapReset marks one map as reset in the current round. When that round is
// already complete, or the territory has never had one, the reset is the start of
// the next round instead; maps nobody has worked yet need no reset of their own to
// count in it. Runs inside the reset's transaction, under the territory lock.
func recordMapReset(txApp core.App, territory *core.Record, mapID string, actor string) error {
	now := types.NowDateTime()
	newRound := !roundOpen(territory) || territory.GetDateTime("round_started_at").IsZero()

	query := "UPDATE maps SET reset_at = {:now} WHERE id = {:map}"
	if newRound {
		query = `UPDATE maps SET reset_at = {:now}
			WHERE id = {:map}
			OR (territory = {:territory}
				AND COALESCE(json_extract(NULLIF(aggregates, ''), '$.notDone'), 0) = COALESCE(json_extract(NULLIF(aggregates, ''), '$.total'), 0))`
	}
	if _, err := txApp.DB().NewQuery(query).Bind(dbx.Params{
		"now":       now.String(),
		"map":       mapID,
		"territory": territory.Id,
	}).Execute(); err != nil {
		return err
	}

	if newRound {
		territory.Set("round_started_at", now)
		if err := txApp.SaveNoValidate(territory.IgnoreUnchangedFields(true)); err != nil {
			return err
		}
	}
	return logTerritoryEvent(txApp, territory, "map_reset", map[string]any{
		"map":       mapID,
		"new_round": newRound,
	}, actor, now)
}

// logTerritoryEvent writes a territories_log row dated at. The code is copied so
// the S-13 report can still name a territory after it is deleted.
func logTerritoryEvent(app core.App, territory *core.Record, action string, details map[string]any, actor string, at types.DateTime) error {
	collection, err := app.FindCachedCollectionByNameOrId("territories_log")
	if err != nil {
		return err
	}
	record := core.NewRecord(collection)
	record.Set("congregation", territory.GetString("congregation"))
	record.Set("territory", territory.Id)
	record.Set("territory_code", territory.GetString("code"))
	record.Set("action", action)
	record.Set("details", details)
	record.Set("changed_by", actor)
	record.SetRaw("created", at)
	return app.SaveNoValidate(record)
}

// HandleCongregationSettingsChange rechecks the congregation's territories when a
// setting that decides completion changes. A max_tries change moves map progress,
// so the maps are recounted first; a threshold change only needs the check.
// It should be called from OnRecordAfterUpdateSuccess("congregations").
func HandleCongregationSettingsChange(e *core.RecordEvent) {
	original := e.Record.Original()
	triesChanged := original.GetInt("max_tries") != e.Record.GetInt("max_tries")
	if !triesChanged && completionThreshold(original) == completionThreshold(e.Record) {
		return
	}

	app := e.App
	congregationID := e.Record.Id
	routine.FireAndForget(func() {
		if triesChanged {
			var mapIDs []string
			if err := app.DB().NewQuery("SELECT id FROM maps WHERE congregation = {:id}").
				Bind(dbx.Params{"id": congregationID}).Column(&mapIDs); err != nil {
				app.Logger().Error("listing maps for recount failed", "congregation", congregationID, "err", err)
				return
			}
			for _, mapID := range mapIDs {
				if err := ProcessMapAggregates(mapID, app, false); err != nil {
					app.Logger().Error("map recount failed", "map", mapID, "err", err)
				}
			}
		}

		var territoryIDs []string
		if err := app.DB().NewQuery("SELECT id FROM territories WHERE congregation = {:id}").
			Bind(dbx.Params{"id": congregationID}).Column(&territoryIDs); err != nil {
			app.Logger().Error("listing territories for recheck failed", "congregation", congregationID, "err", err)
			return
		}
		for _, territoryID := range territoryIDs {
			if err := ProcessTerritoryAggregates(territoryID, app); err != nil {
				app.Logger().Error("territory recheck failed", "territory", territoryID, "err", err)
			}
		}
	})
}
