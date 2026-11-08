package handlers

import (
	"fmt"
	"log"
	"regexp"
	"strings"

	"ganigeorgiev.com/pocketbase/dbx"
	"ganigeorgiev.com/pocketbase/pocketbase/apis"
	"ganigeorgiev.com/pocketbase/pocketbase/core"
)

type NewMapRequest struct {
	Territory    string `json:"territory"`
	Type         string `json:"type"`
	Floors       int    `json:"floors"`
	Name         string `json:"name"`
	Congregation string `json:"congregation"`
	Coordinates  string `json:"coordinates"`
	Sequence     string `json:"sequence"`
	// StartFloor is a building's lowest floor (default 1): many blocks have
	// homes only from the 2nd floor up. Floor 0 is skipped, as when a floor
	// is added below floor 1.
	StartFloor *int `json:"start_floor"`
}

// Limits well above the largest real maps (48 floors, 322 units on one
// floor, 875 homes), so a typing slip cannot create a runaway map.
const (
	maxMapFloors     = 100
	maxUnitsPerFloor = 500
	maxMapHomes      = 5000
	minStartFloor    = -10
	maxStartFloor    = 100
)

// mapFloors lists the floors of a new map: count floors from start upwards,
// with no floor 0.
func mapFloors(start, count int) []int {
	floors := make([]int, 0, count)
	for f := start; len(floors) < count; f++ {
		if f != 0 {
			floors = append(floors, f)
		}
	}
	return floors
}

func HandleNewMap(c *core.RequestEvent, app core.App) error {
	data := NewMapRequest{}
	if err := c.BindBody(&data); err != nil {
		return apis.NewBadRequestError("Invalid request body", nil)
	}

	territory := data.Territory
	mapType := data.Type
	floors := data.Floors
	name := data.Name
	congregation := data.Congregation
	coordinates := data.Coordinates

	if territory == "" {
		return apis.NewBadRequestError("territory is required", nil)
	}

	if congregation == "" {
		return apis.NewBadRequestError("congregation is required", nil)
	}

	if name == "" {
		return apis.NewBadRequestError("name is required", nil)
	}

	if !isValidSequence(data.Sequence) {
		return apis.NewBadRequestError("Invalid sequence format", nil)
	}

	if mapType != "single" && mapType != "multi" {
		return apis.NewBadRequestError("Invalid map type", nil)
	}

	if floors < 1 {
		return apis.NewBadRequestError("floors must be at least 1", nil)
	}

	if mapType == "single" && floors != 1 {
		return apis.NewBadRequestError("Invalid floor for single map", nil)
	}

	if floors > maxMapFloors {
		return apis.NewBadRequestError(fmt.Sprintf("floors must be at most %d", maxMapFloors), nil)
	}

	startFloor := 1
	if data.StartFloor != nil {
		startFloor = *data.StartFloor
	}
	if startFloor == 0 || startFloor < minStartFloor || startFloor > maxStartFloor {
		return apis.NewBadRequestError(fmt.Sprintf("start_floor must be between %d and %d, and not 0", minStartFloor, maxStartFloor), nil)
	}
	if mapType == "single" && startFloor != 1 {
		return apis.NewBadRequestError("A single map has no start floor", nil)
	}

	sequenceArray := strings.Split(data.Sequence, ",")
	if len(sequenceArray) > maxUnitsPerFloor {
		return apis.NewBadRequestError(fmt.Sprintf("A map may have at most %d units per floor", maxUnitsPerFloor), nil)
	}
	if len(sequenceArray)*floors > maxMapHomes {
		return apis.NewBadRequestError(fmt.Sprintf("A map may have at most %d homes", maxMapHomes), nil)
	}
	seen := make(map[string]bool, len(sequenceArray))
	for _, code := range sequenceArray {
		if seen[code] {
			return apis.NewBadRequestError(fmt.Sprintf("Duplicate unit in sequence: '%s'", code), nil)
		}
		seen[code] = true
	}

	if !AuthorizeByRole(app, c.Auth.Id, congregation, "administrator") {
		return apis.NewForbiddenError("Administrator access required", nil)
	}

	option, err := fetchDefaultCongregationOption(app, congregation)
	if err != nil {
		log.Println("Error fetching default congregation option:", err)
		return apis.NewNotFoundError("Error fetching default congregation option", nil)
	}

	maxSeq, err := fetchTerritoryMaxSequence(app, territory)
	if err != nil {
		log.Println("Error fetching max sequence:", err)
		return apis.NewBadRequestError("Error fetching max sequence", nil)
	}

	var mapRecord *core.Record

	err = app.RunInTransaction(func(txApp core.App) error {
		collection, _ := txApp.FindCollectionByNameOrId("maps")
		mapRecord = core.NewRecord(collection)
		mapRecord.Set("territory", territory)
		mapRecord.Set("type", mapType)
		mapRecord.Set("description", name)
		mapRecord.Set("congregation", congregation)
		mapRecord.Set("coordinates", coordinates)
		mapRecord.Set("sequence", maxSeq)

		if err := txApp.Save(mapRecord); err != nil {
			log.Printf("Error creating map: %v", err)
			return err
		}

		aoCollection, err := txApp.FindCachedCollectionByNameOrId("address_options")
		if err != nil {
			log.Printf("Error finding address_options collection: %v", err)
			return err
		}

		addressCollection, err := txApp.FindCachedCollectionByNameOrId("addresses")
		if err != nil {
			log.Printf("Error finding addresses collection: %v", err)
			return err
		}

		createdBy := c.Auth.GetString("name")

		for _, floor := range mapFloors(startFloor, floors) {
			for index, seq := range sequenceArray {
				address := createNewAddressRecord(addressCollection, seq, territory, floor, index, mapRecord.Id, congregation, createdBy)
				if err := txApp.SaveNoValidate(address); err != nil {
					log.Println("Error saving address record:", err)
					return err
				}
				aoRec := core.NewRecord(aoCollection)
				aoRec.Set("address", address.Id)
				aoRec.Set("option", option.Id)
				aoRec.Set("congregation", congregation)
				aoRec.Set("map", mapRecord.Id)
				if err := txApp.SaveNoValidate(aoRec); err != nil {
					log.Println("Error inserting address_options record:", err)
					return err
				}
			}
		}

		return nil
	})

	if err != nil {
		return newServerError(err)
	}

	// The new map's counts (homes, not done, progress) are worked out now:
	// without them the map list shows 0 homes until someone changes a home.
	// Before the territory, which adds up its maps.
	if err := ProcessMapAggregates(mapRecord.Id, app, false); err != nil {
		log.Printf("Error calculating aggregates for new map %s: %v", mapRecord.Id, err)
	}
	ResetMapTerritory(mapRecord.Id, app)
	if fresh, err := app.FindRecordById("maps", mapRecord.Id); err == nil {
		mapRecord = fresh
	}
	return c.JSON(200, mapRecord)
}

// createNewAddressRecord builds (but does not save) an address record. The
// collection is passed in because this runs once per address in a nested loop.
func createNewAddressRecord(collection *core.Collection, code, territory string, floor, sequence int, mapId, congId, createdBy string) *core.Record {
	address := core.NewRecord(collection)
	address.Set("code", code)
	address.Set("territory", territory)
	address.Set("floor", floor)
	address.Set("sequence", sequence)
	address.Set("status", "not_done")
	address.Set("map", mapId)
	address.Set("congregation", congId)
	address.Set("source", "map_init")
	address.Set("created_by", createdBy)
	return address
}

var sequenceFormatRegex = regexp.MustCompile(`^([a-zA-Z0-9-]+,?)*[a-zA-Z0-9-]+$`)

func isValidSequence(sequence string) bool {
	if sequence == "" {
		return false
	}

	return sequenceFormatRegex.MatchString(sequence)
}

func fetchTerritoryMaxSequence(app core.App, territoryId string) (int, error) {
	result := struct {
		MaxSequence int `db:"max_sequence"`
	}{}
	query := app.DB().NewQuery("SELECT COALESCE(MAX(sequence), 0) + 1 as max_sequence FROM maps WHERE territory = {:territory}")
	err := query.Bind(dbx.Params{"territory": territoryId}).One(&result)
	if err != nil {
		return 1, err
	}
	if result.MaxSequence == 0 {
		return 1, nil
	}
	return result.MaxSequence, nil
}
