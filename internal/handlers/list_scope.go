package handlers

import (
	"net/http"
	"regexp"
	"strings"

	"ganigeorgiev.com/pocketbase/dbx"
	"ganigeorgiev.com/pocketbase/pocketbase/core"
)

// listScopes ties each scoped collection's rows to who may see them: by the
// row's own congregation field, and where set, its map or its owning account.
// The congregation is never reached through a relation: PocketBase applies
// the related collection's list rule to such a filter, and those rules ask
// for a filter shape this one would not have, so nothing would match.
var listScopes = map[string]struct {
	byMap  bool
	byUser bool
	// link is set where a share link may list the collection.
	link bool
}{
	"maps":            {},
	"territories":     {},
	"groups":          {},
	"options":         {link: true},
	"roles":           {byUser: true},
	"assignments":     {byUser: true},
	"addresses":       {byMap: true, link: true},
	"messages":        {byMap: true, link: true},
	"address_options": {byMap: true, link: true},
}

var recordListPath = regexp.MustCompile(`^/api/collections/([^/]+)/records/?$`)

// Ids come from the database, but they are written into a filter, so anything
// that is not plainly an id is left out.
var plainID = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

const noRows = `id = ""`

// ScopeListQuery adds the caller's scope to a list request's filter, as router
// middleware ahead of PocketBase's list handler. The list hooks prune rows
// only after the query has run, when its total and page boundaries have
// already counted hidden rows, and a count asked often enough spells out a
// hidden value, a share link's id included.
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
		// With no filter the collection's rule or list hook refuses the request.
		if !scoped || strings.TrimSpace(filter) == "" {
			return e.Next()
		}

		var allowed string
		switch linkID := e.Request.Header.Get("link-id"); {
		case linkID != "" && scope.link:
			allowed = linkScope(app, linkID, scope.byMap)
		// A link-id where links list nothing doesn't lift an account's scope:
		// these collections' hooks never read the header.
		case e.Auth != nil:
			allowed = accountScope(app, e.Auth.Id, scope.byUser)
		default:
			return e.Next()
		}

		query.Set("filter", "("+filter+") && ("+allowed+")")
		e.Request.URL.RawQuery = query.Encode()
		return e.Next()
	}
}

// linkScope is what a share link may list: its one map's rows, or where the
// collection has no map, its congregation's.
func linkScope(app core.App, linkID string, byMap bool) string {
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
	if byMap {
		return equalsAny("map", []string{link.Map})
	}
	return equalsAny("congregation", []string{link.Congregation})
}

// accountScope is what a signed-in account may list: rows of the
// congregations it is staff in, and where rows belong to an account, its own.
func accountScope(app core.App, userID string, byUser bool) string {
	congregations, err := userCongregationIDs(app, userID)
	if err != nil {
		return noRows
	}
	scope := equalsAny("congregation", congregations)
	if !byUser {
		return scope
	}
	own := equalsAny("user", []string{userID})
	if scope == noRows {
		return own
	}
	return own + " || " + scope
}

// equalsAny writes the value first ("x" = field): the list hooks and the
// collection rules look for the client's own `field="x"` in the filter, and
// must not take the scope added here for something the client asked.
func equalsAny(field string, ids []string) string {
	var terms []string
	for _, id := range ids {
		if plainID.MatchString(id) {
			terms = append(terms, `"`+id+`" = `+field)
		}
	}
	if len(terms) == 0 {
		return noRows
	}
	return strings.Join(terms, " || ")
}
