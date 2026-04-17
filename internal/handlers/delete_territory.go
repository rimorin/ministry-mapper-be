package handlers

import (
	"log"
	"net/http"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// HandleDeleteTerritory deletes a territory and all its child records atomically.
// Child records (address_options, assignments, messages, addresses, maps) are deleted
// via raw SQL to suppress cascade realtime events. The territory is deleted via
// txApp.Delete inside the same transaction, which fires exactly one realtime event
// after the transaction commits.
func HandleDeleteTerritory(e *core.RequestEvent, app *pocketbase.PocketBase) error {
	requestInfo, _ := e.RequestInfo()
	data := requestInfo.Body
	territoryId, ok := data["territory"].(string)
	if !ok || territoryId == "" {
		return apis.NewBadRequestError("Missing territory ID", nil)
	}

	territory, err := app.FindRecordById("territories", territoryId)
	if err != nil {
		return apis.NewNotFoundError("Territory not found", nil)
	}

	err = app.RunInTransaction(func(txApp core.App) error {
		params := dbx.Params{"id": territoryId}
		for _, q := range []string{
			"DELETE FROM address_options WHERE map IN (SELECT id FROM maps WHERE territory = {:id})",
			"DELETE FROM assignments WHERE map IN (SELECT id FROM maps WHERE territory = {:id})",
			"DELETE FROM messages WHERE map IN (SELECT id FROM maps WHERE territory = {:id})",
			"DELETE FROM addresses WHERE map IN (SELECT id FROM maps WHERE territory = {:id})",
			"DELETE FROM maps WHERE territory = {:id}",
		} {
			if _, err := txApp.DB().NewQuery(q).Bind(params).Execute(); err != nil {
				return err
			}
		}
		return txApp.Delete(territory)
	})

	if err != nil {
		log.Printf("Error deleting territory %s: %v", territoryId, err)
		return apis.NewBadRequestError("Error deleting territory", nil)
	}

	return e.JSON(http.StatusOK, "Territory deleted successfully")
}
