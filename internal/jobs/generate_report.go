package jobs

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"text/template"
	"time"

	"github.com/mailersend/mailersend-go"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/xuri/excelize/v2"
)

type ReportTemplateData struct {
	CongregationName string
	CongregationCode string
	ReportDate       string
	FileName         string
}

type ReportRecipient struct {
	Name  string `db:"name"`
	Email string `db:"email"`
}

func GenerateMonthlyReport(app *pocketbase.PocketBase) {
	log.Println("Starting monthly report generation...")

	congregations, err := app.FindRecordsByFilter("congregations", "", "", 0, 0)
	if err != nil {
		log.Printf("Failed to fetch congregations: %v", err)
		return
	}

	for _, congregation := range congregations {
		if err := generateAndSendCongregationReport(app, congregation); err != nil {
			log.Printf("Failed to generate and send report for congregation %s: %v", congregation.Get("code"), err)
			continue
		}
	}

	log.Println("Monthly report generation completed.")
}

func generateAndSendCongregationReport(app *pocketbase.PocketBase, congregation *core.Record) error {
	f := excelize.NewFile()

	congregationOptions, err := app.FindRecordsByFilter(
		"options",
		"congregation = {:congregation}",
		"sequence",
		0,
		0,
		dbx.Params{"congregation": congregation.Id},
	)
	if err != nil {
		return fmt.Errorf("failed to fetch congregation options: %v", err)
	}

	if err := createDetailsSheet(app, f, congregation, congregationOptions); err != nil {
		return fmt.Errorf("failed to create details sheet: %v", err)
	}

	if err := createDNCSheet(app, f, congregation); err != nil {
		return fmt.Errorf("failed to create DNC sheet: %v", err)
	}

	territories, err := app.FindRecordsByFilter(
		"territories",
		"congregation = {:congregation}",
		"code",
		0,
		0,
		dbx.Params{"congregation": congregation.Id},
	)
	if err != nil {
		return fmt.Errorf("failed to fetch territories: %v", err)
	}

	for _, territory := range territories {
		territoryCode := territory.Get("code")
		if err := createTerritorySheet(app, f, territory, congregationOptions); err != nil {
			log.Printf("Failed to create sheet for territory %s: %v", territoryCode, err)
			continue
		}
	}

	currentTime := time.Now()
	filename := fmt.Sprintf("%s_%s_%s.xlsx", congregation.Get("code"), currentTime.Format("01"), currentTime.Format("06"))

	// Generate Excel file in memory
	buffer, err := f.WriteToBuffer()
	if err != nil {
		return fmt.Errorf("failed to generate Excel buffer: %v", err)
	}

	log.Printf("Generated report for congregation %s", congregation.Get("code"))

	// Send email with the report attached
	if err := sendReportEmailFromBuffer(app, congregation, filename, buffer.Bytes()); err != nil {
		return fmt.Errorf("failed to send email for congregation %s: %v", congregation.Get("code"), err)
	}

	return nil
}

func getMainHeaderStyle(f *excelize.File) (int, error) {
	return f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Size: 16, Color: "FFFFFF", Family: "Calibri"},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"1F4E79"}, Pattern: 1},
		Alignment: &excelize.Alignment{
			Horizontal: "left",
			Vertical:   "center",
			Indent:     1,
		},
		Border: []excelize.Border{
			{Type: "left", Color: "1F4E79", Style: 2},
			{Type: "top", Color: "1F4E79", Style: 2},
			{Type: "bottom", Color: "1F4E79", Style: 2},
			{Type: "right", Color: "1F4E79", Style: 2},
		},
	})
}

func getSectionHeaderStyle(f *excelize.File) (int, error) {
	return f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Size: 14, Color: "FFFFFF", Family: "Calibri"},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"4A90B8"}, Pattern: 1},
		Alignment: &excelize.Alignment{
			Horizontal: "left",
			Vertical:   "center",
			Indent:     1,
		},
		Border: []excelize.Border{
			{Type: "left", Color: "4A90B8", Style: 1},
			{Type: "top", Color: "4A90B8", Style: 1},
			{Type: "bottom", Color: "4A90B8", Style: 1},
			{Type: "right", Color: "4A90B8", Style: 1},
		},
	})
}

func getTableHeaderStyle(f *excelize.File) (int, error) {
	return f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Size: 12, Color: "FFFFFF", Family: "Calibri"},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"2E75B6"}, Pattern: 1},
		Alignment: &excelize.Alignment{
			Horizontal: "center",
			Vertical:   "center",
		},
		Border: []excelize.Border{
			{Type: "left", Color: "1F4E79", Style: 2},
			{Type: "top", Color: "1F4E79", Style: 2},
			{Type: "bottom", Color: "1F4E79", Style: 2},
			{Type: "right", Color: "1F4E79", Style: 2},
		},
	})
}

func getDataCellStyle(f *excelize.File, alternate bool) (int, error) {
	color := "FFFFFF"
	if alternate {
		color = "F1F3F4"
	}
	return f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Size: 10, Color: "000000"},
		Fill: excelize.Fill{Type: "pattern", Color: []string{color}, Pattern: 1},
		Alignment: &excelize.Alignment{
			Horizontal: "left",
			Vertical:   "center",
		},
		Border: []excelize.Border{
			{Type: "left", Color: "8EAADB", Style: 1},
			{Type: "top", Color: "8EAADB", Style: 1},
			{Type: "bottom", Color: "8EAADB", Style: 1},
			{Type: "right", Color: "8EAADB", Style: 1},
		},
	})
}

func getPercentageCellStyle(f *excelize.File, alternate bool) (int, error) {
	color := "FFFFFF"
	if alternate {
		color = "F1F3F4"
	}
	return f.NewStyle(&excelize.Style{
		NumFmt:    10,
		Font:      &excelize.Font{Size: 10, Color: "333333", Family: "Calibri"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{color}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		Border: []excelize.Border{
			{Type: "left", Color: "8EAADB", Style: 1},
			{Type: "top", Color: "8EAADB", Style: 1},
			{Type: "bottom", Color: "8EAADB", Style: 1},
			{Type: "right", Color: "8EAADB", Style: 1},
		},
	})
}

func parseProgressValue(value interface{}) (float64, bool) {
	if value == nil {
		return 0, false
	}

	switch v := value.(type) {
	case float64:
		return v / 100, true
	case int:
		return float64(v) / 100, true
	case string:
		if num, err := strconv.ParseFloat(v, 64); err == nil {
			return num / 100, true
		}
	}
	return 0, false
}

func createDetailsSheet(app *pocketbase.PocketBase, f *excelize.File, congregation *core.Record, options []*core.Record) error {
	f.SetSheetName("Sheet1", "Details")
	sheet := "Details"

	headers := [][]interface{}{
		{"Congregation Details", ""},
		{"Name", congregation.Get("name")},
		{"Expiry Hours", congregation.Get("expiry_hours")},
		{"Max Tries", congregation.Get("max_tries")},
		{"Origin", congregation.Get("origin")},
		{"Timezone", congregation.Get("timezone")},
		{"", ""},
		{"Options", ""},
	}

	for i, row := range headers {
		for j, cell := range row {
			cellName, _ := excelize.CoordinatesToCellName(j+1, i+1)
			f.SetCellValue(sheet, cellName, cell)
		}
	}

	mainHeaderStyle, _ := getMainHeaderStyle(f)
	sectionHeaderStyle, _ := getSectionHeaderStyle(f)

	detailLabelStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 11, Color: "1F4E79", Family: "Calibri"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"E8F3FF"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "left", Vertical: "center", Indent: 1},
		Border: []excelize.Border{
			{Type: "left", Color: "4A90B8", Style: 2},
			{Type: "top", Color: "4A90B8", Style: 2},
			{Type: "bottom", Color: "4A90B8", Style: 2},
			{Type: "right", Color: "4A90B8", Style: 2},
		},
	})

	detailValueStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Size: 11, Color: "333333", Family: "Calibri"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"FFFFFF"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "left", Vertical: "center", Indent: 1},
		Border: []excelize.Border{
			{Type: "left", Color: "8EAADB", Style: 1},
			{Type: "top", Color: "8EAADB", Style: 1},
			{Type: "bottom", Color: "8EAADB", Style: 1},
			{Type: "right", Color: "8EAADB", Style: 1},
		},
	})

	f.SetCellStyle(sheet, "A1", "B1", mainHeaderStyle)
	f.SetCellStyle(sheet, "A8", "B8", sectionHeaderStyle)

	// Apply styling to congregation details
	f.SetCellStyle(sheet, "A2", "A6", detailLabelStyle)
	f.SetCellStyle(sheet, "B2", "B6", detailValueStyle)

	row := 9
	for i, option := range options {
		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), option.Get("code"))
		f.SetCellValue(sheet, fmt.Sprintf("B%d", row), option.Get("description"))

		optionRowStyleID, _ := getDataCellStyle(f, i%2 == 0)
		f.SetCellStyle(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("B%d", row), optionRowStyleID)
		row++
	}

	row += 2 // Two empty rows for better spacing
	f.SetCellValue(sheet, fmt.Sprintf("A%d", row), "Territory Overview")
	f.MergeCell(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("C%d", row))
	f.SetCellStyle(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("C%d", row), sectionHeaderStyle)
	f.SetRowHeight(sheet, row, 30)
	row++

	territories, err := app.FindRecordsByFilter(
		"territories",
		"congregation = {:congregation}",
		"code", // Sort by territory code
		0,
		0,
		dbx.Params{"congregation": congregation.Id},
	)
	if err != nil {
		return fmt.Errorf("failed to fetch territories: %v", err)
	}

	if len(territories) == 0 {
		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), "No territories found")
		row++
	} else {
		// Add territory headers - only territory information
		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), "Territory Code")
		f.SetCellValue(sheet, fmt.Sprintf("B%d", row), "Territory Description")
		f.SetCellValue(sheet, fmt.Sprintf("C%d", row), "Territory Progress")

		territoryHeaderStyle, _ := getTableHeaderStyle(f)
		f.SetCellStyle(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("C%d", row), territoryHeaderStyle)
		f.SetRowHeight(sheet, row, 28)
		row++

		// Add each territory
		for i, territory := range territories {
			territoryCode := fmt.Sprintf("%v", territory.Get("code"))
			if territoryCode == "<nil>" {
				territoryCode = ""
			}

			territoryDescription := fmt.Sprintf("%v", territory.Get("description"))
			if territoryDescription == "<nil>" {
				territoryDescription = ""
			}

			f.SetCellValue(sheet, fmt.Sprintf("A%d", row), territoryCode)
			f.SetCellValue(sheet, fmt.Sprintf("B%d", row), territoryDescription)

			if progressNum, isValid := parseProgressValue(territory.Get("progress")); isValid {
				f.SetCellValue(sheet, fmt.Sprintf("C%d", row), progressNum)
			} else {
				f.SetCellValue(sheet, fmt.Sprintf("C%d", row), "N/A")
			}

			territoryRowStyleID, _ := getDataCellStyle(f, i%2 == 0)
			f.SetCellStyle(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("B%d", row), territoryRowStyleID)

			percentageStyle, _ := getPercentageCellStyle(f, i%2 == 0)
			f.SetCellStyle(sheet, fmt.Sprintf("C%d", row), fmt.Sprintf("C%d", row), percentageStyle)

			f.SetRowHeight(sheet, row, 25)
			row++
		}
	}

	// Add spacing and roles section with enhanced styling
	row += 2 // Two empty rows for better spacing
	f.SetCellValue(sheet, fmt.Sprintf("A%d", row), "Roles")
	f.MergeCell(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("C%d", row))
	f.SetCellStyle(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("C%d", row), sectionHeaderStyle)
	f.SetRowHeight(sheet, row, 30)
	row++

	// Get roles for this congregation with user information
	roles, err := app.FindRecordsByFilter(
		"roles",
		"congregation = {:congregation}",
		"role, created", // Sort by role and creation date
		0,
		0,
		dbx.Params{"congregation": congregation.Id},
	)
	if err != nil {
		return fmt.Errorf("failed to fetch roles: %v", err)
	}

	if len(roles) == 0 {
		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), "No roles found")
		row++
	} else {
		// Add role headers
		f.SetCellValue(sheet, fmt.Sprintf("A%d", row), "User Name")
		f.SetCellValue(sheet, fmt.Sprintf("B%d", row), "Email")
		f.SetCellValue(sheet, fmt.Sprintf("C%d", row), "Role")

		roleHeaderStyle, _ := getTableHeaderStyle(f)
		f.SetCellStyle(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("C%d", row), roleHeaderStyle)
		f.SetRowHeight(sheet, row, 28)
		row++

		// Add each role with user information
		for _, role := range roles {
			userId := role.Get("user")
			if userId == nil {
				continue // Skip roles without users
			}

			// Get user information
			user, err := app.FindRecordById("users", fmt.Sprintf("%v", userId))
			if err != nil {
				log.Printf("Failed to fetch user %v: %v", userId, err)
				continue
			}

			// Get user name (prefer name, fallback to username)
			userName := fmt.Sprintf("%v", user.Get("name"))
			if userName == "" || userName == "<nil>" {
				userName = fmt.Sprintf("%v", user.Get("username"))
			}

			// Get user email
			userEmail := fmt.Sprintf("%v", user.Get("email"))
			if userEmail == "<nil>" {
				userEmail = ""
			}

			roleValue := fmt.Sprintf("%v", role.Get("role"))

			f.SetCellValue(sheet, fmt.Sprintf("A%d", row), userName)
			f.SetCellValue(sheet, fmt.Sprintf("B%d", row), userEmail)
			f.SetCellValue(sheet, fmt.Sprintf("C%d", row), roleValue)

			roleRowStyleID, _ := getDataCellStyle(f, (row-1)%2 == 0)
			f.SetCellStyle(sheet, fmt.Sprintf("A%d", row), fmt.Sprintf("C%d", row), roleRowStyleID)
			f.SetRowHeight(sheet, row, 25)
			row++
		}
	}

	f.SetColWidth(sheet, "A", "A", 22)
	f.SetColWidth(sheet, "B", "B", 45)
	f.SetColWidth(sheet, "C", "C", 18)

	for i := 2; i <= 7; i++ {
		f.SetRowHeight(sheet, i, 28)
	}
	f.SetRowHeight(sheet, 1, 40)
	f.SetRowHeight(sheet, 8, 35)

	return nil
}

func createDNCSheet(app *pocketbase.PocketBase, f *excelize.File, congregation *core.Record) error {
	sheetName := "DNC"
	index, err := f.NewSheet(sheetName)
	if err != nil {
		return fmt.Errorf("failed to create DNC sheet: %v", err)
	}
	f.SetActiveSheet(index)

	mainHeaderStyle, _ := getMainHeaderStyle(f)
	tableHeaderStyle, _ := getTableHeaderStyle(f)

	f.SetCellValue(sheetName, "A1", "Do Not Call Addresses")
	f.MergeCell(sheetName, "A1", "E1")
	f.SetCellStyle(sheetName, "A1", "E1", mainHeaderStyle)
	f.SetRowHeight(sheetName, 1, 40)

	dncAddresses := []struct {
		MapDescription string `db:"map_description"`
		MapType        string `db:"map_type"`
		Floor          int    `db:"floor"`
		Code           string `db:"code"`
		Notes          string `db:"notes"`
		DncTime        string `db:"dnc_time"`
		Updated        string `db:"updated"`
	}{}

	err = app.DB().
		Select(
			"maps.description as map_description",
			"maps.type as map_type",
			"addresses.floor as floor",
			"addresses.code as code",
			"addresses.notes as notes",
			"addresses.dnc_time as dnc_time",
			"addresses.updated as updated",
		).
		From("addresses").
		InnerJoin("maps", dbx.NewExp("maps.id = addresses.map")).
		Where(dbx.HashExp{
			"addresses.congregation": congregation.Id,
			"addresses.status":       "do_not_call",
		}).
		OrderBy("maps.description", "addresses.floor DESC").
		All(&dncAddresses)

	if err != nil {
		return fmt.Errorf("failed to fetch do not call addresses: %v", err)
	}

	row := 3
	if len(dncAddresses) == 0 {
		f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), "No do not call addresses found")
		return nil
	}

	f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), "Map")
	f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), "Address")
	f.SetCellValue(sheetName, fmt.Sprintf("C%d", row), "Note")
	f.SetCellValue(sheetName, fmt.Sprintf("D%d", row), "Date")
	f.SetCellValue(sheetName, fmt.Sprintf("E%d", row), "Duration")
	f.SetCellStyle(sheetName, fmt.Sprintf("A%d", row), fmt.Sprintf("E%d", row), tableHeaderStyle)
	f.SetRowHeight(sheetName, row, 28)
	row++

	for i, addr := range dncAddresses {
		mapDesc := addr.MapDescription
		if mapDesc == "" {
			mapDesc = "N/A"
		}

		var address string
		if addr.MapType == "single" {
			address = addr.Code
			if address == "" {
				address = "N/A"
			}
		} else {
			code := addr.Code
			if code == "" {
				code = "N/A"
			}
			address = fmt.Sprintf("%d - %s", addr.Floor, code)
		}

		notes := addr.Notes
		if notes == "" {
			notes = "N/A"
		}

		timeStr := addr.DncTime
		if timeStr == "" {
			timeStr = addr.Updated
		}

		var dncDate, duration string
		if timeStr != "" {
			dncTimeParsed, err := time.Parse("2006-01-02 15:04:05.999Z", timeStr)
			if err != nil {
				dncTimeParsed, err = time.Parse(time.RFC3339, timeStr)
			}

			if err == nil {
				dncDate = dncTimeParsed.Format("02-01-2006")
				days := int(time.Since(dncTimeParsed).Hours() / 24)

				if days >= 365 {
					years := days / 365
					duration = fmt.Sprintf("%d year", years)
					if years > 1 {
						duration += "s"
					}
				} else if days >= 30 {
					months := days / 30
					duration = fmt.Sprintf("%d month", months)
					if months > 1 {
						duration += "s"
					}
				} else if days > 0 {
					duration = fmt.Sprintf("%d day", days)
					if days > 1 {
						duration += "s"
					}
				} else {
					duration = "Today"
				}
			} else {
				dncDate = "N/A"
				duration = "N/A"
			}
		} else {
			dncDate = "N/A"
			duration = "N/A"
		}

		f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), mapDesc)
		f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), address)
		f.SetCellValue(sheetName, fmt.Sprintf("C%d", row), notes)
		f.SetCellValue(sheetName, fmt.Sprintf("D%d", row), dncDate)
		f.SetCellValue(sheetName, fmt.Sprintf("E%d", row), duration)

		rowStyle, _ := getDataCellStyle(f, i%2 == 0)
		f.SetCellStyle(sheetName, fmt.Sprintf("A%d", row), fmt.Sprintf("E%d", row), rowStyle)
		f.SetRowHeight(sheetName, row, 25)
		row++
	}

	f.SetColWidth(sheetName, "A", "A", 25)
	f.SetColWidth(sheetName, "B", "B", 20)
	f.SetColWidth(sheetName, "C", "C", 40)
	f.SetColWidth(sheetName, "D", "D", 15)
	f.SetColWidth(sheetName, "E", "E", 18)

	return nil
}

func createTerritorySheet(app *pocketbase.PocketBase, f *excelize.File, territory *core.Record, options []*core.Record) error {
	territoryCode := fmt.Sprintf("%v", territory.Get("code"))
	if territoryCode == "" || territoryCode == "<nil>" {
		territoryCode = territory.Id // Fallback to territory ID if code is empty
	}
	sheetName := territoryCode

	if territoryCode == "" || territoryCode == "<nil>" || territoryCode == "null" {
		sheetName = territory.Id[:8] // Use first 8 chars of ID
	}

	if len(sheetName) > 31 {
		sheetName = sheetName[:31] // Excel sheet name limit
	}

	existingSheets := f.GetSheetList()
	originalSheetName := sheetName
	counter := 1
	for contains(existingSheets, sheetName) {
		// If duplicate, try using territory ID to make it unique
		if counter == 1 {
			territoryIdShort := territory.Id[:8]
			if len(originalSheetName) > 23 {
				sheetName = fmt.Sprintf("%s_%s", originalSheetName[:23], territoryIdShort)
			} else {
				sheetName = fmt.Sprintf("%s_%s", originalSheetName, territoryIdShort)
			}
		} else {
			if len(originalSheetName) > 28 {
				sheetName = fmt.Sprintf("%s_%d", originalSheetName[:28], counter)
			} else {
				sheetName = fmt.Sprintf("%s_%d", originalSheetName, counter)
			}
		}
		counter++
		if counter > 999 { // Safety break
			break
		}
	}

	f.NewSheet(sheetName)

	territoryTitle := fmt.Sprintf("Territory Details: %s - %s", territory.Get("code"), territory.Get("description"))
	f.SetCellValue(sheetName, "A1", territoryTitle)
	f.MergeCell(sheetName, "A1", "F1")

	f.SetCellValue(sheetName, "A2", "Code")
	f.SetCellValue(sheetName, "B2", territory.Get("code"))
	f.SetCellValue(sheetName, "A3", "Description")
	f.SetCellValue(sheetName, "B3", territory.Get("description"))
	f.SetCellValue(sheetName, "A4", "Progress")
	if progressValue := territory.Get("progress"); progressValue != nil {
		// Convert progress to percentage string for territory details display
		// Note: Using string formatting instead of Excel percentage formatting for better reliability
		var progressStr string
		switch v := progressValue.(type) {
		case float64:
			progressStr = fmt.Sprintf("%.0f%%", v)
		case int:
			progressStr = fmt.Sprintf("%d%%", v)
		case string:
			if num, err := strconv.ParseFloat(v, 64); err == nil {
				progressStr = fmt.Sprintf("%.0f%%", num)
			} else {
				progressStr = "N/A"
			}
		default:
			progressStr = "N/A"
		}
		f.SetCellValue(sheetName, "B4", progressStr)
		detailValueStyle, _ := f.NewStyle(&excelize.Style{
			Font: &excelize.Font{Size: 11, Color: "333333", Family: "Calibri"},
			Fill: excelize.Fill{Type: "pattern", Color: []string{"FFFFFF"}, Pattern: 1},
			Alignment: &excelize.Alignment{
				Horizontal: "left",
				Vertical:   "center",
				Indent:     1,
			},
			Border: []excelize.Border{
				{Type: "left", Color: "8EAADB", Style: 1},
				{Type: "top", Color: "8EAADB", Style: 1},
				{Type: "bottom", Color: "8EAADB", Style: 1},
				{Type: "right", Color: "8EAADB", Style: 1},
			},
		})
		f.SetCellStyle(sheetName, "B4", "B4", detailValueStyle)
	} else {
		f.SetCellValue(sheetName, "B4", "N/A")
	}
	f.SetCellValue(sheetName, "A5", "Territory ID")
	f.SetCellValue(sheetName, "B5", territory.Id)

	territoryHeaderStyle, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Size: 16, Color: "FFFFFF", Family: "Calibri"},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"1F4E79"}, Pattern: 1}, // Consistent with main header
		Alignment: &excelize.Alignment{
			Horizontal: "left",
			Vertical:   "center",
			Indent:     1,
		},
		Border: []excelize.Border{
			{Type: "left", Color: "1F4E79", Style: 2},
			{Type: "top", Color: "1F4E79", Style: 2},
			{Type: "bottom", Color: "1F4E79", Style: 2},
			{Type: "right", Color: "1F4E79", Style: 2},
		},
	})

	detailLabelStyle, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Size: 11, Color: "1F4E79", Family: "Calibri"},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"E8F3FF"}, Pattern: 1}, // Consistent light blue
		Alignment: &excelize.Alignment{
			Horizontal: "left",
			Vertical:   "center",
			Indent:     1,
		},
		Border: []excelize.Border{
			{Type: "left", Color: "4A90B8", Style: 2},   // Strong border for territory labels
			{Type: "top", Color: "4A90B8", Style: 2},    // Strong border for territory labels
			{Type: "bottom", Color: "4A90B8", Style: 2}, // Strong border for territory labels
			{Type: "right", Color: "4A90B8", Style: 2},  // Strong border for territory labels
		},
	})

	f.SetCellStyle(sheetName, "A1", "F1", territoryHeaderStyle) // Apply style to merged header
	f.SetCellStyle(sheetName, "A2", "A5", detailLabelStyle)

	detailValueStyle, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Size: 11, Color: "333333", Family: "Calibri"},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"FFFFFF"}, Pattern: 1},
		Alignment: &excelize.Alignment{
			Horizontal: "left",
			Vertical:   "center",
			Indent:     1,
		},
		Border: []excelize.Border{
			{Type: "left", Color: "8EAADB", Style: 1},   // Visible border for territory values
			{Type: "top", Color: "8EAADB", Style: 1},    // Visible border for territory values
			{Type: "bottom", Color: "8EAADB", Style: 1}, // Visible border for territory values
			{Type: "right", Color: "8EAADB", Style: 1},  // Visible border for territory values
		},
	})
	f.SetCellStyle(sheetName, "B2", "B5", detailValueStyle)

	f.SetColWidth(sheetName, "A", "A", 20)
	f.SetColWidth(sheetName, "B", "Z", 50)
	f.SetRowHeight(sheetName, 1, 45)
	for i := 2; i <= 5; i++ {
		f.SetRowHeight(sheetName, i, 30)
	}

	maps, err := app.FindRecordsByFilter(
		"maps",
		"territory = {:territory}",
		"code",
		0,
		0,
		dbx.Params{"territory": territory.Id},
	)
	if err != nil {
		return fmt.Errorf("failed to fetch maps for territory: %v", err)
	}

	row := 7
	f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), "Maps Overview")
	f.MergeCell(sheetName, fmt.Sprintf("A%d", row), fmt.Sprintf("D%d", row))

	mapsOverviewHeaderStyle, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Size: 14, Color: "FFFFFF", Family: "Calibri"},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"4A90B8"}, Pattern: 1}, // Consistent with section headers
		Alignment: &excelize.Alignment{
			Horizontal: "left",
			Vertical:   "center",
			Indent:     1,
		},
		Border: []excelize.Border{
			{Type: "left", Color: "4A90B8", Style: 1},
			{Type: "top", Color: "4A90B8", Style: 1},
			{Type: "bottom", Color: "4A90B8", Style: 1},
			{Type: "right", Color: "4A90B8", Style: 1},
		},
	})
	f.SetCellStyle(sheetName, fmt.Sprintf("A%d", row), fmt.Sprintf("D%d", row), mapsOverviewHeaderStyle)
	f.SetRowHeight(sheetName, row, 30)
	row++

	if len(maps) == 0 {
		f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), "No maps found for this territory")
		row++
	} else {
		// Add overview table headers
		f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), "Name")
		f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), "Description")
		f.SetCellValue(sheetName, fmt.Sprintf("C%d", row), "Type")
		f.SetCellValue(sheetName, fmt.Sprintf("D%d", row), "Progress")

		overviewHeaderStyle, _ := f.NewStyle(&excelize.Style{
			Font: &excelize.Font{Bold: true, Size: 12, Color: "FFFFFF", Family: "Calibri"},
			Fill: excelize.Fill{Type: "pattern", Color: []string{"2E75B6"}, Pattern: 1}, // Consistent blue
			Alignment: &excelize.Alignment{
				Horizontal: "center",
				Vertical:   "center",
			},
			Border: []excelize.Border{
				{Type: "left", Color: "1F4E79", Style: 2},   // Strong dark border
				{Type: "top", Color: "1F4E79", Style: 2},    // Strong dark border
				{Type: "bottom", Color: "1F4E79", Style: 2}, // Strong dark border
				{Type: "right", Color: "1F4E79", Style: 2},  // Strong dark border
			},
		})
		f.SetCellStyle(sheetName, fmt.Sprintf("A%d", row), fmt.Sprintf("D%d", row), overviewHeaderStyle)
		f.SetRowHeight(sheetName, row, 28)
		row++

		// Add each map to the overview table
		for i, mapRecord := range maps {
			mapName := fmt.Sprintf("%v", mapRecord.Get("code"))
			if mapName == "<nil>" {
				mapName = ""
			}

			mapDescription := fmt.Sprintf("%v", mapRecord.Get("description"))
			if mapDescription == "<nil>" {
				mapDescription = ""
			}

			mapType := fmt.Sprintf("%v", mapRecord.Get("type"))
			if mapType == "<nil>" {
				mapType = ""
			}

			f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), mapName)
			f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), mapDescription)
			f.SetCellValue(sheetName, fmt.Sprintf("C%d", row), mapType)

			// Handle map progress and format as percentage
			mapProgressValue := mapRecord.Get("progress")
			if mapProgressValue != nil {
				var mapProgressNum float64
				var isNumeric bool

				switch v := mapProgressValue.(type) {
				case float64:
					mapProgressNum = v
					isNumeric = true
				case int:
					mapProgressNum = float64(v)
					isNumeric = true
				case string:
					if num, err := strconv.ParseFloat(v, 64); err == nil {
						mapProgressNum = num
						isNumeric = true
					}
				}

				if isNumeric {
					f.SetCellValue(sheetName, fmt.Sprintf("D%d", row), mapProgressNum/100)
				} else {
					if progressStr := fmt.Sprintf("%v", mapProgressValue); progressStr != "" && progressStr != "<nil>" {
						if num, err := strconv.ParseFloat(progressStr, 64); err == nil {
							f.SetCellValue(sheetName, fmt.Sprintf("D%d", row), num/100)
						} else {
							f.SetCellValue(sheetName, fmt.Sprintf("D%d", row), progressStr)
						}
					} else {
						f.SetCellValue(sheetName, fmt.Sprintf("D%d", row), "N/A")
					}
				}
			} else {
				f.SetCellValue(sheetName, fmt.Sprintf("D%d", row), "N/A")
			}

			var overviewRowStyle int
			if i%2 == 0 {
				overviewRowStyle, _ = f.NewStyle(&excelize.Style{
					Font: &excelize.Font{Size: 10, Color: "000000"},
					Fill: excelize.Fill{Type: "pattern", Color: []string{"F8F9FA"}, Pattern: 1},
					Alignment: &excelize.Alignment{
						Horizontal: "left",
						Vertical:   "center",
					},
					Border: []excelize.Border{
						{Type: "left", Color: "8EAADB", Style: 1},   // Visible border
						{Type: "top", Color: "8EAADB", Style: 1},    // Visible border
						{Type: "bottom", Color: "8EAADB", Style: 1}, // Visible border
						{Type: "right", Color: "8EAADB", Style: 1},  // Visible border
					},
				})
			} else {
				overviewRowStyle, _ = f.NewStyle(&excelize.Style{
					Font: &excelize.Font{Size: 10, Color: "000000"},
					Fill: excelize.Fill{Type: "pattern", Color: []string{"FFFFFF"}, Pattern: 1},
					Alignment: &excelize.Alignment{
						Horizontal: "left",
						Vertical:   "center",
					},
					Border: []excelize.Border{
						{Type: "left", Color: "8EAADB", Style: 1},   // Visible border
						{Type: "top", Color: "8EAADB", Style: 1},    // Visible border
						{Type: "bottom", Color: "8EAADB", Style: 1}, // Visible border
						{Type: "right", Color: "8EAADB", Style: 1},  // Visible border
					},
				})
			}
			f.SetCellStyle(sheetName, fmt.Sprintf("A%d", row), fmt.Sprintf("C%d", row), overviewRowStyle)

			progressStyle, _ := getPercentageCellStyle(f, i%2 == 0)
			f.SetCellStyle(sheetName, fmt.Sprintf("D%d", row), fmt.Sprintf("D%d", row), progressStyle)

			f.SetRowHeight(sheetName, row, 25)
			row++
		}
	}

	row += 2

	mapHeaderStyleID, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Size: 13, Color: "FFFFFF", Family: "Calibri"},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"4A90B8"}, Pattern: 1}, // Consistent with section headers
		Alignment: &excelize.Alignment{
			Horizontal: "left",
			Vertical:   "center",
			Indent:     1,
		},
		Border: []excelize.Border{
			{Type: "left", Color: "4A90B8", Style: 1},
			{Type: "top", Color: "4A90B8", Style: 1},
			{Type: "bottom", Color: "4A90B8", Style: 1},
			{Type: "right", Color: "4A90B8", Style: 1},
		},
	})

	f.SetRowHeight(sheetName, 6, 20)

	for _, mapRecord := range maps {
		mapCode := mapRecord.Get("code")
		mapProgress := mapRecord.Get("progress")
		mapDescription := mapRecord.Get("description")

		mapType := fmt.Sprintf("%v", mapRecord.Get("type"))

		mapHeader := fmt.Sprintf("Map: %s", mapCode)
		if mapProgress != nil && fmt.Sprintf("%v", mapProgress) != "" && fmt.Sprintf("%v", mapProgress) != "<nil>" {
			mapHeader = fmt.Sprintf("Map: %s (Progress: %v%%)", mapCode, mapProgress)
		}

		if mapDescription != nil && fmt.Sprintf("%v", mapDescription) != "" && fmt.Sprintf("%v", mapDescription) != "<nil>" {
			mapHeader = fmt.Sprintf("%s - %v", mapHeader, mapDescription)
		}

		if mapType != "" && mapType != "<nil>" {
			mapHeader = fmt.Sprintf("%s - %s", mapHeader, mapType)
		}

		addresses, err := app.FindRecordsByFilter(
			"addresses",
			"map = {:map}",
			"sequence, floor",
			0,
			0,
			dbx.Params{"map": mapRecord.Id},
		)
		if err != nil {
			log.Printf("Failed to fetch addresses for map header sizing: %v", err)
		}

		sequences := make(map[int]bool)
		for _, addr := range addresses {
			sequence := int(addr.Get("sequence").(float64))
			sequences[sequence] = true
		}

		isSingleType := mapType == "single"

		var lastCol string
		if len(sequences) > 0 {
			if isSingleType {
				lastCol = getExcelColumnName(len(sequences))
			} else {
				lastCol = getExcelColumnName(len(sequences) + 1)
			}
		} else {
			lastCol = "F"
		}

		f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), mapHeader)
		f.MergeCell(sheetName, fmt.Sprintf("A%d", row), fmt.Sprintf("%s%d", lastCol, row))
		f.SetCellStyle(sheetName, fmt.Sprintf("A%d", row), fmt.Sprintf("%s%d", lastCol, row), mapHeaderStyleID)
		f.SetRowHeight(sheetName, row, 30)
		row++

		if err := createAddressTable(app, f, sheetName, mapRecord, options, &row); err != nil {
			log.Printf("Failed to create address table for map %s: %v", mapRecord.Get("code"), err)
		}

		if len(maps) > 1 {
			row += 3
		}

		// Apply border styling to the table (no need for borders as cells already have them)
		// The individual cells already have proper styling applied in createAddressTable
	}

	f.SetColWidth(sheetName, "A", "A", 25)
	f.SetColWidth(sheetName, "B", "B", 40)
	f.SetColWidth(sheetName, "C", "C", 20)
	f.SetColWidth(sheetName, "D", "D", 18)
	f.SetColWidth(sheetName, "E", "E", 15)

	return nil
}

func createAddressTable(app *pocketbase.PocketBase, f *excelize.File, sheetName string, mapRecord *core.Record, options []*core.Record, startRow *int) error {
	// Get addresses for this map
	addresses, err := app.FindRecordsByFilter(
		"addresses",
		"map = {:map}",
		"sequence, floor",
		0,
		0,
		dbx.Params{"map": mapRecord.Id},
	)
	if err != nil {
		return fmt.Errorf("failed to fetch addresses: %v", err)
	}

	if len(addresses) == 0 {
		f.SetCellValue(sheetName, fmt.Sprintf("A%d", *startRow), "No addresses found")
		*startRow++
		return nil
	}

	mapType := fmt.Sprintf("%v", mapRecord.Get("type"))
	isSingleType := mapType == "single"

	optionsMap := make(map[string]string)
	for _, option := range options {
		optionsMap[option.Id] = fmt.Sprintf("%v", option.Get("code"))
	}

	addressGrid := make(map[int]map[int]*core.Record)
	sequenceToCode := make(map[int]string)
	sequences := make(map[int]bool)
	floors := make(map[int]bool)

	for _, addr := range addresses {
		var sequence int
		if seqVal := addr.Get("sequence"); seqVal != nil {
			switch v := seqVal.(type) {
			case float64:
				sequence = int(v)
			case int:
				sequence = v
			case int64:
				sequence = int(v)
			default:
				sequence = 0
			}
		}

		var floor int
		if floorVal := addr.Get("floor"); floorVal != nil {
			switch v := floorVal.(type) {
			case float64:
				floor = int(v)
			case int:
				floor = v
			case int64:
				floor = int(v)
			default:
				floor = 0
			}
		} else {
			floor = 0
		}

		code := fmt.Sprintf("%v", addr.Get("code"))

		if addressGrid[sequence] == nil {
			addressGrid[sequence] = make(map[int]*core.Record)
		}
		addressGrid[sequence][floor] = addr
		sequenceToCode[sequence] = code
		sequences[sequence] = true
		floors[floor] = true
	}

	var sequenceList []int
	for seq := range sequences {
		sequenceList = append(sequenceList, seq)
	}
	sort.Ints(sequenceList)

	var floorList []int
	for floor := range floors {
		floorList = append(floorList, floor)
	}

	sort.Sort(sort.Reverse(sort.IntSlice(floorList)))

	headerRow := *startRow

	if isSingleType {
		for i, seq := range sequenceList {
			col := getExcelColumnName(i + 1)
			addressCode := sequenceToCode[seq]
			if numCode, err := strconv.Atoi(addressCode); err == nil {
				f.SetCellValue(sheetName, fmt.Sprintf("%s%d", col, headerRow), numCode)
			} else {
				f.SetCellValue(sheetName, fmt.Sprintf("%s%d", col, headerRow), addressCode)
			}
		}
	} else {
		f.SetCellValue(sheetName, fmt.Sprintf("A%d", headerRow), "Floor\\Code")

		for i, seq := range sequenceList {
			col := getExcelColumnName(i + 2)
			addressCode := sequenceToCode[seq]
			if numCode, err := strconv.Atoi(addressCode); err == nil {
				f.SetCellValue(sheetName, fmt.Sprintf("%s%d", col, headerRow), numCode)
			} else {
				f.SetCellValue(sheetName, fmt.Sprintf("%s%d", col, headerRow), addressCode)
			}
		}
	}

	headerStyleID, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Size: 11, Color: "FFFFFF", Family: "Calibri"},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"2E75B6"}, Pattern: 1}, // Consistent professional blue
		Alignment: &excelize.Alignment{
			Horizontal: "center",
			Vertical:   "center",
		},
		Border: []excelize.Border{
			{Type: "left", Color: "1F4E79", Style: 2},   // Thick dark blue border
			{Type: "top", Color: "1F4E79", Style: 2},    // Thick dark blue border
			{Type: "bottom", Color: "1F4E79", Style: 2}, // Thick dark blue border
			{Type: "right", Color: "1F4E79", Style: 2},  // Thick dark blue border
		},
	})

	var lastCol string
	if isSingleType {
		lastCol = getExcelColumnName(len(sequenceList))
	} else {
		lastCol = getExcelColumnName(len(sequenceList) + 1)
	}
	f.SetCellStyle(sheetName, fmt.Sprintf("A%d", headerRow), fmt.Sprintf("%s%d", lastCol, headerRow), headerStyleID)

	*startRow++

	floorHeaderStyleID, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Size: 11, Color: "FFFFFF", Family: "Calibri"},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"2E75B6"}, Pattern: 1}, // Consistent with table headers
		Alignment: &excelize.Alignment{
			Horizontal: "center",
			Vertical:   "center",
		},
		Border: []excelize.Border{
			{Type: "left", Color: "1F4E79", Style: 2},   // Thick dark blue border
			{Type: "top", Color: "1F4E79", Style: 2},    // Thick dark blue border
			{Type: "bottom", Color: "1F4E79", Style: 2}, // Thick dark blue border
			{Type: "right", Color: "1F4E79", Style: 2},  // Thick dark blue border
		},
	})

	dataCellStyleID, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Size: 10, Color: "333333", Family: "Calibri"},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"FFFFFF"}, Pattern: 1},
		Alignment: &excelize.Alignment{
			Horizontal: "center",
			Vertical:   "center",
		},
		Border: []excelize.Border{
			{Type: "left", Color: "8EAADB", Style: 1},   // Medium blue border for visibility
			{Type: "top", Color: "8EAADB", Style: 1},    // Medium blue border for visibility
			{Type: "bottom", Color: "8EAADB", Style: 1}, // Medium blue border for visibility
			{Type: "right", Color: "8EAADB", Style: 1},  // Medium blue border for visibility
		},
	})

	dataCellAltStyleID, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Size: 10, Color: "333333", Family: "Calibri"},
		Fill: excelize.Fill{Type: "pattern", Color: []string{"F8FBFF"}, Pattern: 1}, // Very light blue
		Alignment: &excelize.Alignment{
			Horizontal: "center",
			Vertical:   "center",
		},
		Border: []excelize.Border{
			{Type: "left", Color: "8EAADB", Style: 1},   // Medium blue border for visibility
			{Type: "top", Color: "8EAADB", Style: 1},    // Medium blue border for visibility
			{Type: "bottom", Color: "8EAADB", Style: 1}, // Medium blue border for visibility
			{Type: "right", Color: "8EAADB", Style: 1},  // Medium blue border for visibility
		},
	})

	if isSingleType {
		row := *startRow

		for i, seq := range sequenceList {
			col := getExcelColumnName(i + 1)
			f.SetCellStyle(sheetName, fmt.Sprintf("%s%d", col, row), fmt.Sprintf("%s%d", col, row), dataCellStyleID)

			var targetAddr *core.Record
			if seqAddresses, exists := addressGrid[seq]; exists {
				for _, addr := range seqAddresses {
					targetAddr = addr
					break
				}
			}

			if targetAddr != nil {
				typeCode := ""
				if targetAddr.Get("type") != nil {
					typeField := targetAddr.Get("type")
					var typeId string
					if typeRels, ok := typeField.([]interface{}); ok && len(typeRels) > 0 {
						if id, ok := typeRels[0].(string); ok {
							typeId = id
						}
					} else if typeRels, ok := typeField.([]string); ok && len(typeRels) > 0 {
						typeId = typeRels[0]
					}

					if typeId != "" {
						if code, found := optionsMap[typeId]; found {
							typeCode = code
						}
					}
				}

				status := fmt.Sprintf("%v", targetAddr.Get("status"))
				statusSymbol := getStatusSymbol(status)
				var cellValue string

				if typeCode != "" && statusSymbol != "" {
					cellValue = fmt.Sprintf("%s %s", typeCode, statusSymbol)
				} else if typeCode != "" {
					cellValue = typeCode
				} else if statusSymbol != "" {
					cellValue = statusSymbol
				} else {
					cellValue = ""
				}
				f.SetCellValue(sheetName, fmt.Sprintf("%s%d", col, row), cellValue)
			}
		}

		*startRow++
	} else {
		for floorIndex, floor := range floorList {
			row := *startRow

			f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), floor)
			f.SetCellStyle(sheetName, fmt.Sprintf("A%d", row), fmt.Sprintf("A%d", row), floorHeaderStyleID)

			currentDataStyleID := dataCellStyleID
			if floorIndex%2 == 1 {
				currentDataStyleID = dataCellAltStyleID
			}

			for i, seq := range sequenceList {
				col := getExcelColumnName(i + 2)
				f.SetCellStyle(sheetName, fmt.Sprintf("%s%d", col, row), fmt.Sprintf("%s%d", col, row), currentDataStyleID)

				if addr, exists := addressGrid[seq][floor]; exists {
					typeCode := ""
					if addr.Get("type") != nil {
						typeField := addr.Get("type")
						var typeId string
						if typeRels, ok := typeField.([]interface{}); ok && len(typeRels) > 0 {
							if id, ok := typeRels[0].(string); ok {
								typeId = id
							}
						} else if typeRels, ok := typeField.([]string); ok && len(typeRels) > 0 {
							typeId = typeRels[0]
						}

						if typeId != "" {
							if code, found := optionsMap[typeId]; found {
								typeCode = code
							} else {
							}
						}
					}

					status := fmt.Sprintf("%v", addr.Get("status"))
					statusSymbol := getStatusSymbol(status)
					var cellValue string

					if typeCode != "" && statusSymbol != "" {
						cellValue = fmt.Sprintf("%s %s", typeCode, statusSymbol)
					} else if typeCode != "" {
						cellValue = typeCode
					} else if statusSymbol != "" {
						cellValue = statusSymbol
					} else {
						cellValue = ""
					}
					f.SetCellValue(sheetName, fmt.Sprintf("%s%d", col, row), cellValue)
				}
			}

			*startRow++
		}
	}

	if isSingleType {
		if len(sequenceList) > 0 {
			lastCol := getExcelColumnName(len(sequenceList))
			f.SetColWidth(sheetName, "A", lastCol, 14) // Consistent width for all address columns
		}

		f.SetRowHeight(sheetName, headerRow, 35)
		f.SetRowHeight(sheetName, headerRow+1, 32)
	} else {
		f.SetColWidth(sheetName, "A", "A", 16)

		if len(sequenceList) > 0 {
			lastCol := getExcelColumnName(len(sequenceList) + 1)
			f.SetColWidth(sheetName, "B", lastCol, 14)
		}

		if len(floorList) > 0 {
			startRowNum := headerRow
			endRowNum := startRowNum + len(floorList)
			f.SetRowHeight(sheetName, headerRow, 35)
			for r := startRowNum + 1; r <= endRowNum; r++ {
				f.SetRowHeight(sheetName, r, 32)
			}
		}
	}

	return nil
}

func getExcelColumnName(col int) string {
	if col <= 0 {
		return "A"
	}

	result := ""
	for col > 0 {
		col--
		result = string(rune('A'+col%26)) + result
		col /= 26
	}
	return result
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func getStatusSymbol(status string) string {
	switch status {
	case "done":
		return "✓"
	case "not_done":
		return "○"
	case "not_home":
		return "⌂"
	case "invalid":
		return "✗"
	case "do_not_call":
		return "⚠"
	case "<nil>", "":
		return "○"
	default:
		return "?"
	}
}

func sendReportEmailFromBuffer(app *pocketbase.PocketBase, congregation *core.Record, filename string, content []byte) error {
	log.Printf("Sending report email for congregation: %s", congregation.Get("code"))

	// Get recipients (administrators) for the congregation
	recipients := []ReportRecipient{}
	err := app.DB().Select("users.*").From("users").InnerJoin("roles", dbx.NewExp("roles.user = users.id and roles.role = 'administrator'")).Where(dbx.NewExp("roles.congregation = {:congregation}", dbx.Params{"congregation": congregation.Id})).All(&recipients)

	if err != nil {
		log.Println("Error fetching recipients:", err)
		return err
	}

	if len(recipients) == 0 {
		log.Println("No recipients found")
		return nil
	}

	log.Printf("Processing %d recipients\n", len(recipients))

	// Load template
	tmpl, err := template.ParseFiles("templates/report.html")
	if err != nil {
		log.Println("Error parsing template:", err)
		return err
	}

	// Prepare email data
	currentDate := time.Now().Format("January 2006")
	emailData := ReportTemplateData{
		CongregationName: congregation.Get("name").(string),
		CongregationCode: congregation.Get("code").(string),
		ReportDate:       currentDate,
		FileName:         filename,
	}

	// Execute template
	var body bytes.Buffer
	if err := tmpl.Execute(&body, emailData); err != nil {
		log.Println("Error executing template:", err)
		return err
	}

	// Encode file as base64
	encoded := base64.StdEncoding.EncodeToString(content)

	// Initialize MailerSend
	ms := mailersend.NewMailersend(os.Getenv("MAILERSEND_API_KEY"))

	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Create email message
	message := ms.Email.NewMessage()

	message.SetFrom(mailersend.From{
		Email: os.Getenv("MAILERSEND_FROM_EMAIL"),
		Name:  "Ministry Mapper",
	})

	// Set recipients
	emailRecipients := []mailersend.Recipient{}
	for _, r := range recipients {
		emailRecipients = append(emailRecipients, mailersend.Recipient{
			Email: r.Email,
			Name:  r.Name,
		})
	}
	message.SetRecipients(emailRecipients)

	// Set subject and body
	subject := fmt.Sprintf("Monthly Report for %s - %s", congregation.Get("name"), currentDate)
	message.SetSubject(subject)
	message.SetHTML(body.String())

	// Add Excel file as attachment
	attachment := mailersend.Attachment{
		Filename: filename,
		Content:  encoded,
	}
	message.AddAttachment(attachment)

	// Send email
	_, err = ms.Email.Send(ctx, message)
	if err != nil {
		log.Printf("Error sending email: %v", err)
		return err
	}

	log.Println("Report email sent successfully")
	return nil
}
