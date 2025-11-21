package jobs

import (
	"log"
	"time"

	"ministry-mapper/internal/handlers"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
)

// updateTerritoryAggregates updates the territory aggregates based on the specified time interval.
// It fetches territories that have been updated within the given time interval and processes their aggregates.
//
// Parameters:
// - app: A pointer to the PocketBase application instance.
// - timeIntervalMinutes: The time interval in minutes to look back for updated territories.
//
// Returns:
// - error: An error if any occurs during the process, otherwise nil.
func updateTerritoryAggregates(app *pocketbase.PocketBase, timeIntervalMinutes int) error {
	log.Printf("Starting territory aggregates update (interval: %d minutes)", timeIntervalMinutes)

	territories := []struct {
		ID string `db:"id"`
	}{}

	// Calculate the time using the time interval
	timeBuffer := time.Duration(-timeIntervalMinutes) * time.Minute

	err := app.DB().Select("territories.id").From("territories").
		InnerJoin("maps", dbx.NewExp("maps.territory = territories.id")).
		Where(dbx.NewExp("maps.updated > {:updated}", dbx.Params{"updated": time.Now().UTC().Add(timeBuffer)})).
		All(&territories)
	if err != nil {
		log.Println("Error fetching territories:", err)
		return err
	}

	// if no territories found, return
	if len(territories) == 0 {
		log.Println("Completed: No territories found during query")
		return nil
	}

	log.Printf("Processing %d territories\n", len(territories))

	for _, t := range territories {
		err := handlers.ProcessTerritoryAggregates(t.ID, app)
		if err != nil {
			log.Printf("Error processing territory ID %s: %v\n", t.ID, err)
		}
	}

	log.Println("Territory aggregates update completed")
	return nil
}
