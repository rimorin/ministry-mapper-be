package handlers

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// authOrLink authorizes if the user has any role in the congregation OR has valid link access.
// For updates/deletes, use Original() values to prevent field-mutation bypass.
func authOrLink(e *core.RecordRequestEvent, app *pocketbase.PocketBase, useOriginal bool) error {
	if e.HasSuperuserAuth() {
		return e.Next()
	}

	rec := e.Record
	if useOriginal && rec.Original() != nil {
		rec = rec.Original()
	}
	congId := rec.GetString("congregation")
	mapId := rec.GetString("map")

	// Branch 1: authenticated user with role in congregation
	if e.Auth != nil && congId != "" && AuthorizeByRole(app, e.Auth.Id, congId) {
		return e.Next()
	}

	// Branch 2: valid link-id with assignment for this map
	linkId := e.Request.Header.Get("link-id")
	if linkId != "" && mapId != "" && AuthorizeLinkAccess(app, linkId, mapId) {
		return e.Next()
	}

	return apis.NewForbiddenError("Unauthorized", nil)
}

// adminOnly authorizes if the user is an administrator for the congregation.
func adminOnly(e *core.RecordRequestEvent, app *pocketbase.PocketBase, congId string) error {
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
func adminOrConductor(e *core.RecordRequestEvent, app *pocketbase.PocketBase, congId string) error {
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

// linkOnly checks if the request is using link-id auth (not a logged-in user).
func linkOnly(e *core.RecordsListRequestEvent) (string, bool) {
	if e.Auth != nil {
		return "", false
	}
	linkId := e.Request.Header.Get("link-id")
	return linkId, linkId != ""
}

var mapIdPattern = regexp.MustCompile(`map\s*=\s*"([^"]+)"`)

func extractMapIdFromFilter(filter string) string {
	matches := mapIdPattern.FindStringSubmatch(filter)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

func getMapCongregation(app *pocketbase.PocketBase, mapId string) string {
	var result struct {
		Congregation string `db:"congregation"`
	}
	err := app.DB().NewQuery(`
		SELECT congregation FROM maps WHERE id = {:mapId}
	`).Bind(dbx.Params{"mapId": mapId}).One(&result)
	if err != nil {
		return ""
	}
	return result.Congregation
}

// authorizeMessageSubscription checks if a user/link can subscribe to messages for a map.
func authorizeMessageSubscription(app *pocketbase.PocketBase, auth *core.Record, linkId string, filter string) bool {
	if strings.Contains(filter, "||") {
		return false
	}
	mapId := extractMapIdFromFilter(filter)
	if mapId == "" {
		return false
	}

	// Auth user: check role in the map's congregation
	if auth != nil {
		congId := getMapCongregation(app, mapId)
		return congId != "" && AuthorizeByRole(app, auth.Id, congId)
	}

	// Link-id: check assignment for this map
	if linkId != "" {
		return AuthorizeLinkAccess(app, linkId, mapId)
	}

	return false
}

// authorizeLinkSubscription validates link-id subscriptions filtered by map.
// Rejects OR conditions to prevent multi-map bypass.
func authorizeLinkSubscription(app *pocketbase.PocketBase, auth *core.Record, linkId string, filter string) bool {
	if auth != nil {
		return true
	}
	if linkId == "" {
		return false
	}
	// Reject OR conditions to prevent subscribing to multiple maps
	if strings.Contains(filter, "||") {
		return false
	}
	mapId := extractMapIdFromFilter(filter)
	return mapId != "" && AuthorizeLinkAccess(app, linkId, mapId)
}

func extractMapIdFromRequest(r *core.RequestEvent) string {
	filter := r.Request.URL.Query().Get("filter")
	if strings.Contains(filter, "||") {
		return ""
	}
	return extractMapIdFromFilter(filter)
}

var protectedCollections = map[string]bool{
	"messages":        true,
	"addresses":       true,
	"address_options": true,
}

// linkMapListAuth validates link-id has a valid assignment for the map in the request filter.
// Auth users and superusers pass through without checks.
func linkMapListAuth(e *core.RecordsListRequestEvent, app *pocketbase.PocketBase) error {
	if e.HasSuperuserAuth() || e.Auth != nil {
		return e.Next()
	}
	linkId := e.Request.Header.Get("link-id")
	if linkId == "" {
		return apis.NewForbiddenError("Auth required", nil)
	}
	mapId := extractMapIdFromRequest(e.RequestEvent)
	if mapId == "" || !AuthorizeLinkAccess(app, linkId, mapId) {
		return apis.NewForbiddenError("Unauthorized", nil)
	}
	return e.Next()
}

// linkViewAuth validates link-id access for a VIEW request.
// Auth users and superusers pass through. The validate func performs the link-specific check.
func linkViewAuth(e *core.RecordRequestEvent, validate func(linkId string) bool) error {
	if e.HasSuperuserAuth() || e.Auth != nil {
		return e.Next()
	}
	linkId := e.Request.Header.Get("link-id")
	if linkId == "" {
		return apis.NewForbiddenError("Auth required", nil)
	}
	if !validate(linkId) {
		return apis.NewForbiddenError("Unauthorized", nil)
	}
	return e.Next()
}

// RegisterAuthHooks registers authorization hooks for all create/update/delete operations
// and list/view hooks that replace @collection joins with efficient indexed queries.
func RegisterAuthHooks(app *pocketbase.PocketBase) {
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

			if e.Auth != nil && collName != "messages" {
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

			var authorized bool
			if collName == "messages" {
				authorized = authorizeMessageSubscription(app, e.Auth, linkId, filter)
			} else {
				authorized = authorizeLinkSubscription(app, e.Auth, linkId, filter)
			}

			if authorized {
				filtered = append(filtered, sub)
			}
		}

		e.Subscriptions = filtered
		return e.Next()
	})

	// messages LIST: auth user needs role in congregation, link-id needs map assignment.
	app.OnRecordsListRequest("messages").BindFunc(func(e *core.RecordsListRequestEvent) error {
		if e.HasSuperuserAuth() {
			return e.Next()
		}
		mapId := extractMapIdFromRequest(e.RequestEvent)
		if mapId == "" {
			return apis.NewForbiddenError("Missing map filter", nil)
		}

		if e.Auth != nil {
			congId := getMapCongregation(app, mapId)
			if congId != "" && AuthorizeByRole(app, e.Auth.Id, congId) {
				return e.Next()
			}
			return apis.NewForbiddenError("Unauthorized", nil)
		}

		linkId := e.Request.Header.Get("link-id")
		if linkId != "" && AuthorizeLinkAccess(app, linkId, mapId) {
			return e.Next()
		}

		return apis.NewForbiddenError("Unauthorized", nil)
	})

	// addresses LIST + address_options LIST: validate link-id has map assignment.
	app.OnRecordsListRequest("addresses").BindFunc(func(e *core.RecordsListRequestEvent) error {
		return linkMapListAuth(e, app)
	})
	app.OnRecordsListRequest("address_options").BindFunc(func(e *core.RecordsListRequestEvent) error {
		return linkMapListAuth(e, app)
	})

	// address_options VIEW: validate link-id has assignment for the record's map.
	app.OnRecordViewRequest("address_options").BindFunc(func(e *core.RecordRequestEvent) error {
		return linkViewAuth(e, func(linkId string) bool {
			mapId := e.Record.GetString("map")
			return mapId != "" && AuthorizeLinkAccess(app, linkId, mapId)
		})
	})

	// maps VIEW: validate link-id has assignment for this map.
	app.OnRecordViewRequest("maps").BindFunc(func(e *core.RecordRequestEvent) error {
		return linkViewAuth(e, func(linkId string) bool {
			return AuthorizeLinkAccess(app, linkId, e.Record.Id)
		})
	})

	// users LIST: only administrators can list users.
	app.OnRecordsListRequest("users").BindFunc(func(e *core.RecordsListRequestEvent) error {
		if e.HasSuperuserAuth() {
			return e.Next()
		}
		if e.Auth == nil {
			return apis.NewForbiddenError("Auth required", nil)
		}
		var result struct {
			Count int `db:"count"`
		}
		err := app.DB().NewQuery(`
			SELECT COUNT(*) as count FROM roles
			WHERE user = {:userId} AND role = 'administrator'
		`).Bind(dbx.Params{"userId": e.Auth.Id}).One(&result)
		if err != nil || result.Count == 0 {
			return apis.NewForbiddenError("Administrator access required", nil)
		}
		return e.Next()
	})

	// congregations VIEW: validate link-id belongs to the viewed congregation.
	app.OnRecordViewRequest("congregations").BindFunc(func(e *core.RecordRequestEvent) error {
		return linkViewAuth(e, func(linkId string) bool {
			return AuthorizeLinkForCongregation(app, linkId, e.Record.Id)
		})
	})

	// options LIST: validate link-id belongs to the queried congregation.
	app.OnRecordsListRequest("options").BindFunc(func(e *core.RecordsListRequestEvent) error {
		if e.HasSuperuserAuth() {
			return e.Next()
		}
		linkId, isLink := linkOnly(e)
		if !isLink {
			return e.Next()
		}
		if strings.Contains(e.Request.URL.Query().Get("filter"), "||") {
			return apis.NewForbiddenError("Invalid filter", nil)
		}
		if len(e.Records) == 0 {
			return e.Next()
		}
		congId := e.Records[0].GetString("congregation")
		if congId == "" || !AuthorizeLinkForCongregation(app, linkId, congId) {
			return apis.NewForbiddenError("Invalid link access", nil)
		}
		return e.Next()
	})

	// options VIEW: validate link-id belongs to the option's congregation.
	app.OnRecordViewRequest("options").BindFunc(func(e *core.RecordRequestEvent) error {
		return linkViewAuth(e, func(linkId string) bool {
			congId := e.Record.GetString("congregation")
			return congId != "" && AuthorizeLinkForCongregation(app, linkId, congId)
		})
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
