package handlers

import (
	"errors"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/search"
)

func toSet(ids []string) map[string]bool {
	set := make(map[string]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return set
}

// filterListResults drops already-fetched records that don't satisfy keep,
// patching e.Records and the already-built e.Result payload (including
// pagination counts) to match.
//
// OnRecordsListRequest fires after PocketBase has already run the query
// with the client's own filter, so a ListRule can't be overridden per
// request from inside this hook to change which rows come back. This is
// the only remaining point to enforce a scope computed from trusted,
// server-side data instead of the client's filter.
func filterListResults(e *core.RecordsListRequestEvent, keep func(*core.Record) bool) {
	filtered := make([]*core.Record, 0, len(e.Records))
	for _, r := range e.Records {
		if keep(r) {
			filtered = append(filtered, r)
		}
	}
	removed := len(e.Records) - len(filtered)
	if removed == 0 {
		return
	}

	e.Records = filtered
	if items, ok := e.Result.Items.(*[]*core.Record); ok {
		*items = filtered
	}

	// Accurate for a single-page result, an approximation across multiple.
	e.Result.TotalItems -= removed
	if e.Result.TotalItems < 0 {
		e.Result.TotalItems = 0
	}
	if e.Result.PerPage > 0 {
		e.Result.TotalPages = (e.Result.TotalItems + e.Result.PerPage - 1) / e.Result.PerPage
	}
}

// userCongregationIDs returns every congregation the user holds any role in.
func userCongregationIDs(app core.App, userId string) ([]string, error) {
	var rows []struct {
		Congregation string `db:"congregation"`
	}
	err := app.DB().NewQuery(`
		SELECT DISTINCT congregation FROM roles WHERE user = {:userId}
	`).Bind(dbx.Params{"userId": userId}).All(&rows)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.Congregation
	}
	return ids, nil
}

// userMapIDs returns every map in every congregation the user holds any role in.
func userMapIDs(app core.App, userId string) ([]string, error) {
	var rows []struct {
		Id string `db:"id"`
	}
	err := app.DB().NewQuery(`
		SELECT DISTINCT m.id FROM maps m
		JOIN roles r ON r.congregation = m.congregation
		WHERE r.user = {:userId}
	`).Bind(dbx.Params{"userId": userId}).All(&rows)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.Id
	}
	return ids, nil
}

// resolveMapScopeIDs returns the maps a request may access: the single map
// behind a valid, non-expired link-id, or every map in every congregation
// the auth user holds a role in.
func resolveMapScopeIDs(app core.App, auth *core.Record, linkId string) ([]string, error) {
	if linkId != "" {
		var result struct {
			Map string `db:"map"`
		}
		err := app.DB().NewQuery(`
			SELECT map FROM assignments
			WHERE id = {:linkId} AND expiry_date > datetime('now')
			LIMIT 1
		`).Bind(dbx.Params{"linkId": linkId}).One(&result)
		if err != nil || result.Map == "" {
			return nil, errors.New("unauthorized")
		}
		return []string{result.Map}, nil
	}

	if auth == nil {
		return nil, errors.New("auth required")
	}
	ids, err := userMapIDs(app, auth.Id)
	if err != nil || len(ids) == 0 {
		return nil, errors.New("unauthorized")
	}
	return ids, nil
}

func mapScopeKeep(app core.App, auth *core.Record, linkId string) (func(*core.Record) bool, error) {
	ids, err := resolveMapScopeIDs(app, auth, linkId)
	if err != nil {
		return nil, err
	}
	set := toSet(ids)
	return func(r *core.Record) bool { return set[r.GetString("map")] }, nil
}

// filterEscapesMapScope reports whether clientFilter could match a record
// outside allowedIds. It runs the client's filter through PocketBase's parser
// against the collection restricted to out-of-scope maps: any row returned
// means the filter reaches wider than the requester may see.
//
// Fails closed — any parse or query error reports true.
func filterEscapesMapScope(
	app core.App,
	collectionName string,
	clientFilter string,
	allowedIds []string,
) bool {
	if clientFilter == "" || len(allowedIds) == 0 {
		return true
	}

	collection, err := app.FindCachedCollectionByNameOrId(collectionName)
	if err != nil {
		return true
	}

	// nil requestInfo: a filter referencing @request.* fails to resolve and is
	// refused, which is correct — its match set can't be verified here.
	resolver := core.NewRecordFieldResolver(app, collection, nil, false)
	expr, err := search.FilterData(clientFilter).BuildExpr(resolver)
	if err != nil {
		return true
	}

	outOfScope := make([]any, len(allowedIds))
	for i, id := range allowedIds {
		outOfScope[i] = id
	}

	// Mirrors PocketBase's own subscription-filter check
	// (apis.realtimeCanAccessRecord), narrowed to out-of-scope maps.
	var exists int
	query := app.ConcurrentDB().Select("(1)").
		From(collection.Name).
		AndWhere(expr).
		AndWhere(dbx.NotIn(collection.Name+".map", outOfScope...))
	if err := resolver.UpdateQuery(query); err != nil {
		return true
	}

	return query.Limit(1).Row(&exists) == nil && exists > 0
}

// congregationScopeKeep returns a predicate matching every congregation the
// user holds any role in.
func congregationScopeKeep(app core.App, userId string) (func(*core.Record) bool, error) {
	ids, err := userCongregationIDs(app, userId)
	if err != nil || len(ids) == 0 {
		return nil, errors.New("unauthorized")
	}
	set := toSet(ids)
	return func(r *core.Record) bool { return set[r.GetString("congregation")] }, nil
}

// rolesScopeKeep matches the user's own role records, plus every role
// record in a congregation they hold any role in — any congregation member
// may list the full roster, not just admins.
func rolesScopeKeep(app core.App, userId string) (func(*core.Record) bool, error) {
	congIds, err := userCongregationIDs(app, userId)
	if err != nil {
		return nil, err
	}
	congSet := toSet(congIds)
	return func(r *core.Record) bool {
		return r.GetString("user") == userId || congSet[r.GetString("congregation")]
	}, nil
}

// assignmentsScopeKeep matches the user's own assignment history, every
// assignment for a map they hold a role for, and — for global
// administrators/conductors — the other user's assignments named in
// clientFilter, mirroring the "admin/conductor may look up any publisher's
// history" behavior.
func assignmentsScopeKeep(app core.App, userId string, clientFilter string) (func(*core.Record) bool, error) {
	mapIds, err := userMapIDs(app, userId)
	if err != nil {
		return nil, err
	}
	mapSet := toSet(mapIds)

	var extraUser string
	if requested := extractUserIdFromFilter(clientFilter); requested != "" && requested != userId {
		if hasRoleAnywhere(app, userId, "administrator", "conductor") {
			extraUser = requested
		}
	}

	return func(r *core.Record) bool {
		user := r.GetString("user")
		return user == userId || mapSet[r.GetString("map")] || (extraUser != "" && user == extraUser)
	}, nil
}
