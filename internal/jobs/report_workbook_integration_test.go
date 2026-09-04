//go:build testdata

package jobs

import (
	"bytes"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/tests"
	"github.com/xuri/excelize/v2"
)

// generateAlphaWorkbook builds the on-demand report for the alpha congregation
// and opens it again with excelize so tests can inspect what an admin receives.
func generateAlphaWorkbook(t *testing.T) (*excelize.File, string) {
	t.Helper()

	app, err := tests.NewTestApp("../../test_pb_data")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)

	congregation, err := app.FindRecordById("congregations", "testcongalpha01")
	if err != nil {
		t.Fatal(err)
	}

	filename, content, err := generateReportBuffer(app, congregation, RollingDays(OnDemandReportDays))
	if err != nil {
		t.Fatal(err)
	}

	f, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		t.Fatalf("report is not a readable workbook: %v", err)
	}
	t.Cleanup(func() { f.Close() })
	return f, filename
}

func cell(t *testing.T, f *excelize.File, sheet, ref string) string {
	t.Helper()
	v, err := f.GetCellValue(sheet, ref)
	if err != nil {
		t.Fatalf("%s!%s: %v", sheet, ref, err)
	}
	return v
}

func assertCells(t *testing.T, f *excelize.File, sheet string, want map[string]string) {
	t.Helper()
	for ref, wantValue := range want {
		if got := cell(t, f, sheet, ref); got != wantValue {
			t.Errorf("%s!%s = %q; want %q", sheet, ref, got, wantValue)
		}
	}
}

func cellFill(t *testing.T, f *excelize.File, sheet, ref string) string {
	t.Helper()
	id, err := f.GetCellStyle(sheet, ref)
	if err != nil {
		t.Fatal(err)
	}
	style, err := f.GetStyle(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(style.Fill.Color) == 0 {
		return ""
	}
	return style.Fill.Color[0]
}

func rowCount(t *testing.T, f *excelize.File, sheet string) int {
	t.Helper()
	rows, err := f.GetRows(sheet)
	if err != nil {
		t.Fatal(err)
	}
	return len(rows)
}

// TestReportWorkbook_SeedData pins the workbook generated for the alpha
// congregation: sheet set, Summary tiles and table, the Addresses table, and the
// territory grids. Values come from 1780000000_seed_test_data.go. Dates and
// T02's progress are not pinned because they depend on when the seed ran.
func TestReportWorkbook_SeedData(t *testing.T) {
	f, filename := generateAlphaWorkbook(t)

	if !strings.HasPrefix(filename, "ALPHA_") || !strings.HasSuffix(filename, ".xlsx") {
		t.Errorf("filename = %q; want ALPHA_<from>_<to>.xlsx", filename)
	}

	wantSheets := []string{"Summary", "Addresses", "T01", "T02", "Setup", "Data"}
	if got := f.GetSheetList(); strings.Join(got, ",") != strings.Join(wantSheets, ",") {
		t.Fatalf("sheets = %v; want %v", got, wantSheets)
	}
	if visible, _ := f.GetSheetVisible("Data"); visible {
		t.Error("Data sheet should be hidden; it only feeds the chart")
	}
	if active := f.GetSheetName(f.GetActiveSheetIndex()); active != "Summary" {
		t.Errorf("active sheet = %q; want Summary", active)
	}

	t.Run("summary", func(t *testing.T) {
		const sheet = "Summary"
		assertCells(t, f, sheet, map[string]string{
			"B1": "Alpha Congregation",
			"B4": "ADDRESSES", "B5": "27", "B6": "9 maps in 2 territories",
			"D4": "PROGRESS",
			"F4": "DONE", "F5": "2", "F6": "7% of all addresses",
			"H4": "NOT HOME", "H5": "4",
			"J4": "DO NOT CALL", "J5": "0",
			"B8": "Territories",
			"B9": "Code", "C9": "Territory", "D9": "Progress", "F9": "Addresses", "G9": "Done", "H9": "Not home", "I9": "Invalid", "J9": "DNC", "K9": "Maps",
			// T01: 15 addresses, 1 done, 2 not home, 4 maps (A, B, SC, CF).
			"B10": "T01", "C10": "Alpha Territory 01", "D10": "0%", "F10": "15", "G10": "1", "H10": "2", "I10": "0", "J10": "0", "K10": "4",
			// T02: 12 addresses, 1 done, 2 not home, 5 maps (A, B, N, R, X).
			"B11": "T02", "C11": "Alpha Territory 02", "F11": "12", "G11": "1", "H11": "2", "I11": "0", "J11": "0", "K11": "5",
		})
		if !strings.Contains(cell(t, f, sheet, "B2"), "Territory activity report · ") {
			t.Errorf("B2 = %q; want the report subtitle with the period", cell(t, f, sheet, "B2"))
		}
		for row, sheetName := range map[int]string{10: "T01", 11: "T02"} {
			ref := "B" + string(rune('0'+row/10)) + string(rune('0'+row%10))
			ok, target, err := f.GetCellHyperLink(sheet, ref)
			if err != nil || !ok || target != "'"+sheetName+"'!A1" {
				t.Errorf("%s hyperlink = %v %q (%v); want link to %s", ref, ok, target, err, sheetName)
			}
		}
		panes, err := f.GetPanes(sheet)
		if err != nil {
			t.Fatal(err)
		}
		if !panes.Freeze || panes.YSplit != 9 {
			t.Errorf("panes = %+v; want the table header frozen", panes)
		}
		if got := cell(t, f, "Data", "A1"); got != "Done  2  (7%)" {
			t.Errorf("chart legend label = %q; want counts and share", got)
		}
	})

	t.Run("addresses", func(t *testing.T) {
		const sheet = "Addresses"
		header := []string{"Territory", "Map", "Address", "Status", "Type", "Note", "Note updated", "Note by", "Last updated", "Updated by", "DNC date", "DNC duration"}
		for i, want := range header {
			ref, _ := excelize.CoordinatesToCellName(i+1, 1)
			if got := cell(t, f, sheet, ref); got != want {
				t.Errorf("header %s = %q; want %q", ref, got, want)
			}
		}
		// 27 seeded alpha addresses, ordered by territory, map, floor desc, sequence.
		if got := rowCount(t, f, sheet); got != 28 {
			t.Errorf("rows = %d; want 28", got)
		}
		assertCells(t, f, sheet, map[string]string{
			"A2": "T01", "B2": "Blk 100A", "C2": "10", "D2": "Not done", "E2": "", "F2": "",
			"A4": "T01", "B4": "Blk 100A", "C4": "12", "D4": "Not home", "E4": "NH",
			"C6": "14", "D6": "Done",
			"A17": "T02", "B17": "Blk 200A", "C17": "30",
			"C20": "33", "D20": "Done", "E20": "DNC",
			// Multi-type map: "<floor> - <code>", localised JSON name rendered in English.
			"B27": "Rich Map", "C27": "1 - R01", "D27": "Not home", "F27": "Speaks Mandarin",
			"C28": "1 - R02", "J28": "testuseralpha01",
		})
		// Dates are real date cells, not text.
		if raw, _ := f.GetCellValue(sheet, "I2", excelize.Options{RawCellValue: true}); !strings.HasPrefix(raw, "4") {
			t.Errorf("Last updated I2 raw = %q; want an Excel date serial", raw)
		}
		if got := cell(t, f, sheet, "I2"); len(got) != len("04 Sep 2026") {
			t.Errorf("Last updated I2 = %q; want dd mmm yyyy", got)
		}
		panes, err := f.GetPanes(sheet)
		if err != nil {
			t.Fatal(err)
		}
		if !panes.Freeze || panes.YSplit != 1 {
			t.Errorf("panes = %+v; want header row frozen", panes)
		}
		tables, err := f.GetTables(sheet)
		if err != nil {
			t.Fatal(err)
		}
		if len(tables) != 1 || tables[0].Range != "A1:L28" || tables[0].StyleName != "TableStyleLight9" {
			t.Errorf("tables = %+v; want one styled table over A1:L28", tables)
		}
	})

	t.Run("territory T01", func(t *testing.T) {
		const sheet = "T01"
		assertCells(t, f, sheet, map[string]string{
			"A1": "← Summary",
			"A2": "T01 · Alpha Territory 01",
			"A3": "4 maps · 15 addresses · 0% progress",
			"B5": "Done", "E5": "Not home", "H5": "Not done", "K5": "Invalid", "N5": "Do not call",
			"B7": "Map", "J7": "Type", "L7": "Addresses", "N7": "Progress",
			// Maps ordered by sequence: A(1), B(2), SC(10), CF(11).
			"B8": "Blk 100A", "J8": "single", "L8": "5", "N8": "0%",
			"B9": "Blk 100B", "B10": "Single Code Blk", "B11": "Multi Floor Blk",
			// Grid for Blk 100A: codes across, option code in each cell.
			"A14": "Blk 100A",
			"A15": "5 addresses · 0% progress · 1 done · 2 not home · 0 invalid · 0 do not call",
			"B16": "10", "C16": "11", "D16": "12", "E16": "13", "F16": "14",
			"B17": "", "D17": "NH", "E17": "NH", "F17": "",
			"A20": "Blk 100B", "B22": "20", "F22": "24",
		})
		if ok, target, _ := f.GetCellHyperLink(sheet, "A1"); !ok || target != "Summary!A1" {
			t.Errorf("A1 hyperlink = %v %q; want Summary!A1", ok, target)
		}
		// Status is carried by the fill: not done, not home, done.
		for ref, want := range map[string]string{"B17": "F3F4F6", "D17": "FBBF24", "F17": "22C55E"} {
			if got := cellFill(t, f, sheet, ref); got != want {
				t.Errorf("%s fill = %q; want %q", ref, got, want)
			}
		}
		panes, err := f.GetPanes(sheet)
		if err != nil {
			t.Fatal(err)
		}
		if !panes.Freeze || panes.YSplit != 3 {
			t.Errorf("panes = %+v; want title rows frozen", panes)
		}
	})

	t.Run("territory T02", func(t *testing.T) {
		const sheet = "T02"
		assertCells(t, f, sheet, map[string]string{
			"A2":  "T02 · Alpha Territory 02",
			"B10": "Rich Map", "J10": "multi", "L10": "2", "N10": "100%",
			"B11": "X", "B12": "713",
			// Multi-type grid: floor labels down column A, codes across the header.
			"A27": "Rich Map",
			"A29": "Floor", "B29": "R01", "C29": "R02",
			"A30": "1", "B30": "NH", "C30": "",
			"A33": "X",
			"A34": "0 addresses · 0% progress · 0 done · 0 not home · 0 invalid · 0 do not call",
		})
		if got := cellFill(t, f, sheet, "B30"); got != "FBBF24" {
			t.Errorf("B30 fill = %q; want not-home amber", got)
		}
	})

	t.Run("setup", func(t *testing.T) {
		const sheet = "Setup"
		assertCells(t, f, sheet, map[string]string{
			"B1": "Setup", "B2": "Alpha Congregation",
			"B4": "Congregation", "B5": "Setting", "C5": "Value",
			"B6": "Expiry hours", "C6": "24", "B7": "Max tries", "C7": "3",
			"B11": "Options", "B12": "Code", "C12": "Description", "D12": "Default",
			"B13": "NH", "C13": "Not Home", "D13": "", "B15": "LN", "D15": "Yes",
			"B17": "Roles", "B18": "Name", "C18": "Role",
			"B19": "Alpha Admin", "C19": "administrator", "B21": "Alpha ReadOnly", "C21": "read_only",
		})
		rows, err := f.GetRows(sheet)
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range rows {
			for _, v := range row {
				if strings.Contains(v, "@") {
					t.Fatalf("Setup sheet contains an email address: %q", v)
				}
			}
		}
	})
}
