//go:build testdata

package jobs

import (
	"bytes"
	"regexp"
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

func rowCount(t *testing.T, f *excelize.File, sheet string) int {
	t.Helper()
	rows, err := f.GetRows(sheet)
	if err != nil {
		t.Fatal(err)
	}
	return len(rows)
}

// hasHeaderFilter reports whether the sheet has a table starting at A1, which is
// what gives the header row its filter dropdowns.
func hasHeaderFilter(t *testing.T, f *excelize.File, sheet string) bool {
	t.Helper()
	tables, err := f.GetTables(sheet)
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range tables {
		if strings.HasPrefix(table.Range, "A1:") {
			return true
		}
	}
	return false
}

// TestReportWorkbook_SeedData pins the workbook generated for the alpha
// congregation: sheet set, the Addresses table, and the territory grids. Values
// come from 1780000000_seed_test_data.go. Dates and T02's progress are not
// pinned because they depend on when the seed ran.
func TestReportWorkbook_SeedData(t *testing.T) {
	f, filename := generateAlphaWorkbook(t)

	if !strings.HasPrefix(filename, "ALPHA_") || !strings.HasSuffix(filename, ".xlsx") {
		t.Errorf("filename = %q; want ALPHA_<from>_<to>.xlsx", filename)
	}

	wantSheets := []string{"Details", "Addresses", "T01", "T02"}
	if got := f.GetSheetList(); strings.Join(got, ",") != strings.Join(wantSheets, ",") {
		t.Fatalf("sheets = %v; want %v", got, wantSheets)
	}

	t.Run("details", func(t *testing.T) {
		assertCells(t, f, "Details", map[string]string{
			"A1": "Congregation Details",
			"B2": "Alpha Congregation",
			"B3": "24",
			"B4": "3",
			"A8": "Options",
			"A9": "NH", "B9": "Not Home",
			"A10": "DNC", "B10": "Do Not Call",
			"A11": "LN", "B11": "Language Note",
			"A14": "Territory Overview",
			"A16": "T01", "B16": "Alpha Territory 01", "C16": "0.00%",
			"A17": "T02", "B17": "Alpha Territory 02",
			"A20": "Roles",
			"A22": "Alpha Admin", "B22": "admin@alpha.test", "C22": "administrator",
			"A23": "Alpha Conductor", "C23": "conductor",
			"A24": "Alpha ReadOnly", "C24": "read_only",
		})
	})

	t.Run("addresses", func(t *testing.T) {
		const sheet = "Addresses"
		header := []string{"Map", "Address", "Status", "Type", "Note", "Note Updated", "Note By", "Last Updated", "Updated By", "DNC Date", "DNC Duration"}
		for i, want := range header {
			ref, _ := excelize.CoordinatesToCellName(i+1, 1)
			if got := cell(t, f, sheet, ref); got != want {
				t.Errorf("header %s = %q; want %q", ref, got, want)
			}
		}

		// 27 seeded alpha addresses, one per row after the header, ordered by map
		// description, then floor descending, then sequence.
		if got := rowCount(t, f, sheet); got != 28 {
			t.Errorf("rows = %d; want 28", got)
		}
		assertCells(t, f, sheet, map[string]string{
			"A2": "Blk 100A", "B2": "10", "C2": "not_done", "D2": "N/A", "E2": "N/A",
			"A4": "Blk 100A", "B4": "12", "C4": "not_home", "D4": "NH",
			"A6": "Blk 100A", "B6": "14", "C6": "done",
			"A7": "Blk 100B", "B7": "20",
			"A15": "Blk 200A", "B15": "33", "C15": "done", "D15": "DNC",
			"A22": "Multi Floor Blk", "B22": "01",
			"A26": "Single Code Blk", "B26": "99",
			// Multi-type map: address is rendered as "<floor> - <code>".
			"A27": `{"en":"Rich Map","zh":"富地图"}`, "B27": "1 - R01", "C27": "not_home", "D27": "NH", "E27": "Speaks Mandarin",
			"B28": "1 - R02", "C28": "dnc", "I28": "testuseralpha01",
		})

		dateRe := regexp.MustCompile(`^\d{2}-\d{2}-\d{4}$`)
		if got := cell(t, f, sheet, "H2"); !dateRe.MatchString(got) {
			t.Errorf("Last Updated H2 = %q; want DD-MM-YYYY", got)
		}

		panes, err := f.GetPanes(sheet)
		if err != nil {
			t.Fatal(err)
		}
		if !panes.Freeze || panes.YSplit != 1 {
			t.Errorf("panes = %+v; want header row frozen", panes)
		}
		if !hasHeaderFilter(t, f, sheet) {
			t.Error("Addresses header row has no filter")
		}
	})

	t.Run("territory T01", func(t *testing.T) {
		const sheet = "T01"
		assertCells(t, f, sheet, map[string]string{
			"A1": "Territory Details: T01 - Alpha Territory 01",
			"B2": "T01",
			"B3": "Alpha Territory 01",
			"B4": "0%",
			"B5": "testterralpha01",
			"A7": "Maps Overview",
			"A8": "Name", "B8": "Type", "C8": "Progress",
			// Maps ordered by sequence: A(1), B(2), SC(10), CF(11).
			"A9": "Blk 100A", "B9": "single", "C9": "0.00%",
			"A10": "Blk 100B",
			"A11": "Single Code Blk",
			"A12": "Multi Floor Blk",
			// Single-type grid: one header row of codes, one row of status symbols.
			"A15": "Map: Blk 100A (Progress: 0%) - single",
			"A16": "10", "B16": "11", "C16": "12", "D16": "13", "E16": "14",
			"A17": "○", "B17": "○", "C17": "NH ⌂", "D17": "NH ⌂", "E17": "✓",
			"A21": "Map: Blk 100B (Progress: 0%) - single",
			"A23": "○", "E23": "○",
			"A27": "Map: Single Code Blk (Progress: 0%) - single",
			"A28": "99", "A29": "○",
			"A33": "Map: Multi Floor Blk (Progress: 0%) - single",
			"A34": "1", "B34": "2",
		})
		if got := rowCount(t, f, sheet); got != 35 {
			t.Errorf("rows = %d; want 35", got)
		}
	})

	t.Run("territory T02", func(t *testing.T) {
		const sheet = "T02"
		assertCells(t, f, sheet, map[string]string{
			"A1":  "Territory Details: T02 - Alpha Territory 02",
			"B5":  "testterralpha02",
			"A9":  "Blk 200A",
			"A10": "Blk 200B",
			"A11": `{"en":"Rich Map","zh":"富地图"}`, "B11": "multi", "C11": "100.00%",
			"A12": "", "B12": "single",
			"A13": "713", "B13": "single",
			"A16": "Map: Blk 200A (Progress: 0%) - single",
			"D18": "DNC ✓", "E18": "⌂",
			// Multi-type grid: floor labels down column A, codes across the header.
			"A28": `Map: {"en":"Rich Map","zh":"富地图"} (Progress: 100%) - multi`,
			"A29": `Floor\Code`, "B29": "R01", "C29": "R02",
			"A30": "1", "B30": "NH ⌂", "C30": "?",
			"A34": "Map:  (Progress: 0%) - single",
			"A35": "No addresses found",
			"A39": "Map: 713 (Progress: 0%) - single",
			"A40": "No addresses found",
		})

		merged, err := f.GetMergeCells(sheet)
		if err != nil {
			t.Fatal(err)
		}
		var ranges []string
		for _, m := range merged {
			ranges = append(ranges, m.GetStartAxis()+":"+m.GetEndAxis())
		}
		for _, want := range []string{"A1:F1", "A7:C7", "A28:C28"} {
			found := false
			for _, r := range ranges {
				if r == want {
					found = true
				}
			}
			if !found {
				t.Errorf("merged ranges %v missing %s", ranges, want)
			}
		}
	})
}
