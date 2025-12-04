package handlers

import (
	"log"
	"regexp"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

func HandleNewMap(c *core.RequestEvent, app *pocketbase.PocketBase) error {
	requestInfo, _ := c.RequestInfo()
	data := requestInfo.Body
	territory := data["territory"].(string)
	mapType := data["type"].(string)
	floors := int(data["floors"].(float64))
	name := data["name"].(string)
	congregation := data["congregation"].(string)
	coordinates := data["coordinates"].(string)
	sequence := data["sequence"].(string)

	if !isValidSequence(sequence) {
		log.Println("Invalid sequence format")
		return apis.NewBadRequestError("Invalid sequence format", nil)
	}

	if mapType != "single" && mapType != "multi" {
		log.Println("Invalid map type")
		return apis.NewBadRequestError("Invalid map type", nil)
	}

	if mapType == "single" && floors != 1 {
		log.Println("Invalid number of floors for single map")
		return apis.NewBadRequestError("Invalid floor for single map", nil)
	}

	option, err := fetchDefaultCongregationOption(app, congregation)
	if err != nil {
		log.Println("Error fetching default congregation option:", err)
		return apis.NewNotFoundError("Error fetching default congregation option", nil)
	}

	sequenceArray := splitSequence(sequence)
	// Get max sequence for this territory
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

		log.Printf("Map created successfully with ID: %s", mapRecord.Id)

		for i := 1; i <= floors; i++ {
			for index, seq := range sequenceArray {
				address := createNewAddressRecord(txApp, seq, territory, option.Id, i, index, mapRecord.Id, congregation)
				if err := txApp.Save(address); err != nil {
					log.Println("Error saving address record:", err)
					return err
				}
			}
		}

		return nil
	})

	if err != nil {
		return apis.NewBadRequestError("Error creating map and addresses", nil)
	}

	ResetMapTerritory(mapRecord.Id, app)
	return c.JSON(200, mapRecord)
}

func createNewAddressRecord(txApp core.App, code, territory, option string, floor, sequence int, mapId, congId string) *core.Record {
	collection, _ := txApp.FindCollectionByNameOrId("addresses")
	address := core.NewRecord(collection)
	address.Set("code", code)
	address.Set("territory", territory)
	address.Set("floor", floor)
	address.Set("sequence", sequence)
	address.Set("status", "not_done")
	address.Set("type", option)
	address.Set("map", mapId)
	address.Set("congregation", congId)
	return address
}

func splitSequence(sequence string) []string {
	return strings.Split(sequence, ",")
}

func isValidSequence(sequence string) bool {
	if sequence == "" {
		return false
	}

	re := regexp.MustCompile(`^([a-zA-Z0-9-]+,?)*[a-zA-Z0-9-]+$`)
	return re.MatchString(sequence)
}

func fetchTerritoryMaxSequence(app *pocketbase.PocketBase, territoryId string) (int, error) {
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
