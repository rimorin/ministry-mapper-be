package handlers

import (
	"github.com/getsentry/sentry-go"
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

// fetchMapFloors retrieves a list of distinct floor levels for a given map ID from the database.
// It queries the 'addresses' table to find unique floor levels associated with the specified map.
//
// Parameters:
//   - app: A pointer to the PocketBase application instance.
//   - mapId: A string representing the ID of the map.
//
// Returns:
//   - A slice of integers representing the distinct floor levels.
//   - An error if the query fails or any other issue occurs during the execution.
func fetchMapFloors(app *pocketbase.PocketBase, mapId string) ([]int, error) {
	floors := []struct {
		Level int `db:"floor"`
	}{}
	err := app.DB().NewQuery("SELECT DISTINCT floor FROM addresses WHERE map = {:id}").Bind(dbx.Params{"id": mapId}).All(&floors)
	if err != nil {
		sentry.CaptureException(err)
		return nil, err
	}
	result := make([]int, len(floors))
	for i, floor := range floors {
		result[i] = int(floor.Level)
	}
	return result, nil
}

// fetchMapMaxSequence retrieves the maximum sequence number from the addresses table for a given map ID.
// If no sequence number is found, it defaults to 1.
//
// Parameters:
//   - app: A pointer to the PocketBase application instance.
//   - mapId: The ID of the map for which the maximum sequence number is to be fetched.
//
// Returns:
//   - int: The maximum sequence number for the given map ID, or 1 if no sequence number is found.
//   - error: An error object if there is an issue executing the query.
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

func fetchDefaultCongregationOption(app *pocketbase.PocketBase, congregation string) (*core.Record, error) {
	return app.FindFirstRecordByFilter("options", "congregation = {:congregation} && is_default = 1", dbx.Params{"congregation": congregation})
}

func fetchMapAddressCodes(app *pocketbase.PocketBase, mapId string, floor int) ([]*core.Record, error) {
	return app.FindRecordsByFilter("addresses", "floor = {:floor} && map = {:id}", "", 0, 0, dbx.Params{"id": mapId, "floor": floor})
}

// fetchMapMaxFloor retrieves the maximum floor number for a given map from the database.
// If no floors are found, it defaults to 1.
//
// Parameters:
//   - app: A pointer to the PocketBase application instance.
//   - mapId: The ID of the map for which to fetch the maximum floor.
//
// Returns:
//   - int: The maximum floor number for the specified map.
//   - error: An error object if an error occurred during the query execution.
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

// fetchMapLowestFloor retrieves the lowest floor number for a given map from the database.
// If no floors are found, it defaults to returning 1.
//
// Parameters:
//   - app: A pointer to the PocketBase application instance.
//   - mapId: The ID of the map for which to fetch the lowest floor.
//
// Returns:
//   - int: The lowest floor number for the specified map.
//   - error: An error object if an error occurred during the query, otherwise nil.
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
