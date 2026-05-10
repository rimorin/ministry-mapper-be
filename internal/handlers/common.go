package handlers

import (
	"fmt"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

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

// fetchMapMaxSequence returns the maximum address sequence for a map, defaulting to 1.
func fetchMapMaxSequence(app core.App, mapId string) (int, error) {
	sequence := struct {
		Number int `db:"sequence"`
	}{}
	query := app.DB().NewQuery("SELECT MAX(sequence) as sequence FROM addresses WHERE map = {:map}")
	err := query.Bind(dbx.Params{"map": mapId}).One(&sequence)
	if sequence.Number == 0 {
		sequence.Number = 1
	}
	return sequence.Number, err
}

func fetchMapData(app core.App, mapId string) (*core.Record, error) {
	return app.FindRecordById("maps", mapId)
}

// AuthorizeByRole checks if userId has one of the specified roles in the given congregation.
// If no allowedRoles are provided, any role grants access.
// Uses LIMIT 1 for early exit instead of COUNT(*).
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

// authorizeUserForMap checks if userId has any role in the map's congregation
// using a single joined query instead of two separate lookups.
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
	maxFloor := struct {
		MaxFloor int `db:"max_floor"`
	}{}
	query := app.DB().NewQuery("SELECT MAX(floor) as max_floor FROM addresses WHERE map = {:map}")
	err := query.Bind(dbx.Params{"map": mapId}).One(&maxFloor)
	if maxFloor.MaxFloor == 0 {
		maxFloor.MaxFloor = 1
	}
	return maxFloor.MaxFloor, err
}

// fetchMapLowestFloor returns the lowest floor number for a map, defaulting to 1.
func fetchMapLowestFloor(app core.App, mapId string) (int, error) {
	lowestFloor := struct {
		MinFloor int `db:"min_floor"`
	}{}
	query := app.DB().NewQuery("SELECT MIN(floor) as min_floor FROM addresses WHERE map = {:map}")
	err := query.Bind(dbx.Params{"map": mapId}).One(&lowestFloor)
	if lowestFloor.MinFloor == 0 {
		lowestFloor.MinFloor = 1
	}
	return lowestFloor.MinFloor, err
}
