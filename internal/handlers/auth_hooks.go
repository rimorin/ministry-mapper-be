package handlers

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// authOrLink authorizes via link-id (if present) or role. Link-id takes precedence.
// For updates/deletes, use Original() values to prevent field-mutation bypass.
func authOrLink(e *core.RecordRequestEvent, app core.App, useOriginal bool) error {
	if e.HasSuperuserAuth() {
		return e.Next()
	}

	rec := e.Record
	if useOriginal && rec.Original() != nil {
		rec = rec.Original()
	}
	mapId := rec.GetString("map")

	linkId := e.Request.Header.Get("link-id")
	if linkId != "" {
		if mapId != "" && AuthorizeLinkAccess(app, linkId, mapId) {
			return e.Next()
		}
		return apis.NewForbiddenError("Unauthorized", nil)
	}

	congId := rec.GetString("congregation")
	if e.Auth != nil && congId != "" && AuthorizeByRole(app, e.Auth.Id, congId) {
		return e.Next()
	}

	return apis.NewForbiddenError("Unauthorized", nil)
}

// adminOnly authorizes if the user is an administrator for the congregation.
func adminOnly(e *core.RecordRequestEvent, app core.App, congId string) error {
	if e.HasSuperuserAuth() {
		return e.Next()
	}
	if e.Auth == nil {
		return apis.NewForbiddenError("Auth required", nil)
	}
	if !AuthorizeByRole(app, e.Auth.Id, congId, "administrator") {
		return apis.NewForbiddenError("Administrator access required", nil)
	}
	return e.Next()
}

// adminOrConductor authorizes if the user is an administrator or conductor for the congregation.
func adminOrConductor(e *core.RecordRequestEvent, app core.App, congId string) error {
	if e.HasSuperuserAuth() {
		return e.Next()
	}
	if e.Auth == nil {
		return apis.NewForbiddenError("Auth required", nil)
	}
	if !AuthorizeByRole(app, e.Auth.Id, congId, "administrator", "conductor") {
		return apis.NewForbiddenError("Administrator or conductor access required", nil)
	}
	return e.Next()
}

// getCongId returns the congregation ID from the record, using Original() for updates/deletes.
func getCongId(e *core.RecordRequestEvent, useOriginal bool) string {
	if useOriginal && e.Record.Original() != nil {
		return e.Record.Original().GetString("congregation")
	}
	return e.Record.GetString("congregation")
}

var mapIdPattern = regexp.MustCompile(`map\s*=\s*['"]([^'"]+)['"]`)
var congIdPattern = regexp.MustCompile(`congregation\s*=\s*['"]([^'"]+)['"]`)
var territoryIdPattern = regexp.MustCompile(`territory\s*=\s*['"]([^'"]+)['"]`)
var userIdPattern = regexp.MustCompile(`user\s*=\s*['"]([^'"]+)['"]`)

func extractAllMapIdsFromFilter(filter string) []string {
	matches := mapIdPattern.FindAllStringSubmatch(filter, -1)
	ids := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) >= 2 {
			ids = append(ids, m[1])
		}
	}
	return ids
}

func extractAllCongIdsFromFilter(filter string) []string {
	matches := congIdPattern.FindAllStringSubmatch(filter, -1)
	ids := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) >= 2 {
			ids = append(ids, m[1])
		}
	}
	return ids
}

func extractAllTerritoryIdsFromFilter(filter string) []string {
	matches := territoryIdPattern.FindAllStringSubmatch(filter, -1)
	ids := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) >= 2 {
			ids = append(ids, m[1])
		}
	}
	return ids
}

func extractUserIdFromFilter(filter string) string {
	matches := userIdPattern.FindStringSubmatch(filter)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

func uniqueStrings(ss []string) []string {
	seen := make(map[string]struct{}, len(ss))
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	return out
}

func getTerritoryCongregation(app core.App, territoryId string) string {
	var result struct {
		Congregation string `db:"congregation"`
	}
	err := app.DB().NewQuery(`
		SELECT congregation FROM territories WHERE id = {:id}
	`).Bind(dbx.Params{"id": territoryId}).One(&result)
	if err != nil {
		return ""
	}
	return result.Congregation
}

// authorizeMapSubscription checks if a user/link can subscribe to map-scoped collections.
// If link-id is present it takes precedence and must be valid; otherwise role check is used.
// All map IDs present in the filter must be authorized.
func authorizeMapSubscription(app core.App, auth *core.Record, linkId string, filter string) bool {
	mapIds := uniqueStrings(extractAllMapIdsFromFilter(filter))
	if len(mapIds) == 0 {
		return false
	}
	if linkId != "" {
		// A link-id is scoped to exactly one map; reject multi-map filters.
		return len(mapIds) == 1 && AuthorizeLinkAccess(app, linkId, mapIds[0])
	}
	return auth != nil && authorizeUserForMaps(app, auth.Id, mapIds)
}

// isAdminAnywhere checks if the user is an administrator in any congregation.
func isAdminAnywhere(app core.App, userId string) bool {
	var v struct{ V int `db:"v"` }
	err := app.DB().NewQuery(`
		SELECT 1 as v FROM roles
		WHERE user = {:userId} AND role = 'administrator'
		LIMIT 1
	`).Bind(dbx.Params{"userId": userId}).One(&v)
	return err == nil
}

// isAdminOrConductorAnywhere checks if the user is an administrator or conductor in any congregation.
func isAdminOrConductorAnywhere(app core.App, userId string) bool {
	var v struct{ V int `db:"v"` }
	err := app.DB().NewQuery(`
		SELECT 1 as v FROM roles
		WHERE user = {:userId} AND role IN ('administrator', 'conductor')
		LIMIT 1
	`).Bind(dbx.Params{"userId": userId}).One(&v)
	return err == nil
}

var protectedCollections = map[string]bool{
	"messages":        true,
	"addresses":       true,
	"address_options": true,
}

// authorizeList validates access for a LIST request.
// If link-id is present it takes precedence and must be valid; otherwise role check is used.
func authorizeList(e *core.RecordsListRequestEvent, authCheck func() bool, linkCheck func(linkId string) bool) error {
	if e.HasSuperuserAuth() {
		return e.Next()
	}
	linkId := e.Request.Header.Get("link-id")
	if linkId != "" {
		if linkCheck(linkId) {
			return e.Next()
		}
		return apis.NewForbiddenError("Unauthorized", nil)
	}
	if e.Auth != nil && authCheck() {
		return e.Next()
	}
	return apis.NewForbiddenError("Unauthorized", nil)
}

// authorizeView validates access for a VIEW request.
// If link-id is present it takes precedence and must be valid; otherwise role check is used.
func authorizeView(e *core.RecordRequestEvent, authCheck func() bool, linkCheck func(linkId string) bool) error {
	if e.HasSuperuserAuth() {
		return e.Next()
	}
	linkId := e.Request.Header.Get("link-id")
	if linkId != "" {
		if linkCheck(linkId) {
			return e.Next()
		}
		return apis.NewForbiddenError("Unauthorized", nil)
	}
	if e.Auth != nil && authCheck() {
		return e.Next()
	}
	return apis.NewForbiddenError("Auth required", nil)
}

// linkMapListAuth validates map access for LIST requests.
// If link-id is present it takes precedence and must be valid; otherwise role check is used.
// All map IDs present in the filter must be authorized.
func linkMapListAuth(e *core.RecordsListRequestEvent, app core.App) error {
	mapIds := uniqueStrings(extractAllMapIdsFromFilter(e.Request.URL.Query().Get("filter")))
	return authorizeList(e,
		func() bool { return len(mapIds) > 0 && authorizeUserForMaps(app, e.Auth.Id, mapIds) },
		// A link-id is scoped to exactly one map; reject multi-map filters.
		func(linkId string) bool { return len(mapIds) == 1 && AuthorizeLinkAccess(app, linkId, mapIds[0]) },
	)
}

// RegisterAuthHooks registers authorization hooks for all create/update/delete operations
// and list/view hooks that replace @collection joins with efficient indexed queries.
func RegisterAuthHooks(app core.App) {
	// --- List/View hooks (post-query authorization) ---

	// Realtime subscribe: validate link-id authorization at subscribe time.
	// PB's built-in filter check (realtimeCanAccessRecord) handles per-event scoping.
	app.OnRealtimeSubscribeRequest().BindFunc(func(e *core.RealtimeSubscribeRequestEvent) error {
		if e.HasSuperuserAuth() {
			return e.Next()
		}

		filtered := make([]string, 0, len(e.Subscriptions))
		for _, sub := range e.Subscriptions {
			collName := sub
			if idx := strings.IndexByte(sub, '?'); idx >= 0 {
				collName = sub[:idx]
			}
			collName = strings.SplitN(collName, "/", 2)[0]

			if !protectedCollections[collName] {
				filtered = append(filtered, sub)
				continue
			}

			var filter, linkId string
			u, err := url.Parse(sub)
			if err == nil {
				raw := u.Query().Get("options")
				if raw != "" {
					type subOpts struct {
						Query   map[string]any `json:"query"`
						Headers map[string]any `json:"headers"`
					}
					var opts subOpts
					if jsonErr := json.Unmarshal([]byte(raw), &opts); jsonErr == nil {
						if f, ok := opts.Query["filter"]; ok {
							filter, _ = f.(string)
						}
						if h, ok := opts.Headers["link-id"]; ok {
							linkId, _ = h.(string)
						}
						if h, ok := opts.Headers["link_id"]; ok {
							linkId, _ = h.(string)
						}
					}
				}
			}

			if authorizeMapSubscription(app, e.Auth, linkId, filter) {
				filtered = append(filtered, sub)
			}
		}

		e.Subscriptions = filtered
		return e.Next()
	})

	// messages LIST: auth user needs role in congregation, link-id needs map assignment.
	app.OnRecordsListRequest("messages").BindFunc(func(e *core.RecordsListRequestEvent) error {
		return linkMapListAuth(e, app)
	})

	// addresses LIST + address_options LIST: validate map access.
	app.OnRecordsListRequest("addresses").BindFunc(func(e *core.RecordsListRequestEvent) error {
		return linkMapListAuth(e, app)
	})
	app.OnRecordsListRequest("address_options").BindFunc(func(e *core.RecordsListRequestEvent) error {
		return linkMapListAuth(e, app)
	})

	// maps LIST: auth user must have role in every congregation referenced by the filter.
	app.OnRecordsListRequest("maps").BindFunc(func(e *core.RecordsListRequestEvent) error {
		if e.HasSuperuserAuth() {
			return e.Next()
		}
		if e.Auth == nil {
			return apis.NewForbiddenError("Auth required", nil)
		}
		filter := e.Request.URL.Query().Get("filter")
		congIds := uniqueStrings(extractAllCongIdsFromFilter(filter))
		for _, territoryId := range uniqueStrings(extractAllTerritoryIdsFromFilter(filter)) {
			if cid := getTerritoryCongregation(app, territoryId); cid != "" {
				congIds = append(congIds, cid)
			}
		}
		congIds = uniqueStrings(congIds)
		if len(congIds) == 0 {
			return apis.NewForbiddenError("Missing congregation or territory filter", nil)
		}
		for _, congId := range congIds {
			if !AuthorizeByRole(app, e.Auth.Id, congId) {
				return apis.NewForbiddenError("Unauthorized", nil)
			}
		}
		return e.Next()
	})

	// territories LIST: auth user must have role in every congregation referenced by the filter.
	app.OnRecordsListRequest("territories").BindFunc(func(e *core.RecordsListRequestEvent) error {
		if e.HasSuperuserAuth() {
			return e.Next()
		}
		if e.Auth == nil {
			return apis.NewForbiddenError("Auth required", nil)
		}
		filter := e.Request.URL.Query().Get("filter")
		congIds := uniqueStrings(extractAllCongIdsFromFilter(filter))
		if len(congIds) == 0 {
			return apis.NewForbiddenError("Missing congregation filter", nil)
		}
		for _, congId := range congIds {
			if !AuthorizeByRole(app, e.Auth.Id, congId) {
				return apis.NewForbiddenError("Unauthorized", nil)
			}
		}
		return e.Next()
	})

	// address_options VIEW: validate role or link-id for the record's map.
	app.OnRecordViewRequest("address_options").BindFunc(func(e *core.RecordRequestEvent) error {
		mapId := e.Record.GetString("map")
		return authorizeView(e,
			func() bool {
				return authorizeUserForMap(app, e.Auth.Id, mapId)
			},
			func(linkId string) bool {
				return mapId != "" && AuthorizeLinkAccess(app, linkId, mapId)
			},
		)
	})

	// maps VIEW: validate role or link-id for this map.
	app.OnRecordViewRequest("maps").BindFunc(func(e *core.RecordRequestEvent) error {
		return authorizeView(e,
			func() bool {
				congId := e.Record.GetString("congregation")
				return congId != "" && AuthorizeByRole(app, e.Auth.Id, congId)
			},
			func(linkId string) bool {
				return AuthorizeLinkAccess(app, linkId, e.Record.Id)
			},
		)
	})

	// users LIST: only administrators can list users.
	app.OnRecordsListRequest("users").BindFunc(func(e *core.RecordsListRequestEvent) error {
		if e.HasSuperuserAuth() {
			return e.Next()
		}
		if e.Auth == nil {
			return apis.NewForbiddenError("Auth required", nil)
		}
		if !isAdminAnywhere(app, e.Auth.Id) {
			return apis.NewForbiddenError("Administrator access required", nil)
		}
		return e.Next()
	})

	// congregations VIEW: validate role or link-id for the congregation.
	app.OnRecordViewRequest("congregations").BindFunc(func(e *core.RecordRequestEvent) error {
		return authorizeView(e,
			func() bool {
				return AuthorizeByRole(app, e.Auth.Id, e.Record.Id)
			},
			func(linkId string) bool {
				return AuthorizeLinkForCongregation(app, linkId, e.Record.Id)
			},
		)
	})

	// options LIST: validate auth user has role in every congregation in the filter,
	// or link-id belongs to the single congregation queried.
	app.OnRecordsListRequest("options").BindFunc(func(e *core.RecordsListRequestEvent) error {
		filter := e.Request.URL.Query().Get("filter")
		congIds := uniqueStrings(extractAllCongIdsFromFilter(filter))
		return authorizeList(e,
			func() bool {
				if len(congIds) == 0 {
					return false
				}
				for _, congId := range congIds {
					if !AuthorizeByRole(app, e.Auth.Id, congId) {
						return false
					}
				}
				return true
			},
			// A link-id is congregation-scoped; reject multi-congregation filters.
			func(linkId string) bool {
				return len(congIds) == 1 && AuthorizeLinkForCongregation(app, linkId, congIds[0])
			},
		)
	})

	// options VIEW: validate role or link-id for the option's congregation.
	app.OnRecordViewRequest("options").BindFunc(func(e *core.RecordRequestEvent) error {
		congId := e.Record.GetString("congregation")
		return authorizeView(e,
			func() bool {
				return congId != "" && AuthorizeByRole(app, e.Auth.Id, congId)
			},
			func(linkId string) bool {
				return congId != "" && AuthorizeLinkForCongregation(app, linkId, congId)
			},
		)
	})

	// roles LIST: auth user must have a role in every congregation referenced by the filter.
	app.OnRecordsListRequest("roles").BindFunc(func(e *core.RecordsListRequestEvent) error {
		if e.HasSuperuserAuth() {
			return e.Next()
		}
		if e.Auth == nil {
			return apis.NewForbiddenError("Auth required", nil)
		}
		filter := e.Request.URL.Query().Get("filter")
		congIds := uniqueStrings(extractAllCongIdsFromFilter(filter))
		if len(congIds) > 0 {
			for _, congId := range congIds {
				if !AuthorizeByRole(app, e.Auth.Id, congId) {
					return apis.NewForbiddenError("Unauthorized", nil)
				}
			}
			return e.Next()
		}
		// user= filter only: allow if querying own roles
		userId := extractUserIdFromFilter(filter)
		if userId == e.Auth.Id {
			return e.Next()
		}
		return apis.NewForbiddenError("Unauthorized", nil)
	})

	// users VIEW: only administrators can view other users.
	app.OnRecordViewRequest("users").BindFunc(func(e *core.RecordRequestEvent) error {
		if e.HasSuperuserAuth() {
			return e.Next()
		}
		if e.Auth == nil {
			return apis.NewForbiddenError("Auth required", nil)
		}
		if e.Auth.Id == e.Record.Id {
			return e.Next()
		}
		if !isAdminAnywhere(app, e.Auth.Id) {
			return apis.NewForbiddenError("Administrator access required", nil)
		}
		return e.Next()
	})

	// assignments LIST: auth user must have role in the map's congregation or be querying own assignments.
	app.OnRecordsListRequest("assignments").BindFunc(func(e *core.RecordsListRequestEvent) error {
		if e.HasSuperuserAuth() {
			return e.Next()
		}
		if e.Auth == nil {
			return apis.NewForbiddenError("Auth required", nil)
		}
		filter := e.Request.URL.Query().Get("filter")
		mapIds := extractAllMapIdsFromFilter(filter)
		if len(mapIds) > 0 {
			if !authorizeUserForMaps(app, e.Auth.Id, mapIds) {
				return apis.NewForbiddenError("Unauthorized", nil)
			}
			return e.Next()
		}
		// user= filter: allow self-query or admin/conductor
		userId := extractUserIdFromFilter(filter)
		if userId == e.Auth.Id {
			return e.Next()
		}
		if !isAdminOrConductorAnywhere(app, e.Auth.Id) {
			return apis.NewForbiddenError("Unauthorized", nil)
		}
		return e.Next()
	})

	// assignments VIEW: link-id takes precedence when present; otherwise role check.
	app.OnRecordViewRequest("assignments").BindFunc(func(e *core.RecordRequestEvent) error {
		congId := e.Record.GetString("congregation")
		return authorizeView(e,
			func() bool { return congId != "" && AuthorizeByRole(app, e.Auth.Id, congId) },
			func(linkId string) bool { return linkId == e.Record.Id },
		)
	})

	// --- Create/Update/Delete hooks (pre-operation authorization) ---

	// Pattern A: Any role OR link access
	// addresses create/update
	app.OnRecordCreateRequest("addresses").BindFunc(func(e *core.RecordRequestEvent) error {
		return authOrLink(e, app, false)
	})
	app.OnRecordUpdateRequest("addresses").BindFunc(func(e *core.RecordRequestEvent) error {
		return authOrLink(e, app, true)
	})

	// address_options create/delete
	app.OnRecordCreateRequest("address_options").BindFunc(func(e *core.RecordRequestEvent) error {
		return authOrLink(e, app, false)
	})
	app.OnRecordDeleteRequest("address_options").BindFunc(func(e *core.RecordRequestEvent) error {
		return authOrLink(e, app, true)
	})

	// messages create
	app.OnRecordCreateRequest("messages").BindFunc(func(e *core.RecordRequestEvent) error {
		return authOrLink(e, app, false)
	})

	// Pattern B: Administrator only
	// maps update/delete
	app.OnRecordUpdateRequest("maps").BindFunc(func(e *core.RecordRequestEvent) error {
		return adminOnly(e, app, getCongId(e, true))
	})
	app.OnRecordDeleteRequest("maps").BindFunc(func(e *core.RecordRequestEvent) error {
		return adminOnly(e, app, getCongId(e, true))
	})

	// messages update/delete
	app.OnRecordUpdateRequest("messages").BindFunc(func(e *core.RecordRequestEvent) error {
		return adminOnly(e, app, getCongId(e, true))
	})
	app.OnRecordDeleteRequest("messages").BindFunc(func(e *core.RecordRequestEvent) error {
		return adminOnly(e, app, getCongId(e, true))
	})

	// roles create/update/delete
	app.OnRecordCreateRequest("roles").BindFunc(func(e *core.RecordRequestEvent) error {
		return adminOnly(e, app, getCongId(e, false))
	})
	app.OnRecordUpdateRequest("roles").BindFunc(func(e *core.RecordRequestEvent) error {
		return adminOnly(e, app, getCongId(e, true))
	})
	app.OnRecordDeleteRequest("roles").BindFunc(func(e *core.RecordRequestEvent) error {
		return adminOnly(e, app, getCongId(e, true))
	})

	// territories create/update/delete
	app.OnRecordCreateRequest("territories").BindFunc(func(e *core.RecordRequestEvent) error {
		return adminOnly(e, app, getCongId(e, false))
	})
	app.OnRecordUpdateRequest("territories").BindFunc(func(e *core.RecordRequestEvent) error {
		return adminOnly(e, app, getCongId(e, true))
	})
	app.OnRecordDeleteRequest("territories").BindFunc(func(e *core.RecordRequestEvent) error {
		return adminOnly(e, app, getCongId(e, true))
	})

	// congregations update — congregation ID is the record ID itself
	app.OnRecordUpdateRequest("congregations").BindFunc(func(e *core.RecordRequestEvent) error {
		return adminOnly(e, app, e.Record.Id)
	})

	// Pattern C: Administrator or conductor
	// assignments create/delete
	app.OnRecordCreateRequest("assignments").BindFunc(func(e *core.RecordRequestEvent) error {
		return adminOrConductor(e, app, getCongId(e, false))
	})
	app.OnRecordDeleteRequest("assignments").BindFunc(func(e *core.RecordRequestEvent) error {
		return adminOrConductor(e, app, getCongId(e, true))
	})
}
