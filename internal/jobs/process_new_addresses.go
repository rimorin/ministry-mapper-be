package jobs

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"log"
	"os"
	"strings"
	"time"

	"github.com/mailersend/mailersend-go"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
)

type newAddressEntry struct {
	Display     string
	Date        string
	CreatedBy   string
	StatusLabel string
	StatusClass string
	Types       []string
	Notes       string
	HasDetails  bool
}

type newAddressMapGroup struct {
	MapName   string
	Territory string
	Entries   []newAddressEntry
}

// NewAddressesTemplateData holds the data passed to the new_addresses.html template.
type NewAddressesTemplateData struct {
	Maps  []newAddressMapGroup
	Count int
}

type addrTypeRow struct {
	Address     string `db:"address"`
	Description string `db:"description"`
}

type territoryRow struct {
	ID   string `db:"id"`
	Code string `db:"code"`
}

// ProcessNewAddress sends a daily digest of app-created addresses to all
// administrators of the given congregation.
func ProcessNewAddress(congID string, app *pocketbase.PocketBase, since time.Time, tmpl *template.Template) error {
	log.Printf("Processing new addresses for congregation: %s", congID)

	congRecord, err := app.FindRecordById("congregations", congID)
	if err != nil {
		log.Printf("Error finding congregation %s: %v", congID, err)
		return err
	}

	addresses, err := app.FindRecordsByFilter(
		"addresses",
		"congregation = {:congregation} && source = 'app' && created >= {:since}",
		"created",
		0, 0,
		dbx.Params{"congregation": congID, "since": since},
	)
	if err != nil {
		log.Printf("Error finding new addresses for congregation %s: %v", congID, err)
		return err
	}

	if len(addresses) == 0 {
		log.Printf("No new app addresses for congregation %s", congID)
		return nil
	}

	app.ExpandRecords(addresses, []string{"map"}, nil)

	// Batch-fetch option descriptions for all addresses to avoid N+1 queries.
	addressTypes := make(map[string][]string)
	{
		placeholders := make([]string, len(addresses))
		typeParams := dbx.Params{}
		for i, addr := range addresses {
			key := fmt.Sprintf("aid%d", i)
			placeholders[i] = "{:" + key + "}"
			typeParams[key] = addr.Id
		}
		typeQuery := fmt.Sprintf(
			`SELECT ao.address, o.description FROM address_options ao JOIN options o ON ao.option = o.id WHERE ao.address IN (%s) ORDER BY o.sequence ASC`,
			strings.Join(placeholders, ","),
		)
		var typeRows []addrTypeRow
		if err := app.DB().NewQuery(typeQuery).Bind(typeParams).All(&typeRows); err != nil {
			log.Printf("Error fetching address types for congregation %s: %v", congID, err)
			// non-fatal: continue without type labels
		} else {
			for _, row := range typeRows {
				addressTypes[row.Address] = append(addressTypes[row.Address], row.Description)
			}
		}
	}

	// Batch territory code lookup.
	territoryCodes := make(map[string]string)
	{
		uniqueTIDs := []string{}
		seen := map[string]bool{}
		for _, addr := range addresses {
			tid, _ := addr.Get("territory").(string)
			if tid != "" && !seen[tid] {
				seen[tid] = true
				uniqueTIDs = append(uniqueTIDs, tid)
			}
		}
		if len(uniqueTIDs) > 0 {
			placeholders := make([]string, len(uniqueTIDs))
			tParams := dbx.Params{}
			for i, tid := range uniqueTIDs {
				key := fmt.Sprintf("tid%d", i)
				placeholders[i] = "{:" + key + "}"
				tParams[key] = tid
			}
			tQuery := fmt.Sprintf(
				"SELECT id, code FROM territories WHERE id IN (%s)",
				strings.Join(placeholders, ","),
			)
			var tRows []territoryRow
			if err := app.DB().NewQuery(tQuery).Bind(tParams).All(&tRows); err != nil {
				log.Printf("Error fetching territories for congregation %s: %v", congID, err)
				// non-fatal: continue without territory codes
			} else {
				for _, row := range tRows {
					territoryCodes[row.ID] = row.Code
				}
			}
		}
	}

	congregationTz, _ := congRecord.Get("timezone").(string)
	location, err := time.LoadLocation(congregationTz)
	if err != nil {
		location = time.UTC
	}

	// Group addresses by map, preserving insertion order
	groupOrder := []string{}
	groups := make(map[string]*newAddressMapGroup)

	for _, addr := range addresses {
		mapData := addr.ExpandedOne("map")
		mapID, _ := addr.Get("map").(string)
		mapName := ""
		mapType := "single"
		if mapData != nil {
			mapName, _ = mapData.Get("description").(string)
			if t, ok := mapData.Get("type").(string); ok && t != "" {
				mapType = t
			}
		}

		tid, _ := addr.Get("territory").(string)
		territoryCode := territoryCodes[tid]

		code, _ := addr.Get("code").(string)
		floor, _ := addr.Get("floor").(float64)
		var display string
		if mapType == "single" {
			display = code
		} else {
			display = fmt.Sprintf("#%.0f - %s", floor, code)
		}

		createdBy, _ := addr.Get("created_by").(string)

		status, _ := addr.Get("status").(string)
		var statusLabel, statusClass string
		switch status {
		case "done":
			statusLabel, statusClass = "Done", "status-done"
		case "not_home":
			statusLabel, statusClass = "Not Home", "status-not_home"
		case "do_not_call":
			statusLabel, statusClass = "Do Not Call", "status-dnc"
		case "invalid":
			statusLabel, statusClass = "Invalid", "status-invalid"
		}

		notes, _ := addr.Get("notes").(string)
		types := addressTypes[addr.Id]

		entry := newAddressEntry{
			Display:     display,
			Date:        addr.GetDateTime("created").Time().In(location).Format("03:04 PM"),
			CreatedBy:   createdBy,
			StatusLabel: statusLabel,
			StatusClass: statusClass,
			Types:       types,
			Notes:       notes,
			HasDetails:  statusLabel != "" || notes != "" || len(types) > 0,
		}

		if _, exists := groups[mapID]; !exists {
			groupOrder = append(groupOrder, mapID)
			groups[mapID] = &newAddressMapGroup{
				MapName:   mapName,
				Territory: territoryCode,
				Entries:   []newAddressEntry{},
			}
		}
		groups[mapID].Entries = append(groups[mapID].Entries, entry)
	}

	emailData := NewAddressesTemplateData{
		Maps:  make([]newAddressMapGroup, 0, len(groupOrder)),
		Count: len(addresses),
	}
	for _, id := range groupOrder {
		emailData.Maps = append(emailData.Maps, *groups[id])
	}

	recipients := []Recipient{}
	err = app.DB().Select("users.*").From("users").
		InnerJoin("roles", dbx.NewExp("roles.user = users.id and roles.role = 'administrator'")).
		Where(dbx.NewExp("roles.congregation = {:congregation}", dbx.Params{"congregation": congID})).
		All(&recipients)
	if err != nil {
		log.Printf("Error fetching recipients for congregation %s: %v", congID, err)
		return err
	}

	if len(recipients) == 0 {
		log.Printf("No admin recipients for congregation %s", congID)
		return nil
	}

	var body bytes.Buffer
	if err := tmpl.Execute(&body, emailData); err != nil {
		log.Printf("Error executing new_addresses template: %v", err)
		return err
	}

	ms := mailersend.NewMailersend(os.Getenv("MAILERSEND_API_KEY"))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	message := ms.Email.NewMessage()
	message.SetFrom(mailersend.From{
		Email: os.Getenv("MAILERSEND_FROM_EMAIL"),
		Name:  "Ministry Mapper",
	})

	emailRecipients := make([]mailersend.Recipient, 0, len(recipients))
	for _, r := range recipients {
		emailRecipients = append(emailRecipients, mailersend.Recipient{
			Email: r.Email,
			Name:  r.Name,
		})
	}
	message.SetRecipients(emailRecipients)

	congName, _ := congRecord.Get("name").(string)
	message.SetSubject(fmt.Sprintf("New Addresses Added - %s - %s", congName, since.In(location).Format("02 Jan 2006")))
	message.SetHTML(body.String())

	if _, err = ms.Email.Send(ctx, message); err != nil {
		log.Printf("Error sending new addresses email for congregation %s: %v", congID, err)
		return err
	}

	log.Printf("New addresses digest sent for congregation %s (%d addresses)", congID, len(addresses))
	return nil
}

// ProcessNewAddresses finds all congregations that had addresses created via the
// app (source = "app") in the last timeIntervalHours hours and sends a digest
// email to their administrators.
func ProcessNewAddresses(app *pocketbase.PocketBase, since time.Time) error {
	log.Println("Starting new addresses processing")

	congregations := []CongregationData{}
	err := app.DB().NewQuery(
		"SELECT DISTINCT congregation FROM addresses WHERE source = 'app' AND created >= {:since}",
	).Bind(dbx.Params{"since": since}).All(&congregations)
	if err != nil {
		log.Printf("Error fetching congregations with new addresses: %v", err)
		return err
	}

	if len(congregations) == 0 {
		log.Println("Completed: No congregations with new app addresses found")
		return nil
	}

	tmpl, err := template.ParseFiles("templates/new_addresses.html")
	if err != nil {
		log.Printf("Error parsing new_addresses template: %v", err)
		return err
	}

	log.Printf("Processing %d congregation(s) with new addresses", len(congregations))
	for _, c := range congregations {
		if err := ProcessNewAddress(c.ID, app, since, tmpl); err != nil {
			log.Printf("Error processing new addresses for congregation %s: %v", c.ID, err)
		}
	}

	log.Println("New addresses processing completed")
	return nil
}
