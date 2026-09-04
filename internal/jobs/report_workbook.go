package jobs

import (
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/xuri/excelize/v2"
)

// Palette: one neutral family carries structure, status colours carry meaning,
// and blue is reserved for links. Progress bars reuse the "done" green.
const (
	inkColor   = "1F2937"
	mutedColor = "6B7280"
	lineColor  = "E5E7EB"
	tileColor  = "F9FAFB"
	linkColor  = "2563EB"
	doneColor  = "22C55E"
)

type statusStyle struct {
	Label string
	Fill  string
	Ink   string
}

var statusStyles = map[string]statusStyle{
	"done":        {"Done", doneColor, "FFFFFF"},
	"not_home":    {"Not home", "FBBF24", "78350F"},
	"not_done":    {"Not done", "F3F4F6", mutedColor},
	"invalid":     {"Invalid", "9CA3AF", "FFFFFF"},
	"do_not_call": {"Do not call", "EF4444", "FFFFFF"},
}

var statusOrder = []string{"done", "not_home", "not_done", "invalid", "do_not_call"}

func statusOf(status string) statusStyle {
	if s, ok := statusStyles[status]; ok {
		return s
	}
	return statusStyle{Label: status, Fill: "FFFFFF", Ink: inkColor}
}

// reportStyles holds every cell style the workbook uses, created once per file.
type reportStyles struct {
	title, subtitle                           int
	tileLabel, tileValue, tilePct, tileNote   int
	header, headerNum                         int
	row, rowMuted, rowNum, rowPct, link, back int
	axis, mapTitle, mapSub, date              int
	cell, legend                              map[string]int
}

func newReportStyles(f *excelize.File) (*reportStyles, error) {
	font := func(size float64, bold bool, color string) *excelize.Font {
		return &excelize.Font{Size: size, Bold: bold, Color: color}
	}
	fill := func(c string) excelize.Fill { return excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{c}} }
	bottom := []excelize.Border{{Type: "bottom", Color: lineColor, Style: 1}}
	white := []excelize.Border{
		{Type: "left", Color: "FFFFFF", Style: 1}, {Type: "right", Color: "FFFFFF", Style: 1},
		{Type: "top", Color: "FFFFFF", Style: 1}, {Type: "bottom", Color: "FFFFFF", Style: 1},
	}
	left := &excelize.Alignment{Horizontal: "left", Vertical: "center", Indent: 1}
	right := &excelize.Alignment{Horizontal: "right", Vertical: "center", Indent: 1}
	center := &excelize.Alignment{Horizontal: "center", Vertical: "center"}
	pct := "0%"
	date := "dd mmm yyyy"

	var err error
	add := func(s *excelize.Style) int {
		if err != nil {
			return 0
		}
		var id int
		id, err = f.NewStyle(s)
		return id
	}

	st := &reportStyles{cell: map[string]int{}, legend: map[string]int{}}
	st.title = add(&excelize.Style{Font: font(20, true, inkColor)})
	st.subtitle = add(&excelize.Style{Font: font(11, false, mutedColor)})
	st.tileLabel = add(&excelize.Style{Font: font(9, true, mutedColor), Fill: fill(tileColor), Alignment: &excelize.Alignment{Vertical: "bottom", Indent: 1}})
	st.tileValue = add(&excelize.Style{Font: font(22, true, inkColor), Fill: fill(tileColor), Alignment: left, NumFmt: 3})
	st.tilePct = add(&excelize.Style{Font: font(22, true, inkColor), Fill: fill(tileColor), Alignment: left, CustomNumFmt: &pct})
	st.tileNote = add(&excelize.Style{Font: font(9, false, mutedColor), Fill: fill(tileColor), Alignment: &excelize.Alignment{Vertical: "top", Indent: 1}})
	st.header = add(&excelize.Style{Font: font(10, true, "FFFFFF"), Fill: fill(inkColor), Alignment: left})
	st.headerNum = add(&excelize.Style{Font: font(10, true, "FFFFFF"), Fill: fill(inkColor), Alignment: right})
	st.row = add(&excelize.Style{Font: font(10, false, inkColor), Border: bottom, Alignment: left})
	st.rowMuted = add(&excelize.Style{Font: font(10, false, mutedColor), Border: bottom, Alignment: left})
	st.rowNum = add(&excelize.Style{Font: font(10, false, inkColor), Border: bottom, Alignment: right, NumFmt: 3})
	st.rowPct = add(&excelize.Style{Font: font(10, false, inkColor), Border: bottom, Alignment: right, CustomNumFmt: &pct})
	st.link = add(&excelize.Style{Font: font(10, true, linkColor), Border: bottom, Alignment: left})
	st.back = add(&excelize.Style{Font: font(10, false, linkColor)})
	st.axis = add(&excelize.Style{Font: font(9, true, mutedColor), Alignment: center})
	st.mapTitle = add(&excelize.Style{Font: font(12, true, inkColor), Alignment: &excelize.Alignment{Vertical: "center"}})
	st.mapSub = add(&excelize.Style{Font: font(9, false, mutedColor), Alignment: &excelize.Alignment{Vertical: "center"}})
	st.date = add(&excelize.Style{CustomNumFmt: &date})
	for status, s := range statusStyles {
		st.cell[status] = add(&excelize.Style{Font: font(8, false, s.Ink), Fill: fill(s.Fill), Alignment: center, Border: white})
		st.legend[status] = add(&excelize.Style{Font: font(9, false, s.Ink), Fill: fill(s.Fill), Alignment: left, Border: white})
	}
	return st, err
}

// ---------------------------------------------------------------------------
// Data
// ---------------------------------------------------------------------------

type territorySummary struct {
	Id          string `db:"id"`
	Code        string `db:"code"`
	Description string `db:"description"`
	Progress    int    `db:"progress"` // 0-100 as stored by ProcessTerritoryAggregates
	Total       int    `db:"total_addresses"`
	Done        int    `db:"done"`
	NotDone     int    `db:"not_done"`
	NotHome     int    `db:"not_home"`
	DNC         int    `db:"dnc"`
	Invalid     int    `db:"invalid"`
	Maps        int    `db:"maps"`
}

func queryTerritorySummaries(app core.App, congregationId string) ([]territorySummary, error) {
	var rows []territorySummary
	err := app.DB().NewQuery(`
		SELECT t.id, t.code, t.description, t.progress, t.total_addresses,
		       t.done, t.not_done, t.not_home, t.dnc, t.invalid,
		       (SELECT COUNT(*) FROM maps m WHERE m.territory = t.id) AS maps
		FROM analytics_territories t
		WHERE t.congregation = {:congregation}
		ORDER BY t.code
	`).Bind(dbx.Params{"congregation": congregationId}).All(&rows)
	return rows, err
}

// queryCongregationProgress mirrors ProcessTerritoryAggregates at congregation
// level: completed over total from every map's stored aggregates.
func queryCongregationProgress(app core.App, congregationId string) (float64, error) {
	var row struct {
		Completed float64 `db:"completed"`
		Total     float64 `db:"total"`
	}
	err := app.DB().NewQuery(`
		SELECT COALESCE(SUM(json_extract(aggregates, '$.completed')), 0) AS completed,
		       COALESCE(SUM(json_extract(aggregates, '$.total')), 0)     AS total
		FROM maps WHERE congregation = {:congregation}
	`).Bind(dbx.Params{"congregation": congregationId}).One(&row)
	if err != nil || row.Total == 0 {
		return 0, err
	}
	return row.Completed / row.Total, nil
}

type mapSummary struct {
	Id          string `db:"id"`
	Code        string `db:"code"`
	Description string `db:"description"`
	Type        string `db:"type"`
	Progress    int    `db:"progress"`
}

func queryTerritoryMaps(app core.App, territoryId string) ([]mapSummary, error) {
	var rows []mapSummary
	err := app.DB().NewQuery(`
		SELECT id, COALESCE(code, '') AS code, COALESCE(description, '') AS description,
		       COALESCE(type, '') AS type, COALESCE(progress, 0) AS progress
		FROM maps WHERE territory = {:territory}
		ORDER BY sequence, code
	`).Bind(dbx.Params{"territory": territoryId}).All(&rows)
	return rows, err
}

type territoryAddress struct {
	Id       string `db:"id"`
	Map      string `db:"map"`
	Code     string `db:"code"`
	Floor    int    `db:"floor"`
	Sequence int    `db:"sequence"`
	Status   string `db:"status"`
}

// fetchAddressesByMapIDs returns every address of the given maps grouped by map id.
func fetchAddressesByMapIDs(app core.App, mapIDs []string) (map[string][]territoryAddress, error) {
	grouped := make(map[string][]territoryAddress, len(mapIDs))
	if len(mapIDs) == 0 {
		return grouped, nil
	}
	ids := make([]any, len(mapIDs))
	for i, id := range mapIDs {
		ids[i] = id
	}
	var rows []territoryAddress
	err := app.DB().Select("id", "map", "code", "floor", "sequence", "status").
		From("addresses").Where(dbx.In("map", ids...)).All(&rows)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		grouped[row.Map] = append(grouped[row.Map], row)
	}
	return grouped, nil
}

// fetchTypeCodesByAddress returns each address's primary option code, the one
// with the lowest option sequence, for every address in the given maps.
func fetchTypeCodesByAddress(app core.App, mapIDs []string) (map[string]string, error) {
	types := map[string]string{}
	if len(mapIDs) == 0 {
		return types, nil
	}
	ids := make([]any, len(mapIDs))
	for i, id := range mapIDs {
		ids[i] = id
	}
	var rows []struct {
		Address string `db:"address"`
		Code    string `db:"code"`
	}
	err := app.DB().Select("ao.address", "o.code").
		From("address_options ao").InnerJoin("options o", dbx.NewExp("o.id = ao.option")).
		Where(dbx.In("ao.map", ids...)).OrderBy("o.sequence ASC").All(&rows)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if _, seen := types[row.Address]; !seen {
			types[row.Address] = row.Code
		}
	}
	return types, nil
}

// mapDisplayName renders a map or territory description, which may be a plain
// string or a JSON object of localised names, preferring English.
func mapDisplayName(description, fallback string) string {
	if strings.HasPrefix(description, "{") {
		var names map[string]string
		if json.Unmarshal([]byte(description), &names) == nil && len(names) > 0 {
			if en, ok := names["en"]; ok && en != "" {
				return en
			}
			keys := make([]string, 0, len(names))
			for k := range names {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			return names[keys[0]]
		}
	}
	if description == "" {
		return fallback
	}
	return description
}

func parsePBTime(s string) (time.Time, bool) {
	for _, layout := range []string{"2006-01-02 15:04:05.999Z", time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func humanDuration(since time.Time) string {
	days := int(time.Since(since).Hours() / 24)
	switch {
	case days >= 365:
		return pluralize(days/365, "year")
	case days >= 30:
		return pluralize(days/30, "month")
	case days > 0:
		return pluralize(days, "day")
	}
	return "Today"
}

func pluralize(count int, unit string) string {
	if count == 1 {
		return fmt.Sprintf("%d %s", count, unit)
	}
	return fmt.Sprintf("%d %ss", count, unit)
}

func cellRef(col, row int) string {
	name, _ := excelize.CoordinatesToCellName(col, row)
	return name
}

func colName(col int) string {
	name, _ := excelize.ColumnNumberToName(col)
	return name
}

// ---------------------------------------------------------------------------
// Workbook
// ---------------------------------------------------------------------------

const (
	summarySheet = "Summary"
	addressSheet = "Addresses"
	setupSheet   = "Setup"
	dataSheet    = "Data"
)

func generateReportBuffer(app core.App, congregation *core.Record, period ReportPeriod) (string, []byte, error) {
	f := excelize.NewFile()
	defer f.Close()

	st, err := newReportStyles(f)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create styles: %v", err)
	}

	territories, err := queryTerritorySummaries(app, congregation.Id)
	if err != nil {
		return "", nil, fmt.Errorf("failed to fetch territories: %v", err)
	}

	// Tab order follows creation order, and a streamed sheet cannot be moved,
	// so every sheet is created up front: Summary, Addresses, territories, Setup.
	if err := f.SetSheetName("Sheet1", summarySheet); err != nil {
		return "", nil, err
	}
	if _, err := f.NewSheet(addressSheet); err != nil {
		return "", nil, err
	}
	sheetNames := make([]string, len(territories))
	for i, t := range territories {
		sheetNames[i] = territorySheetName(f, t)
		if _, err := f.NewSheet(sheetNames[i]); err != nil {
			return "", nil, fmt.Errorf("failed to create sheet for territory %s: %v", t.Code, err)
		}
	}
	if _, err := f.NewSheet(setupSheet); err != nil {
		return "", nil, err
	}

	if err := writeSummarySheet(app, f, st, congregation, period, territories, sheetNames); err != nil {
		return "", nil, fmt.Errorf("failed to create summary sheet: %v", err)
	}
	if err := writeAddressSheet(app, f, st, congregation); err != nil {
		return "", nil, fmt.Errorf("failed to create address sheet: %v", err)
	}
	for i, t := range territories {
		if err := writeTerritorySheet(app, f, st, sheetNames[i], t); err != nil {
			log.Printf("Failed to create sheet for territory %s: %v", t.Code, err)
		}
	}
	if err := writeSetupSheet(app, f, st, congregation); err != nil {
		return "", nil, fmt.Errorf("failed to create setup sheet: %v", err)
	}

	name, _ := congregation.Get("name").(string)
	if err := f.SetDocProps(&excelize.DocProperties{
		Title:   fmt.Sprintf("%s territory activity report %s", name, period.Label),
		Creator: "Ministry Mapper",
		Created: time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		return "", nil, err
	}
	index, _ := f.GetSheetIndex(summarySheet)
	f.SetActiveSheet(index)

	buffer, err := f.WriteToBuffer()
	if err != nil {
		return "", nil, fmt.Errorf("failed to generate Excel buffer: %v", err)
	}
	log.Printf("Generated report for congregation %s", congregation.Get("code"))
	return fmt.Sprintf("%s_%s.xlsx", congregation.Get("code"), period.fileTag), buffer.Bytes(), nil
}

var sheetNameForbidden = regexp.MustCompile(`[\[\]:*?/\\]`)

// territorySheetName derives a valid, unique sheet name from the territory code.
func territorySheetName(f *excelize.File, t territorySummary) string {
	base := sheetNameForbidden.ReplaceAllString(strings.TrimSpace(t.Code), "-")
	if base == "" {
		base = t.Id[:8]
	}
	if len(base) > 31 {
		base = base[:31]
	}
	name := base
	existing := f.GetSheetList()
	for n := 2; slices.Contains(existing, name) || name == dataSheet || name == setupSheet; n++ {
		suffix := fmt.Sprintf("-%d", n)
		name = base[:min(len(base), 31-len(suffix))] + suffix
	}
	return name
}

func hideGridLines(f *excelize.File, sheet string) error {
	off := false
	return f.SetSheetView(sheet, 0, &excelize.ViewOptions{ShowGridLines: &off})
}

// progressBar adds a solid, bar-only data bar to a range of 0-1 progress values.
func progressBar(f *excelize.File, sheet, rangeRef string) error {
	return f.SetConditionalFormat(sheet, rangeRef, []excelize.ConditionalFormatOptions{{
		Type: "data_bar", Criteria: "=", MinType: "num", MinValue: "0", MaxType: "num", MaxValue: "1",
		BarColor: doneColor, BarBorderColor: doneColor, BarSolid: true, BarOnly: true,
	}})
}

// writeLegend draws the five status swatches as merged blocks of three narrow columns.
func writeLegend(f *excelize.File, st *reportStyles, sheet string, row int) {
	for i, status := range statusOrder {
		from, to := cellRef(2+i*3, row), cellRef(4+i*3, row)
		f.SetCellValue(sheet, from, statusOf(status).Label)
		f.SetCellStyle(sheet, from, to, st.legend[status])
		f.MergeCell(sheet, from, to)
	}
	f.SetRowHeight(sheet, row, 20)
}

// ---------------------------------------------------------------------------
// Summary
// ---------------------------------------------------------------------------

func writeSummarySheet(app core.App, f *excelize.File, st *reportStyles, congregation *core.Record, period ReportPeriod, territories []territorySummary, sheetNames []string) error {
	sh := summarySheet
	if err := hideGridLines(f, sh); err != nil {
		return err
	}
	for col, width := range map[string]float64{"A": 2, "B": 11, "C": 44, "D": 9, "E": 16, "L": 2} {
		f.SetColWidth(sh, col, col, width)
	}
	f.SetColWidth(sh, "F", "K", 11)

	name, _ := congregation.Get("name").(string)
	f.SetCellValue(sh, "B1", name)
	f.SetCellStyle(sh, "B1", "B1", st.title)
	f.SetRowHeight(sh, 1, 30)
	f.SetCellValue(sh, "B2", "Territory activity report · "+period.Label)
	f.SetCellStyle(sh, "B2", "B2", st.subtitle)

	totals := map[string]int{}
	total, mapCount := 0, 0
	for _, t := range territories {
		totals["done"] += t.Done
		totals["not_home"] += t.NotHome
		totals["not_done"] += t.NotDone
		totals["invalid"] += t.Invalid
		totals["do_not_call"] += t.DNC
		total += t.Total
		mapCount += t.Maps
	}
	progress, err := queryCongregationProgress(app, congregation.Id)
	if err != nil {
		return err
	}
	donePct := 0.0
	if total > 0 {
		donePct = float64(totals["done"]) / float64(total) * 100
	}

	tiles := []struct {
		label string
		value any
		note  string
		style int
	}{
		{"ADDRESSES", total, fmt.Sprintf("%d maps in %d territories", mapCount, len(territories)), st.tileValue},
		{"PROGRESS", progress, "completed share of countable addresses", st.tilePct},
		{"DONE", totals["done"], fmt.Sprintf("%.0f%% of all addresses", donePct), st.tileValue},
		{"NOT HOME", totals["not_home"], "awaiting a return visit", st.tileValue},
		{"DO NOT CALL", totals["do_not_call"], "dates on the Addresses sheet", st.tileValue},
	}
	tileCols := [][2]string{{"B", "C"}, {"D", "E"}, {"F", "G"}, {"H", "I"}, {"J", "K"}}
	for i, tile := range tiles {
		c1, c2 := tileCols[i][0], tileCols[i][1]
		for row, style := range map[int]int{4: st.tileLabel, 5: tile.style, 6: st.tileNote} {
			f.SetCellStyle(sh, c1+strconv.Itoa(row), c2+strconv.Itoa(row), style)
			f.MergeCell(sh, c1+strconv.Itoa(row), c2+strconv.Itoa(row))
		}
		f.SetCellValue(sh, c1+"4", tile.label)
		f.SetCellValue(sh, c1+"5", tile.value)
		f.SetCellValue(sh, c1+"6", tile.note)
	}
	f.SetRowHeight(sh, 4, 18)
	f.SetRowHeight(sh, 5, 34)
	f.SetRowHeight(sh, 6, 18)

	f.SetCellValue(sh, "B8", "Territories")
	f.SetCellStyle(sh, "B8", "B8", st.mapTitle)
	f.SetRowHeight(sh, 8, 26)
	const headerRow = 9
	headers := []string{"Code", "Territory", "Progress", "", "Addresses", "Done", "Not home", "Invalid", "DNC", "Maps"}
	for i, h := range headers {
		ref := cellRef(i+2, headerRow)
		f.SetCellValue(sh, ref, h)
		style := st.header
		if i >= 2 && i != 3 {
			style = st.headerNum
		}
		f.SetCellStyle(sh, ref, ref, style)
	}
	f.SetRowHeight(sh, headerRow, 24)
	for i, t := range territories {
		row := headerRow + 1 + i
		f.SetCellValue(sh, cellRef(2, row), t.Code)
		f.SetCellStyle(sh, cellRef(2, row), cellRef(2, row), st.link)
		if err := f.SetCellHyperLink(sh, cellRef(2, row), fmt.Sprintf("'%s'!A1", sheetNames[i]), "Location"); err != nil {
			return err
		}
		f.SetCellValue(sh, cellRef(3, row), mapDisplayName(t.Description, t.Code))
		f.SetCellStyle(sh, cellRef(3, row), cellRef(3, row), st.row)
		f.SetCellValue(sh, cellRef(4, row), float64(t.Progress)/100)
		f.SetCellStyle(sh, cellRef(4, row), cellRef(4, row), st.rowPct)
		f.SetCellValue(sh, cellRef(5, row), float64(t.Progress)/100) // bar-only twin of the number
		f.SetCellStyle(sh, cellRef(5, row), cellRef(5, row), st.row)
		for j, v := range []int{t.Total, t.Done, t.NotHome, t.Invalid, t.DNC, t.Maps} {
			f.SetCellValue(sh, cellRef(6+j, row), v)
			f.SetCellStyle(sh, cellRef(6+j, row), cellRef(6+j, row), st.rowNum)
		}
		f.SetRowHeight(sh, row, 20)
	}
	if len(territories) > 0 {
		if err := progressBar(f, sh, fmt.Sprintf("E%d:E%d", headerRow+1, headerRow+len(territories))); err != nil {
			return err
		}
	}
	if err := f.SetPanes(sh, &excelize.Panes{Freeze: true, YSplit: headerRow, TopLeftCell: cellRef(1, headerRow+1), ActivePane: "bottomLeft"}); err != nil {
		return err
	}

	// Status doughnut, fed from a hidden sheet so the legend can carry the counts.
	if _, err := f.NewSheet(dataSheet); err != nil {
		return err
	}
	points := make([]excelize.ChartDataPoint, len(statusOrder))
	for i, status := range statusOrder {
		share := 0.0
		if total > 0 {
			share = float64(totals[status]) / float64(total) * 100
		}
		f.SetCellValue(dataSheet, cellRef(1, i+1), fmt.Sprintf("%s  %d  (%.0f%%)", statusOf(status).Label, totals[status], share))
		f.SetCellValue(dataSheet, cellRef(2, i+1), totals[status])
		points[i] = excelize.ChartDataPoint{Index: i, Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{statusOf(status).Fill}}}
	}
	if err := f.SetSheetVisible(dataSheet, false); err != nil {
		return err
	}
	last := len(statusOrder)
	return f.AddChart(sh, "M1", &excelize.Chart{
		Type:      excelize.Doughnut,
		Dimension: excelize.ChartDimension{Width: 420, Height: 230},
		Series: []excelize.ChartSeries{{
			Name:       "Addresses",
			Categories: fmt.Sprintf("%s!$A$1:$A$%d", dataSheet, last),
			Values:     fmt.Sprintf("%s!$B$1:$B$%d", dataSheet, last),
			DataPoint:  points,
		}},
		Title:  excelize.ChartTitle{Paragraph: []excelize.RichTextRun{{Text: "Address status", Font: &excelize.Font{Size: 12, Bold: true, Color: inkColor}}}},
		Legend: excelize.ChartLegend{Position: "right"},
	})
}

// ---------------------------------------------------------------------------
// Addresses
// ---------------------------------------------------------------------------

// writeAddressSheet streams the flat address list, which reaches 116k rows for
// the largest congregation; the regular cell API would hold every cell in memory
// until WriteToBuffer. StreamWriter needs column widths and panes set before the
// first row and has no AutoFilter, so the header filter comes from a table.
func writeAddressSheet(app core.App, f *excelize.File, st *reportStyles, congregation *core.Record) error {
	var rows []struct {
		Territory          string `db:"territory"`
		MapDescription     string `db:"map_description"`
		MapCode            string `db:"map_code"`
		MapType            string `db:"map_type"`
		Floor              int    `db:"floor"`
		Code               string `db:"code"`
		Status             string `db:"status"`
		TypeCodes          string `db:"type_codes"`
		Notes              string `db:"notes"`
		LastNotesUpdated   string `db:"last_notes_updated"`
		LastNotesUpdatedBy string `db:"last_notes_updated_by"`
		Updated            string `db:"updated"`
		UpdatedBy          string `db:"updated_by"`
		DncTime            string `db:"dnc_time"`
	}
	err := app.DB().Select(
		"COALESCE(t.code, '') AS territory",
		"COALESCE(m.description, '') AS map_description",
		"COALESCE(m.code, '') AS map_code",
		"COALESCE(m.type, '') AS map_type",
		"a.floor", "a.code", "a.status",
		`COALESCE((SELECT GROUP_CONCAT(o.code, ', ') FROM address_options ao
		           JOIN options o ON ao.option = o.id WHERE ao.address = a.id), '') AS type_codes`,
		"a.notes", "a.last_notes_updated", "a.last_notes_updated_by", "a.updated", "a.updated_by", "a.dnc_time",
	).
		From("addresses a").
		InnerJoin("maps m", dbx.NewExp("m.id = a.map")).
		LeftJoin("territories t", dbx.NewExp("t.id = a.territory")).
		Where(dbx.HashExp{"a.congregation": congregation.Id}).
		OrderBy("t.code", "m.description", "a.floor DESC", "a.sequence").
		All(&rows)
	if err != nil {
		return fmt.Errorf("failed to fetch addresses: %v", err)
	}
	if len(rows) == 0 {
		f.SetCellValue(addressSheet, "A1", "No addresses found")
		return nil
	}

	sw, err := f.NewStreamWriter(addressSheet)
	if err != nil {
		return err
	}
	for i, width := range []float64{10, 34, 12, 12, 10, 40, 14, 18, 14, 18, 14, 12} {
		if err := sw.SetColWidth(i+1, i+1, width); err != nil {
			return err
		}
	}
	if err := sw.SetPanes(&excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"}); err != nil {
		return err
	}
	header := []interface{}{"Territory", "Map", "Address", "Status", "Type", "Note", "Note updated", "Note by", "Last updated", "Updated by", "DNC date", "DNC duration"}
	if err := sw.SetRow("A1", header, excelize.RowOpts{Height: 22}); err != nil {
		return err
	}
	dateCell := func(s string) interface{} {
		if t, ok := parsePBTime(s); ok {
			return excelize.Cell{StyleID: st.date, Value: t}
		}
		return nil
	}
	for i, r := range rows {
		var dncDate interface{}
		dncDuration := ""
		if r.Status == "do_not_call" {
			if t, ok := parsePBTime(defaultIfEmpty(r.DncTime, r.Updated)); ok {
				dncDate = excelize.Cell{StyleID: st.date, Value: t}
				dncDuration = humanDuration(t)
			}
		}
		cells := []interface{}{
			r.Territory, mapDisplayName(r.MapDescription, r.MapCode), formatAddress(r.MapType, r.Code, r.Floor), statusOf(r.Status).Label,
			r.TypeCodes, r.Notes, dateCell(r.LastNotesUpdated), r.LastNotesUpdatedBy, dateCell(r.Updated), r.UpdatedBy, dncDate, dncDuration,
		}
		if err := sw.SetRow(cellRef(1, i+2), cells); err != nil {
			return err
		}
	}
	if err := sw.AddTable(&excelize.Table{Range: fmt.Sprintf("A1:L%d", len(rows)+1), Name: "AddressList", StyleName: "TableStyleLight9"}); err != nil {
		return err
	}
	return sw.Flush()
}

func defaultIfEmpty(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func formatAddress(mapType, code string, floor int) string {
	code = defaultIfEmpty(code, "N/A")
	if mapType == "single" {
		return code
	}
	return fmt.Sprintf("%d - %s", floor, code)
}

// ---------------------------------------------------------------------------
// Territory sheets
// ---------------------------------------------------------------------------

func writeTerritorySheet(app core.App, f *excelize.File, st *reportStyles, sh string, t territorySummary) error {
	if err := hideGridLines(f, sh); err != nil {
		return err
	}
	maps, err := queryTerritoryMaps(app, t.Id)
	if err != nil {
		return err
	}
	mapIDs := make([]string, len(maps))
	for i, m := range maps {
		mapIDs[i] = m.Id
	}
	addressesByMap, err := fetchAddressesByMapIDs(app, mapIDs)
	if err != nil {
		return err
	}
	typesByAddress, err := fetchTypeCodesByAddress(app, mapIDs)
	if err != nil {
		return err
	}

	// The widest map decides how many narrow grid columns the sheet has; the
	// map overview above the grids is merged across those same columns.
	gridCols := 18
	for _, m := range maps {
		codes := map[string]bool{}
		for _, a := range addressesByMap[m.Id] {
			codes[a.Code] = true
		}
		gridCols = max(gridCols, len(codes))
	}
	f.SetColWidth(sh, "A", "A", 7)
	f.SetColWidth(sh, "B", colName(gridCols+1), 5)

	f.SetCellValue(sh, "A1", "← Summary")
	f.SetCellStyle(sh, "A1", "A1", st.back)
	if err := f.SetCellHyperLink(sh, "A1", summarySheet+"!A1", "Location"); err != nil {
		return err
	}
	f.SetCellValue(sh, "A2", fmt.Sprintf("%s · %s", t.Code, mapDisplayName(t.Description, t.Code)))
	f.SetCellStyle(sh, "A2", "A2", st.title)
	f.SetRowHeight(sh, 2, 30)
	f.SetCellValue(sh, "A3", fmt.Sprintf("%d maps · %d addresses · %d%% progress", len(maps), t.Total, t.Progress))
	f.SetCellStyle(sh, "A3", "A3", st.subtitle)

	writeLegend(f, st, sh, 5)

	row := 7
	blocks := [][2]int{{2, 9}, {10, 11}, {12, 13}, {14, 15}, {16, 19}}
	for i, h := range []string{"Map", "Type", "Addresses", "Progress", ""} {
		style := st.header
		if i >= 2 {
			style = st.headerNum
		}
		f.SetCellValue(sh, cellRef(blocks[i][0], row), h)
		f.SetCellStyle(sh, cellRef(blocks[i][0], row), cellRef(blocks[i][1], row), style)
		f.MergeCell(sh, cellRef(blocks[i][0], row), cellRef(blocks[i][1], row))
	}
	f.SetRowHeight(sh, row, 22)
	row++
	firstMapRow := row
	for _, m := range maps {
		progress := float64(m.Progress) / 100
		values := []interface{}{mapDisplayName(m.Description, m.Code), m.Type, len(addressesByMap[m.Id]), progress, progress}
		styles := []int{st.row, st.rowMuted, st.rowNum, st.rowPct, st.row}
		for i, v := range values {
			f.SetCellValue(sh, cellRef(blocks[i][0], row), v)
			f.SetCellStyle(sh, cellRef(blocks[i][0], row), cellRef(blocks[i][1], row), styles[i])
			f.MergeCell(sh, cellRef(blocks[i][0], row), cellRef(blocks[i][1], row))
		}
		f.SetRowHeight(sh, row, 20)
		row++
	}
	if len(maps) > 0 {
		if err := progressBar(f, sh, fmt.Sprintf("%s:%s", cellRef(16, firstMapRow), cellRef(16, row-1))); err != nil {
			return err
		}
	}
	row += 2

	for _, m := range maps {
		row = writeMapGrid(f, st, sh, row, m, addressesByMap[m.Id], typesByAddress)
	}
	return f.SetPanes(sh, &excelize.Panes{Freeze: true, YSplit: 3, TopLeftCell: "A4", ActivePane: "bottomLeft"})
}

// writeMapGrid draws one map as a floor-by-code grid: each cell is filled with
// its status colour and shows the address's primary option code. Returns the
// next free row.
func writeMapGrid(f *excelize.File, st *reportStyles, sh string, row int, m mapSummary, addresses []territoryAddress, typesByAddress map[string]string) int {
	counts := map[string]int{}
	codeSeq := map[string]int{}
	floors := map[int]bool{}
	grid := map[int]map[string]territoryAddress{}
	for _, a := range addresses {
		counts[a.Status]++
		if seq, ok := codeSeq[a.Code]; !ok || a.Sequence < seq {
			codeSeq[a.Code] = a.Sequence
		}
		floors[a.Floor] = true
		if grid[a.Floor] == nil {
			grid[a.Floor] = map[string]territoryAddress{}
		}
		grid[a.Floor][a.Code] = a
	}
	codes := make([]string, 0, len(codeSeq))
	for c := range codeSeq {
		codes = append(codes, c)
	}
	sort.Slice(codes, func(i, j int) bool {
		if codeSeq[codes[i]] != codeSeq[codes[j]] {
			return codeSeq[codes[i]] < codeSeq[codes[j]]
		}
		return codes[i] < codes[j]
	})
	floorList := make([]int, 0, len(floors))
	for fl := range floors {
		floorList = append(floorList, fl)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(floorList)))
	single := m.Type == "single"

	f.SetCellValue(sh, cellRef(1, row), mapDisplayName(m.Description, m.Code))
	f.SetCellStyle(sh, cellRef(1, row), cellRef(1, row), st.mapTitle)
	f.SetRowHeight(sh, row, 24)
	row++
	f.SetCellValue(sh, cellRef(1, row), fmt.Sprintf("%d addresses · %d%% progress · %d done · %d not home · %d invalid · %d do not call",
		len(addresses), m.Progress, counts["done"], counts["not_home"], counts["invalid"], counts["do_not_call"]))
	f.SetCellStyle(sh, cellRef(1, row), cellRef(1, row), st.mapSub)
	row++
	if len(addresses) == 0 {
		return row + 2
	}

	if !single {
		f.SetCellValue(sh, cellRef(1, row), "Floor")
	}
	f.SetCellStyle(sh, cellRef(1, row), cellRef(1, row), st.axis)
	for i, code := range codes {
		ref := cellRef(i+2, row)
		if n, err := strconv.Atoi(code); err == nil {
			f.SetCellValue(sh, ref, n)
		} else {
			f.SetCellValue(sh, ref, code)
		}
		f.SetCellStyle(sh, ref, ref, st.axis)
	}
	f.SetRowHeight(sh, row, 18)
	row++
	for _, floor := range floorList {
		if !single {
			f.SetCellValue(sh, cellRef(1, row), floor)
			f.SetCellStyle(sh, cellRef(1, row), cellRef(1, row), st.axis)
		}
		for i, code := range codes {
			a, ok := grid[floor][code]
			if !ok {
				continue
			}
			ref := cellRef(i+2, row)
			f.SetCellValue(sh, ref, typesByAddress[a.Id])
			f.SetCellStyle(sh, ref, ref, st.cell[a.Status])
		}
		f.SetRowHeight(sh, row, 20)
		row++
	}
	return row + 2
}

// ---------------------------------------------------------------------------
// Setup
// ---------------------------------------------------------------------------

func writeSetupSheet(app core.App, f *excelize.File, st *reportStyles, congregation *core.Record) error {
	sh := setupSheet
	if err := hideGridLines(f, sh); err != nil {
		return err
	}
	f.SetColWidth(sh, "A", "A", 2)
	f.SetColWidth(sh, "B", "B", 22)
	f.SetColWidth(sh, "C", "C", 40)
	f.SetColWidth(sh, "D", "D", 16)

	name, _ := congregation.Get("name").(string)
	f.SetCellValue(sh, "B1", "Setup")
	f.SetCellStyle(sh, "B1", "B1", st.title)
	f.SetRowHeight(sh, 1, 30)
	f.SetCellValue(sh, "B2", name)
	f.SetCellStyle(sh, "B2", "B2", st.subtitle)

	row := 4
	section := func(title string, headers ...string) {
		f.SetCellValue(sh, cellRef(2, row), title)
		f.SetCellStyle(sh, cellRef(2, row), cellRef(2, row), st.mapTitle)
		f.SetRowHeight(sh, row, 26)
		row++
		for i, h := range headers {
			f.SetCellValue(sh, cellRef(2+i, row), h)
			f.SetCellStyle(sh, cellRef(2+i, row), cellRef(2+i, row), st.header)
		}
		f.SetRowHeight(sh, row, 22)
		row++
	}
	line := func(values ...interface{}) {
		for i, v := range values {
			f.SetCellValue(sh, cellRef(2+i, row), v)
			f.SetCellStyle(sh, cellRef(2+i, row), cellRef(2+i, row), map[bool]int{true: st.rowMuted, false: st.row}[i == 0])
		}
		f.SetRowHeight(sh, row, 20)
		row++
	}

	section("Congregation", "Setting", "Value")
	line("Expiry hours", congregation.Get("expiry_hours"))
	line("Max tries", congregation.Get("max_tries"))
	line("Origin", congregation.Get("origin"))
	line("Timezone", congregation.Get("timezone"))
	row++

	var options []struct {
		Code        string `db:"code"`
		Description string `db:"description"`
		IsDefault   bool   `db:"is_default"`
	}
	err := app.DB().NewQuery(`SELECT code, description, is_default FROM options WHERE congregation = {:c} ORDER BY sequence`).
		Bind(dbx.Params{"c": congregation.Id}).All(&options)
	if err != nil {
		return fmt.Errorf("failed to fetch options: %v", err)
	}
	section("Options", "Code", "Description", "Default")
	for _, o := range options {
		def := ""
		if o.IsDefault {
			def = "Yes"
		}
		line(o.Code, o.Description, def)
	}
	row++

	// Names only: the workbook is forwarded, so personal emails stay out of it.
	var roles []struct {
		Name string `db:"name"`
		Role string `db:"role"`
	}
	err = app.DB().NewQuery(`
		SELECT COALESCE(u.name, '') AS name, r.role
		FROM roles r JOIN users u ON u.id = r.user
		WHERE r.congregation = {:c}
		ORDER BY r.role, r.created
	`).Bind(dbx.Params{"c": congregation.Id}).All(&roles)
	if err != nil {
		return fmt.Errorf("failed to fetch roles: %v", err)
	}
	section("Roles", "Name", "Role")
	for _, r := range roles {
		line(r.Name, r.Role)
	}
	return nil
}
