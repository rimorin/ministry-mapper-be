package handlers

import (
	"net/http"
	"regexp"
	"strings"

	"ganigeorgiev.com/pocketbase/dbx"
	"ganigeorgiev.com/pocketbase/pocketbase/core"
)

// A list request's filter is the client's. The list hooks drop the rows a
// caller may not see after PocketBase has run it, which hides the rows but
// not what the query said about them: its total still counted them, and a
// page cut where they sorted gave them away. Asked enough questions, a count
// spells out a hidden value, a share link's id included.
//
// So the caller's scope goes into the query itself, before it runs: hidden
// rows never match, and the totals and pages are of visible rows only. The
// hooks stay as they are; they refuse what should be refused and now have
// nothing left to drop.

// listScopes says how each scoped collection ties a row to who may see it:
// the field holding its congregation, and where it has them, its map and the
// account it belongs to. The congregation is the row's own field, never one
// reached through a relation: PocketBase applies the related collection's
// list rule to such a filter, and those rules ask for a filter shape this
// one would not have, so nothing would match.
var listScopes = map[string]struct {
	congregation string
	mapField     string
	userField    string
	// link is set where a share link may list the collection.
	link bool
}{
	"maps":            {congregation: "congregation"},
	"territories":     {congregation: "congregation"},
	"options":         {congregation: "congregation", link: true},
	"roles":           {congregation: "congregation", userField: "user"},
	"assignments":     {congregation: "congregation", userField: "user"},
	"addresses":       {congregation: "congregation", mapField: "map", link: true},
	"messages":        {congregation: "congregation", mapField: "map", link: true},
	"address_options": {congregation: "congregation", mapField: "map", link: true},
}

var recordListPath = regexp.MustCompile(`^/api/collections/([^/]+)/records/?$`)

// Ids come from the database, but they are written into a filter, so anything
// that is not plainly an id is left out.
var plainID = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// noRows is a filter no record satisfies.
const noRows = `id = ""`

// ScopeListQuery narrows a list request to the rows its caller may see. It
// runs as router middleware, ahead of PocketBase's own list handler.
func ScopeListQuery(app core.App) func(*core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		if e.Request.Method != http.MethodGet || e.HasSuperuserAuth() {
			return e.Next()
		}
		match := recordListPath.FindStringSubmatch(e.Request.URL.Path)
		if match == nil {
			return e.Next()
		}
		// A collection answers to its id as well as its name.
		collection, err := app.FindCachedCollectionByNameOrId(match[1])
		if err != nil {
			return e.Next()
		}
		scope, scoped := listScopes[collection.Name]
		query := e.Request.URL.Query()
		filter := query.Get("filter")
		// With no filter the collection's own rule refuses the request.
		if !scoped || strings.TrimSpace(filter) == "" {
			return e.Next()
		}

		var allowed string
		switch linkID := e.Request.Header.Get("link-id"); {
		case linkID != "" && scope.link:
			allowed = linkScope(app, linkID, scope.mapField)
		case linkID != "":
			// A link lists nothing here; the hook refuses it.
			return e.Next()
		case e.Auth != nil:
			allowed = accountScope(app, e.Auth.Id, scope.congregation, scope.userField)
		default:
			return e.Next()
		}

		query.Set("filter", "("+filter+") && ("+allowed+")")
		e.Request.URL.RawQuery = query.Encode()
		return e.Next()
	}
}

// The scope is written value first ("x" = field). The list hooks and the
// collection rules look for the client's own `field="x"` in the filter, and
// must not take the scope added here for something the client asked.

// linkScope is what a share link may list: its one map's rows, or where the
// collection has no map, its congregation's.
func linkScope(app core.App, linkID, mapField string) string {
	var link struct {
		Map          string `db:"map"`
		Congregation string `db:"congregation"`
	}
	err := app.DB().NewQuery(`
		SELECT map, congregation FROM assignments
		WHERE id = {:id} AND expiry_date > datetime('now') LIMIT 1
	`).Bind(dbx.Params{"id": linkID}).One(&link)
	if err != nil {
		return noRows
	}
	if mapField != "" {
		return equalsAny(mapField, []string{link.Map})
	}
	return equalsAny("congregation", []string{link.Congregation})
}

// accountScope is what a signed-in account may list: rows of the
// congregations it is staff in, and where rows belong to an account, its own.
func accountScope(app core.App, userID, congregationField, userField string) string {
	congregations, err := userCongregationIDs(app, userID)
	if err != nil {
		return noRows
	}
	scope := equalsAny(congregationField, congregations)
	if userField == "" {
		return scope
	}
	own := equalsAny(userField, []string{userID})
	if scope == noRows {
		return own
	}
	return own + " || " + scope
}

func equalsAny(field string, ids []string) string {
	var any []string
	for _, id := range ids {
		if plainID.MatchString(id) {
			any = append(any, `"`+id+`" = `+field)
		}
	}
	if len(any) == 0 {
		return noRows
	}
	return strings.Join(any, " || ")
}
