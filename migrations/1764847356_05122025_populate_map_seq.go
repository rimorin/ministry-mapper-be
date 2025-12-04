package migrations

import (
	"log"
	"strconv"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		return app.RunInTransaction(func(txApp core.App) error {
			records, err := txApp.FindAllRecords("maps")
			if err != nil {
				return err
			}

			for _, record := range records {
				code := record.GetString("code")
				sequence, err := strconv.Atoi(code)
				if err != nil {
					log.Printf("Skipping map %s: invalid code '%s'", record.Id, code)
					continue
				}

				record.Set("sequence", sequence)
				if err := txApp.Save(record); err != nil {
					return err
				}
			}

			log.Printf("Updated sequence field for %d maps", len(records))
			return nil
		})
	}, func(app core.App) error {
		return nil
	})
}
