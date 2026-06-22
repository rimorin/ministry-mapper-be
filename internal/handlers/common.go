package handlers

import (
	"errors"
	"fmt"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
)

// serverError wraps a raw infrastructure error so PocketBase responds with HTTP 500
// and the Sentry middleware captures the real cause via the causer interface.
type serverError struct{ cause error }

func (e *serverError) Error() string { return e.cause.Error() }
func (e *serverError) Cause() error  { return e.cause }
func (e *serverError) Unwrap() error {
	return router.NewInternalServerError("Something went wrong while processing your request.", nil)
}

func newServerError(cause error) error { return &serverError{cause: cause} }

// wrapTransactionError passes through business-logic ApiErrors (400/403/404) unchanged
// and wraps all other errors in a serverError so Sentry captures the real cause.
// Use after RunInTransaction when the transaction body can return both types.
func wrapTransactionError(err error) error {
	var apiErr *router.ApiError
	if errors.As(err, &apiErr) {
		return err
	}
	return newServerError(err)
}

func authID(auth *core.Record) string {
	if auth == nil {
		return ""
	}
	return auth.Id
}

func fetchAddressByCode(app core.App, code string, mapId string) (*core.Record, error) {
	return app.FindFirstRecordByFilter("addresses", "code = {:code} && map = {:map}", dbx.Params{"code": code, "map": mapId})
}

func fetchAddressesByCode(app core.App, code string, mapId string) ([]*core.Record, error) {
	return app.FindRecordsByFilter("addresses", "code = {:code} && map = {:map}", "", 0, 0, dbx.Params{"code": code, "map": mapId})
}

func fetchAddressesByMap(app core.App, mapId string) ([]*core.Record, error) {
	return app.FindRecordsByFilter("addresses", "map = {:id}", "", 0, 0, dbx.Params{"id": mapId})
}

// fetchMapFloors returns the distinct floor levels for a given map.
func fetchMapFloors(app core.App, mapId string) ([]int, error) {
	floors := []struct {
		Level int `db:"floor"`
	}{}
	err := app.DB().NewQuery("SELECT DISTINCT floor FROM addresses WHERE map = {:id}").Bind(dbx.Params{"id": mapId}).All(&floors)
	if err != nil {
		return nil, err
	}
	result := make([]int, len(floors))
	for i, floor := range floors {
		result[i] = int(floor.Level)
	}
	return result, nil
}

// fetchMapScalar runs an aggregate query selecting a single int column `v`
// for the given map, defaulting to 1 when the result is 0/NULL.
func fetchMapScalar(app core.App, sql string, mapId string) (int, error) {
	result := struct {
		V int `db:"v"`
	}{}
	err := app.DB().NewQuery(sql).Bind(dbx.Params{"map": mapId}).One(&result)
	if result.V == 0 {
		result.V = 1
	}
	return result.V, err
}

// fetchMapMaxSequence returns the maximum address sequence for a map, defaulting to 1.
func fetchMapMaxSequence(app core.App, mapId string) (int, error) {
	return fetchMapScalar(app, "SELECT MAX(sequence) as v FROM addresses WHERE map = {:map}", mapId)
}

func fetchMapData(app core.App, mapId string) (*core.Record, error) {
	return app.FindRecordById("maps", mapId)
}

// AuthorizeByRole checks if userId has one of the specified roles in the given congregation.
// If no allowedRoles are provided, any role grants access.
func AuthorizeByRole(app core.App, userId string, congregationId string, allowedRoles ...string) bool {
	var v struct {
		V int `db:"v"`
	}

	if len(allowedRoles) == 0 {
		err := app.DB().NewQuery(`
			SELECT 1 as v FROM roles
			WHERE user = {:userId} AND congregation = {:congId}
			LIMIT 1
		`).Bind(dbx.Params{"userId": userId, "congId": congregationId}).One(&v)
		return err == nil
	}

	params := dbx.Params{"userId": userId, "congId": congregationId}
	placeholders := make([]string, len(allowedRoles))
	for i, role := range allowedRoles {
		key := fmt.Sprintf("role%d", i)
		params[key] = role
		placeholders[i] = "{:" + key + "}"
	}

	query := fmt.Sprintf(`
		SELECT 1 as v FROM roles
		WHERE user = {:userId} AND congregation = {:congId} AND role IN (%s)
		LIMIT 1
	`, strings.Join(placeholders, ", "))

	err := app.DB().NewQuery(query).Bind(params).One(&v)
	return err == nil
}

// AuthorizeLinkAccess checks if a link ID maps to a valid, non-expired assignment for the given map.
func AuthorizeLinkAccess(app core.App, linkId string, mapId string) bool {
	var v struct {
		V int `db:"v"`
	}
	err := app.DB().NewQuery(`
		SELECT 1 as v FROM assignments
		WHERE id = {:linkId} AND map = {:mapId} AND expiry_date > datetime('now')
		LIMIT 1
	`).Bind(dbx.Params{"linkId": linkId, "mapId": mapId}).One(&v)
	return err == nil
}

// AuthorizeLinkForCongregation checks if a link ID maps to a valid, non-expired
// assignment belonging to the given congregation.
func AuthorizeLinkForCongregation(app core.App, linkId string, congregationId string) bool {
	var v struct {
		V int `db:"v"`
	}
	err := app.DB().NewQuery(`
		SELECT 1 as v FROM assignments
		WHERE id = {:linkId} AND congregation = {:congId} AND expiry_date > datetime('now')
		LIMIT 1
	`).Bind(dbx.Params{"linkId": linkId, "congId": congregationId}).One(&v)
	return err == nil
}

// AuthorizeMapAccess checks if the request has access to the given map.
// If link-id is present it takes precedence and must be valid; otherwise role check is used.
func AuthorizeMapAccess(c *core.RequestEvent, app core.App, mapId string) bool {
	if c.HasSuperuserAuth() {
		return true
	}
	linkId := c.Request.Header.Get("link-id")
	if linkId != "" {
		return AuthorizeLinkAccess(app, linkId, mapId)
	}
	return c.Auth != nil && authorizeUserForMap(app, c.Auth.Id, mapId)
}

// resolveActor returns the identity to attribute an address change to, derived
// server-side rather than trusted from the request body: the authenticated
// user's name, or the linked assignment's publisher name for link-id access.
func resolveActor(c *core.RequestEvent, app core.App) string {
	if c.Auth != nil {
		return c.Auth.GetString("name")
	}
	linkId := c.Request.Header.Get("link-id")
	if linkId == "" {
		return ""
	}
	assignment, err := app.FindRecordById("assignments", linkId)
	if err != nil {
		return ""
	}
	return assignment.GetString("publisher")
}

// authorizeUserForMap checks if userId has any role in the map's congregation.
func authorizeUserForMap(app core.App, userId string, mapId string) bool {
	var v struct {
		V int `db:"v"`
	}
	err := app.DB().NewQuery(`
		SELECT 1 as v FROM roles r
		JOIN maps m ON m.congregation = r.congregation
		WHERE m.id = {:mapId} AND r.user = {:userId}
		LIMIT 1
	`).Bind(dbx.Params{"mapId": mapId, "userId": userId}).One(&v)
	return err == nil
}

// authorizeUserForMaps checks if userId has a role in the congregation of every
// map in mapIds using a single query. Returns true only if all maps are authorized.
func authorizeUserForMaps(app core.App, userId string, mapIds []string) bool {
	if len(mapIds) == 0 {
		return false
	}
	unique := make(map[string]struct{}, len(mapIds))
	for _, id := range mapIds {
		unique[id] = struct{}{}
	}
	params := dbx.Params{"userId": userId}
	placeholders := make([]string, 0, len(unique))
	i := 0
	for id := range unique {
		key := fmt.Sprintf("m%d", i)
		params[key] = id
		placeholders = append(placeholders, "{:"+key+"}")
		i++
	}
	var result struct {
		Cnt int `db:"cnt"`
	}
	err := app.DB().NewQuery(
		`SELECT COUNT(DISTINCT m.id) as cnt FROM roles r
		JOIN maps m ON m.congregation = r.congregation
		WHERE m.id IN (` + strings.Join(placeholders, ",") + `) AND r.user = {:userId}`,
	).Bind(params).One(&result)
	return err == nil && result.Cnt == len(unique)
}

func fetchDefaultCongregationOption(app core.App, congregation string) (*core.Record, error) {
	return app.FindFirstRecordByFilter("options", "congregation = {:congregation} && is_default = 1", dbx.Params{"congregation": congregation})
}

func fetchMapAddressCodes(app core.App, mapId string, floor int) ([]*core.Record, error) {
	return app.FindRecordsByFilter("addresses", "floor = {:floor} && map = {:id}", "", 0, 0, dbx.Params{"id": mapId, "floor": floor})
}

// fetchMapMaxFloor returns the highest floor number for a map, defaulting to 1.
func fetchMapMaxFloor(app core.App, mapId string) (int, error) {
	return fetchMapScalar(app, "SELECT MAX(floor) as v FROM addresses WHERE map = {:map}", mapId)
}

// fetchMapLowestFloor returns the lowest floor number for a map, defaulting to 1.
func fetchMapLowestFloor(app core.App, mapId string) (int, error) {
	return fetchMapScalar(app, "SELECT MIN(floor) as v FROM addresses WHERE map = {:map}", mapId)
}
