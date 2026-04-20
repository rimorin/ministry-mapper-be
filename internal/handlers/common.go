package handlers

import (
	"fmt"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

func fetchAddressByCode(app *pocketbase.PocketBase, code string, mapId string) (*core.Record, error) {
	return app.FindFirstRecordByFilter("addresses", "code = {:code} && map = {:map}", dbx.Params{"code": code, "map": mapId})
}

func fetchAddressesByCode(app *pocketbase.PocketBase, code string, mapId string) ([]*core.Record, error) {
	return app.FindRecordsByFilter("addresses", "code = {:code} && map = {:map}", "", 0, 0, dbx.Params{"code": code, "map": mapId})
}

func fetchAddressesByMap(app *pocketbase.PocketBase, mapId string) ([]*core.Record, error) {
	return app.FindRecordsByFilter("addresses", "map = {:id}", "", 0, 0, dbx.Params{"id": mapId})
}

// fetchMapFloors returns the distinct floor levels for a given map.
func fetchMapFloors(app *pocketbase.PocketBase, mapId string) ([]int, error) {
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
func fetchMapMaxSequence(app *pocketbase.PocketBase, mapId string) (int, error) {
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

func fetchMapData(app *pocketbase.PocketBase, mapId string) (*core.Record, error) {
	return app.FindRecordById("maps", mapId)
}

// AuthorizeByRole checks if userId has one of the specified roles in the given congregation.
// If no allowedRoles are provided, any role grants access.
func AuthorizeByRole(app *pocketbase.PocketBase, userId string, congregationId string, allowedRoles ...string) bool {
	var result struct {
		Count int `db:"count"`
	}

	if len(allowedRoles) == 0 {
		err := app.DB().NewQuery(`
			SELECT COUNT(*) as count FROM roles
			WHERE user = {:userId} AND congregation = {:congId}
		`).Bind(dbx.Params{"userId": userId, "congId": congregationId}).One(&result)
		return err == nil && result.Count > 0
	}

	params := dbx.Params{"userId": userId, "congId": congregationId}
	placeholders := make([]string, len(allowedRoles))
	for i, role := range allowedRoles {
		key := fmt.Sprintf("role%d", i)
		params[key] = role
		placeholders[i] = "{:" + key + "}"
	}

	query := fmt.Sprintf(`
		SELECT COUNT(*) as count FROM roles
		WHERE user = {:userId} AND congregation = {:congId} AND role IN (%s)
	`, strings.Join(placeholders, ", "))

	err := app.DB().NewQuery(query).Bind(params).One(&result)
	return err == nil && result.Count > 0
}

// AuthorizeLinkAccess checks if a link ID maps to a valid, non-expired assignment for the given map.
func AuthorizeLinkAccess(app *pocketbase.PocketBase, linkId string, mapId string) bool {
	var result struct {
		Count int `db:"count"`
	}
	err := app.DB().NewQuery(`
		SELECT COUNT(*) as count FROM assignments
		WHERE id = {:linkId} AND map = {:mapId} AND expiry_date > datetime('now')
	`).Bind(dbx.Params{"linkId": linkId, "mapId": mapId}).One(&result)

	return err == nil && result.Count > 0
}

// AuthorizeLinkForCongregation checks if a link ID maps to a valid, non-expired
// assignment belonging to the given congregation.
func AuthorizeLinkForCongregation(app *pocketbase.PocketBase, linkId string, congregationId string) bool {
	var result struct {
		Count int `db:"count"`
	}
	err := app.DB().NewQuery(`
		SELECT COUNT(*) as count FROM assignments
		WHERE id = {:linkId} AND congregation = {:congId} AND expiry_date > datetime('now')
	`).Bind(dbx.Params{"linkId": linkId, "congId": congregationId}).One(&result)

	return err == nil && result.Count > 0
}

// AuthorizeMapAccess checks if the request has access to the given map.
// Accepts either an authenticated user or a valid link-id header
// tied to an assignment for the requested map.
func AuthorizeMapAccess(c *core.RequestEvent, app *pocketbase.PocketBase, mapId string) bool {
	if c.Auth != nil {
		return true
	}

	linkId := c.Request.Header.Get("link-id")
	if linkId == "" {
		return false
	}

	return AuthorizeLinkAccess(app, linkId, mapId)
}

func fetchDefaultCongregationOption(app *pocketbase.PocketBase, congregation string) (*core.Record, error) {
	return app.FindFirstRecordByFilter("options", "congregation = {:congregation} && is_default = 1", dbx.Params{"congregation": congregation})
}

func fetchMapAddressCodes(app *pocketbase.PocketBase, mapId string, floor int) ([]*core.Record, error) {
	return app.FindRecordsByFilter("addresses", "floor = {:floor} && map = {:id}", "", 0, 0, dbx.Params{"id": mapId, "floor": floor})
}

// fetchMapMaxFloor returns the highest floor number for a map, defaulting to 1.
func fetchMapMaxFloor(app *pocketbase.PocketBase, mapId string) (int, error) {
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
func fetchMapLowestFloor(app *pocketbase.PocketBase, mapId string) (int, error) {
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
