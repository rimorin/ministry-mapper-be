//go:build testdata

package migrations

import (
	"fmt"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

const (
	seedAlphaCongID = "testcongalpha01"
	seedBetaCongID  = "testcongbeta001"
)

func init() {
	m.Register(func(app core.App) error {
		// Idempotency: skip if seed already applied.
		if _, err := app.FindFirstRecordByFilter("congregations", "id = {:id}", dbx.Params{"id": seedAlphaCongID}); err == nil {
			return nil
		}

		now := time.Now().UTC().Format("2006-01-02 15:04:05.000Z")

		// -------------------------------------------------------------------
		// Congregations
		// -------------------------------------------------------------------
		congCol, err := app.FindCollectionByNameOrId("congregations")
		if err != nil {
			return fmt.Errorf("find congregations: %w", err)
		}
		for _, c := range []struct{ id, name, code, origin, timezone string }{
			{seedAlphaCongID, "Alpha Congregation", "ALPHA", "sg", "Asia/Singapore"},
			{seedBetaCongID, "Beta Congregation", "BETA", "sg", "Asia/Singapore"},
		} {
			rec := core.NewRecord(congCol)
			rec.Id = c.id
			rec.Set("name", c.name)
			rec.Set("code", c.code)
			rec.Set("origin", c.origin)
			rec.Set("timezone", c.timezone)
			rec.Set("expiry_hours", 24)
			rec.Set("max_tries", 3)
			rec.Set("created", now)
			rec.Set("updated", now)
			if err := app.SaveNoValidate(rec); err != nil {
				return fmt.Errorf("save congregation %s: %w", c.id, err)
			}
		}

		// -------------------------------------------------------------------
		// Options
		// -------------------------------------------------------------------
		optCol, err := app.FindCollectionByNameOrId("options")
		if err != nil {
			return fmt.Errorf("find options: %w", err)
		}
		for _, o := range []struct {
			id, congregation, code, description string
			seq                                 int
			isCountable, isDefault              bool
		}{
			{"testoptialpha01", seedAlphaCongID, "NH", "Not Home", 1, true, false},
			{"testoptialpha02", seedAlphaCongID, "DNC", "Do Not Call", 2, false, false},
			{"testoptialpha03", seedAlphaCongID, "LN", "Language Note", 3, true, true},
			{"testoptibeta001", seedBetaCongID, "NH", "Not Home", 1, true, true},
			{"testoptibeta002", seedBetaCongID, "DNC", "Do Not Call", 2, false, false},
		} {
			rec := core.NewRecord(optCol)
			rec.Id = o.id
			rec.Set("congregation", o.congregation)
			rec.Set("code", o.code)
			rec.Set("description", o.description)
			rec.Set("sequence", o.seq)
			rec.Set("is_countable", o.isCountable)
			rec.Set("is_default", o.isDefault)
			rec.Set("created", now)
			rec.Set("updated", now)
			if err := app.SaveNoValidate(rec); err != nil {
				return fmt.Errorf("save option %s: %w", o.id, err)
			}
		}

		// -------------------------------------------------------------------
		// Users
		// -------------------------------------------------------------------
		userCol, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return fmt.Errorf("find users: %w", err)
		}
		for _, u := range []struct{ id, email, name string }{
			{"testuseralpha01", "admin@alpha.test", "Alpha Admin"},
			{"testuseralpha02", "conductor@alpha.test", "Alpha Conductor"},
			{"testuseralpha03", "readonly@alpha.test", "Alpha ReadOnly"},
			{"testuserbeta001", "admin@beta.test", "Beta Admin"},
			{"testuserbeta002", "xcong@beta.test", "Beta Xcong"},
		} {
			rec := core.NewRecord(userCol)
			rec.Id = u.id
			rec.Set("email", u.email)
			rec.Set("name", u.name)
			rec.Set("verified", true)
			rec.Set("disabled", false)
			rec.SetPassword("Test1234!")
			rec.Set("created", now)
			rec.Set("updated", now)
			if err := app.SaveNoValidate(rec); err != nil {
				return fmt.Errorf("save user %s: %w", u.email, err)
			}
		}

		// -------------------------------------------------------------------
		// Roles
		// -------------------------------------------------------------------
		roleCol, err := app.FindCollectionByNameOrId("roles")
		if err != nil {
			return fmt.Errorf("find roles: %w", err)
		}
		for _, r := range []struct{ id, congregation, user, role string }{
			{"testrolexcng01a", seedAlphaCongID, "testuseralpha01", "administrator"},
			{"testrolexcng01b", seedAlphaCongID, "testuseralpha02", "conductor"},
			{"testrolexcng01c", seedAlphaCongID, "testuseralpha03", "read_only"},
			{"testrolexcng02a", seedBetaCongID, "testuserbeta001", "administrator"},
			{"testrolexcng02b", seedBetaCongID, "testuserbeta002", "conductor"},
		} {
			rec := core.NewRecord(roleCol)
			rec.Id = r.id
			rec.Set("congregation", r.congregation)
			rec.Set("user", r.user)
			rec.Set("role", r.role)
			rec.Set("created", now)
			rec.Set("updated", now)
			if err := app.SaveNoValidate(rec); err != nil {
				return fmt.Errorf("save role %s: %w", r.id, err)
			}
		}

		// -------------------------------------------------------------------
		// Territories
		// -------------------------------------------------------------------
		terrCol, err := app.FindCollectionByNameOrId("territories")
		if err != nil {
			return fmt.Errorf("find territories: %w", err)
		}
		for _, t := range []struct{ id, congregation, code, description string }{
			{"testterralpha01", seedAlphaCongID, "T01", "Alpha Territory 01"},
			{"testterralpha02", seedAlphaCongID, "T02", "Alpha Territory 02"},
			{"testterrbeta001", seedBetaCongID, "T01", "Beta Territory 01"},
		} {
			rec := core.NewRecord(terrCol)
			rec.Id = t.id
			rec.Set("congregation", t.congregation)
			rec.Set("code", t.code)
			rec.Set("description", t.description)
			rec.Set("progress", 0)
			rec.Set("created", now)
			rec.Set("updated", now)
			if err := app.SaveNoValidate(rec); err != nil {
				return fmt.Errorf("save territory %s: %w", t.id, err)
			}
		}

		// -------------------------------------------------------------------
		// Maps
		// -------------------------------------------------------------------
		mapCol, err := app.FindCollectionByNameOrId("maps")
		if err != nil {
			return fmt.Errorf("find maps: %w", err)
		}
		for _, mp := range []struct {
			id, territory, congregation, code, description, mapType string
			seq                                                      int
		}{
			{"testmapalpha01a", "testterralpha01", seedAlphaCongID, "A", "Blk 100A", "single", 1},
			{"testmapalpha01b", "testterralpha01", seedAlphaCongID, "B", "Blk 100B", "single", 2},
			{"testmapalpha02a", "testterralpha02", seedAlphaCongID, "A", "Blk 200A", "single", 1},
			{"testmapalpha02b", "testterralpha02", seedAlphaCongID, "B", "Blk 200B", "single", 2},
			{"testmapbeta001a", "testterrbeta001", seedBetaCongID, "A", "Blk 300A", "single", 1},
			{"testmapbeta001b", "testterrbeta001", seedBetaCongID, "B", "Blk 300B", "single", 2},
			// testmapalphsc01: single-code map for "last code delete" guard test
			{"testmapalphsc01", "testterralpha01", seedAlphaCongID, "SC", "Single Code Blk", "single", 10},
			// testmapalphcf01: multi-floor map for floor add/remove tests
			{"testmapalphcf01", "testterralpha01", seedAlphaCongID, "CF", "Multi Floor Blk", "single", 11},
		} {
			rec := core.NewRecord(mapCol)
			rec.Id = mp.id
			rec.Set("territory", mp.territory)
			rec.Set("congregation", mp.congregation)
			rec.Set("code", mp.code)
			rec.Set("description", mp.description)
			rec.Set("type", mp.mapType)
			rec.Set("sequence", mp.seq)
			rec.Set("progress", 0)
			rec.Set("created", now)
			rec.Set("updated", now)
			if err := app.SaveNoValidate(rec); err != nil {
				return fmt.Errorf("save map %s: %w", mp.id, err)
			}
		}
		// testmapalphrich1: JSON description, coordinates, aggregates, non-zero progress
		{
			rec := core.NewRecord(mapCol)
			rec.Id = "testmapalphrich1"
			rec.Set("territory", "testterralpha02")
			rec.Set("congregation", seedAlphaCongID)
			rec.Set("code", "R")
			rec.Set("description", `{"en":"Rich Map","zh":"富地图"}`)
			rec.Set("type", "multi")
			rec.Set("sequence", 12)
			rec.Set("progress", 40)
			rec.Set("coordinates", map[string]float64{"lat": 1.234, "lng": 103.456})
			rec.Set("aggregates", map[string]any{"notDone": 3, "notHome": 1, "done": 2, "dnc": 0, "invalid": 0})
			rec.Set("created", now)
			rec.Set("updated", now)
			if err := app.SaveNoValidate(rec); err != nil {
				return fmt.Errorf("save map testmapalphrich1: %w", err)
			}
		}
		// testmapalphempt1: empty description, null coordinates/aggregates, no addresses
		{
			rec := core.NewRecord(mapCol)
			rec.Id = "testmapalphempt1"
			rec.Set("territory", "testterralpha02")
			rec.Set("congregation", seedAlphaCongID)
			rec.Set("code", "X")
			rec.Set("description", "")
			rec.Set("type", "single")
			rec.Set("sequence", 13)
			rec.Set("progress", 0)
			rec.Set("created", now)
			rec.Set("updated", now)
			if err := app.SaveNoValidate(rec); err != nil {
				return fmt.Errorf("save map testmapalphempt1: %w", err)
			}
		}

		// -------------------------------------------------------------------
		// Addresses
		// testmapalpha01a has 2 not_home + 1 done so the reset test can verify changes.
		// All maps have at least 2 not_done addresses for the address-fetch test.
		// -------------------------------------------------------------------
		addrCol, err := app.FindCollectionByNameOrId("addresses")
		if err != nil {
			return fmt.Errorf("find addresses: %w", err)
		}
		for _, a := range []struct {
			id, mapID, territory, congregation, code, status string
			floor, seq                                       int
		}{
			// testmapalpha01a: 2 not_done, 2 not_home, 1 done
			{"testalpha01a001", "testmapalpha01a", "testterralpha01", seedAlphaCongID, "10", "not_done", 1, 1},
			{"testalpha01a002", "testmapalpha01a", "testterralpha01", seedAlphaCongID, "11", "not_done", 1, 2},
			{"testalpha01a003", "testmapalpha01a", "testterralpha01", seedAlphaCongID, "12", "not_home", 1, 3},
			{"testalpha01a004", "testmapalpha01a", "testterralpha01", seedAlphaCongID, "13", "not_home", 1, 4},
			{"testalpha01a005", "testmapalpha01a", "testterralpha01", seedAlphaCongID, "14", "done", 1, 5},
			// testmapalpha01b: all not_done
			{"testalpha01b001", "testmapalpha01b", "testterralpha01", seedAlphaCongID, "20", "not_done", 1, 1},
			{"testalpha01b002", "testmapalpha01b", "testterralpha01", seedAlphaCongID, "21", "not_done", 1, 2},
			{"testalpha01b003", "testmapalpha01b", "testterralpha01", seedAlphaCongID, "22", "not_done", 1, 3},
			{"testalpha01b004", "testmapalpha01b", "testterralpha01", seedAlphaCongID, "23", "not_done", 1, 4},
			{"testalpha01b005", "testmapalpha01b", "testterralpha01", seedAlphaCongID, "24", "not_done", 1, 5},
			// testmapalpha02a: 3 not_done, 1 done, 1 not_home
			{"testalpha02a001", "testmapalpha02a", "testterralpha02", seedAlphaCongID, "30", "not_done", 1, 1},
			{"testalpha02a002", "testmapalpha02a", "testterralpha02", seedAlphaCongID, "31", "not_done", 1, 2},
			{"testalpha02a003", "testmapalpha02a", "testterralpha02", seedAlphaCongID, "32", "not_done", 1, 3},
			{"testalpha02a004", "testmapalpha02a", "testterralpha02", seedAlphaCongID, "33", "done", 1, 4},
			{"testalpha02a005", "testmapalpha02a", "testterralpha02", seedAlphaCongID, "34", "not_home", 1, 5},
			// testmapalpha02b: all not_done
			{"testalpha02b001", "testmapalpha02b", "testterralpha02", seedAlphaCongID, "40", "not_done", 1, 1},
			{"testalpha02b002", "testmapalpha02b", "testterralpha02", seedAlphaCongID, "41", "not_done", 1, 2},
			{"testalpha02b003", "testmapalpha02b", "testterralpha02", seedAlphaCongID, "42", "not_done", 1, 3},
			{"testalpha02b004", "testmapalpha02b", "testterralpha02", seedAlphaCongID, "43", "not_done", 1, 4},
			{"testalpha02b005", "testmapalpha02b", "testterralpha02", seedAlphaCongID, "44", "not_done", 1, 5},
			// testmapbeta001a: all not_done
			{"testbeta001a001", "testmapbeta001a", "testterrbeta001", seedBetaCongID, "50", "not_done", 1, 1},
			{"testbeta001a002", "testmapbeta001a", "testterrbeta001", seedBetaCongID, "51", "not_done", 1, 2},
			{"testbeta001a003", "testmapbeta001a", "testterrbeta001", seedBetaCongID, "52", "not_done", 1, 3},
			{"testbeta001a004", "testmapbeta001a", "testterrbeta001", seedBetaCongID, "53", "not_done", 1, 4},
			{"testbeta001a005", "testmapbeta001a", "testterrbeta001", seedBetaCongID, "54", "not_done", 1, 5},
			// testmapbeta001b: all not_done
			{"testbeta001b001", "testmapbeta001b", "testterrbeta001", seedBetaCongID, "60", "not_done", 1, 1},
			{"testbeta001b002", "testmapbeta001b", "testterrbeta001", seedBetaCongID, "61", "not_done", 1, 2},
			{"testbeta001b003", "testmapbeta001b", "testterrbeta001", seedBetaCongID, "62", "not_done", 1, 3},
			{"testbeta001b004", "testmapbeta001b", "testterrbeta001", seedBetaCongID, "63", "not_done", 1, 4},
			{"testbeta001b005", "testmapbeta001b", "testterrbeta001", seedBetaCongID, "64", "not_done", 1, 5},
			// testmapalphsc01: single code (for "last code delete" guard test)
			{"testalphsc01001", "testmapalphsc01", "testterralpha01", seedAlphaCongID, "99", "not_done", 1, 1},
			// testmapalphcf01: 2 floors × 2 codes (for floor add/remove tests)
			{"testalphcf01001", "testmapalphcf01", "testterralpha01", seedAlphaCongID, "01", "not_done", 1, 1},
			{"testalphcf01002", "testmapalphcf01", "testterralpha01", seedAlphaCongID, "02", "not_done", 1, 2},
			{"testalphcf01003", "testmapalphcf01", "testterralpha01", seedAlphaCongID, "01", "not_done", 2, 1},
			{"testalphcf01004", "testmapalphcf01", "testterralpha01", seedAlphaCongID, "02", "not_done", 2, 2},
		} {
			rec := core.NewRecord(addrCol)
			rec.Id = a.id
			rec.Set("map", a.mapID)
			rec.Set("territory", a.territory)
			rec.Set("congregation", a.congregation)
			rec.Set("code", a.code)
			rec.Set("status", a.status)
			rec.Set("floor", a.floor)
			rec.Set("sequence", a.seq)
			rec.Set("not_home_tries", 0)
			rec.Set("notes", "")
			rec.Set("dnc_time", "")
			rec.Set("source", "")
			rec.Set("created_by", "")
			rec.Set("updated_by", "")
			rec.Set("created", now)
			rec.Set("updated", now)
			if err := app.SaveNoValidate(rec); err != nil {
				return fmt.Errorf("save address %s: %w", a.id, err)
			}
		}

		// testmapalphrich1: 2 rich addresses with varied optional fields
		{
			rec := core.NewRecord(addrCol)
			rec.Id = "testalpharich01"
			rec.Set("map", "testmapalphrich1")
			rec.Set("territory", "testterralpha02")
			rec.Set("congregation", seedAlphaCongID)
			rec.Set("code", "R01")
			rec.Set("status", "not_home")
			rec.Set("floor", 1)
			rec.Set("sequence", 1)
			rec.Set("not_home_tries", 2)
			rec.Set("notes", "Speaks Mandarin")
			rec.Set("coordinates", map[string]float64{"lat": 1.111, "lng": 103.111})
			rec.Set("dnc_time", "")
			rec.Set("source", "")
			rec.Set("created_by", "")
			rec.Set("updated_by", "")
			rec.Set("created", now)
			rec.Set("updated", now)
			if err := app.SaveNoValidate(rec); err != nil {
				return fmt.Errorf("save address testalpharich01: %w", err)
			}
		}
		{
			rec := core.NewRecord(addrCol)
			rec.Id = "testalpharich02"
			rec.Set("map", "testmapalphrich1")
			rec.Set("territory", "testterralpha02")
			rec.Set("congregation", seedAlphaCongID)
			rec.Set("code", "R02")
			rec.Set("status", "dnc")
			rec.Set("floor", 1)
			rec.Set("sequence", 2)
			rec.Set("not_home_tries", 0)
			rec.Set("notes", "")
			rec.Set("dnc_time", "2024-01-15 10:00:00.000Z")
			rec.Set("source", "")
			rec.Set("created_by", "")
			rec.Set("updated_by", "testuseralpha01")
			rec.Set("created", now)
			rec.Set("updated", now)
			if err := app.SaveNoValidate(rec); err != nil {
				return fmt.Errorf("save address testalpharich02: %w", err)
			}
		}

		// -------------------------------------------------------------------
		// Address Options (junction)
		// -------------------------------------------------------------------
		aoCol, err := app.FindCollectionByNameOrId("address_options")
		if err != nil {
			return fmt.Errorf("find address_options: %w", err)
		}
		for _, ao := range []struct{ id, address, mapID, congregation, option string }{
			{"testaoalph01001", "testalpha01a003", "testmapalpha01a", seedAlphaCongID, "testoptialpha01"},
			{"testaoalph01002", "testalpha01a004", "testmapalpha01a", seedAlphaCongID, "testoptialpha01"},
			{"testaoalph02001", "testalpha02a004", "testmapalpha02a", seedAlphaCongID, "testoptialpha02"},
			{"testaorichaddr1", "testalpharich01", "testmapalphrich1", seedAlphaCongID, "testoptialpha01"},
		} {
			rec := core.NewRecord(aoCol)
			rec.Id = ao.id
			rec.Set("address", ao.address)
			rec.Set("map", ao.mapID)
			rec.Set("congregation", ao.congregation)
			rec.Set("option", ao.option)
			rec.Set("created", now)
			rec.Set("updated", now)
			if err := app.SaveNoValidate(rec); err != nil {
				return fmt.Errorf("save address_option %s: %w", ao.id, err)
			}
		}

		// -------------------------------------------------------------------
		// Messages
		// -------------------------------------------------------------------
		msgCol, err := app.FindCollectionByNameOrId("messages")
		if err != nil {
			return fmt.Errorf("find messages: %w", err)
		}
		for _, msg := range []struct {
			id, mapID, congregation, message, msgType, createdBy string
			pinned                                               bool
		}{
			{"testmsgalpha01a", "testmapalpha01a", seedAlphaCongID, "Seed test message", "publisher", "Test Publisher", false},
			{"testmsgalphapin1", "testmapalpha01a", seedAlphaCongID, "Pinned admin notice", "administrator", "admin@alpha.test", true},
			// testmapalphempt1 has an unpinned admin message — has_pinned_messages must still be false
			{"testmsgalphunp1", "testmapalphempt1", seedAlphaCongID, "Unpinned admin notice", "administrator", "admin@alpha.test", false},
		} {
			rec := core.NewRecord(msgCol)
			rec.Id = msg.id
			rec.Set("map", msg.mapID)
			rec.Set("congregation", msg.congregation)
			rec.Set("message", msg.message)
			rec.Set("type", msg.msgType)
			rec.Set("read", false)
			rec.Set("pinned", msg.pinned)
			rec.Set("created_by", msg.createdBy)
			rec.Set("created", now)
			rec.Set("updated", now)
			if err := app.SaveNoValidate(rec); err != nil {
				return fmt.Errorf("save message %s: %w", msg.id, err)
			}
		}

		// -------------------------------------------------------------------
		// Assignments (for auth / link-id integration tests)
		// -------------------------------------------------------------------
		assignCol, err := app.FindCollectionByNameOrId("assignments")
		if err != nil {
			return fmt.Errorf("find assignments: %w", err)
		}
		for _, a := range []struct {
			id, mapID, congregation, publisher, expiry string
		}{
			{"testassignalpha01", "testmapalpha01a", seedAlphaCongID, "Test Publisher Alpha", "2099-01-01 00:00:00.000Z"},
			{"testassignbeta001", "testmapbeta001a", seedBetaCongID, "Test Publisher Beta", "2099-01-01 00:00:00.000Z"},
			{"testassignexprd01", "testmapalpha01a", seedAlphaCongID, "Expired Publisher", "2000-01-01 00:00:00.000Z"},
			{"testassignrich1", "testmapalphrich1", seedAlphaCongID, "Test Publisher Rich", "2099-01-01 00:00:00.000Z"},
			{"testassignmpty1", "testmapalphempt1", seedAlphaCongID, "Test Publisher Empty", "2099-01-01 00:00:00.000Z"},
		} {
			rec := core.NewRecord(assignCol)
			rec.Id = a.id
			rec.Set("map", a.mapID)
			rec.Set("congregation", a.congregation)
			rec.Set("publisher", a.publisher)
			rec.Set("expiry_date", a.expiry)
			rec.Set("type", "publisher")
			rec.Set("created", now)
			rec.Set("updated", now)
			if err := app.SaveNoValidate(rec); err != nil {
				return fmt.Errorf("save assignment %s: %w", a.id, err)
			}
		}

		return nil
	}, func(app core.App) error {
		// DOWN: delete all seed data for both congregations.
		for _, congID := range []string{seedAlphaCongID, seedBetaCongID} {
			for _, col := range []string{"assignments", "messages", "address_options", "addresses", "maps", "territories", "roles", "options"} {
				recs, err := app.FindRecordsByFilter(col, "congregation = {:id}", "", 0, 0, dbx.Params{"id": congID})
				if err != nil {
					continue
				}
				for _, r := range recs {
					_ = app.Delete(r)
				}
			}
			if r, err := app.FindRecordById("congregations", congID); err == nil {
				_ = app.Delete(r)
			}
		}
		for _, email := range []string{
			"admin@alpha.test", "conductor@alpha.test", "readonly@alpha.test",
			"admin@beta.test", "xcong@beta.test",
		} {
			if r, err := app.FindAuthRecordByEmail("users", email); err == nil {
				_ = app.Delete(r)
			}
		}
		return nil
	})
}
