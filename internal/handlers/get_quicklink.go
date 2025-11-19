package handlers

import (
	"encoding/json"
	"log"
	"math"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

type MapWithDistance struct {
	ID          string `db:"id"`
	Description string `db:"description"`
	Progress    int    `db:"progress"`
	Coordinates string `db:"coordinates"`
	Aggregates  string `db:"aggregates"`
	AssignCount int    `db:"assignment_count"`
	Distance    float64
}

type Coordinates struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

type MapAggregates struct {
	NotDone int `json:"notDone"`
	NotHome int `json:"notHome"`
}

// HandleTerritoryQuicklink automatically assigns the best available map to a user
// based on workload balance, proximity, and completion progress.
func HandleTerritoryQuicklink(c *core.RequestEvent, app *pocketbase.PocketBase) error {
	// === INPUT VALIDATION ===
	requestInfo, _ := c.RequestInfo()
	data := requestInfo.Body

	// Validate territory ID
	territoryId, ok := data["territory"].(string)
	if !ok || territoryId == "" {
		return apis.NewBadRequestError("Territory ID is required", nil)
	}

	// Validate coordinates
	coordinates, ok := data["coordinates"].(map[string]interface{})
	if !ok || coordinates == nil {
		return apis.NewBadRequestError("Coordinates are required", nil)
	}

	// Extract lat/lng values
	latValue, latExists := coordinates["lat"]
	longValue, longExists := coordinates["lng"]

	if !latExists || !longExists {
		return apis.NewBadRequestError("Both lat and long coordinates are required", nil)
	}

	// Validate coordinate types
	currentLat, ok := latValue.(float64)
	if !ok {
		return apis.NewBadRequestError("Invalid latitude value", nil)
	}

	currentLong, ok := longValue.(float64)
	if !ok {
		return apis.NewBadRequestError("Invalid longitude value", nil)
	}

	// Extract publisher (optional)
	publisher, ok := data["publisher"].(string)
	if !ok {
		publisher = ""
	}

	userId := c.Auth.Id

	// === MAP SELECTION ===
	// Get all maps with assignment counts
	maps, err := getMapsWithAssignmentCount(app, territoryId)
	if err != nil {
		sentry.CaptureException(err)
		return apis.NewNotFoundError("Error fetching maps", nil)
	}

	if len(maps) == 0 {
		return apis.NewNotFoundError("No maps found for territory", nil)
	}

	// Find the best map using intelligent selection
	bestMap := findBestMap(maps, currentLat, currentLong)
	if bestMap == nil {
		return apis.NewNotFoundError("No suitable map found", nil)
	}

	// === ASSIGNMENT CREATION ===
	// Get congregation settings
	congregationId, err := getCongregationIdFromTerritory(app, territoryId)
	if err != nil {
		sentry.CaptureException(err)
		return apis.NewNotFoundError("Error fetching congregation", nil)
	}

	expiryHours, err := getCongregationExpiryHours(app, congregationId)
	if err != nil {
		sentry.CaptureException(err)
		return apis.NewNotFoundError("Error fetching expiry hours", nil)
	}

	// Create assignment record
	assignmentId, err := createAssignment(app, bestMap.ID, userId, publisher, congregationId, expiryHours)
	if err != nil {
		sentry.CaptureException(err)
		return apis.NewBadRequestError("Error creating assignment", nil)
	}

	// Get other assignees for coordination
	assignees, err := getMapAssignees(app, bestMap.ID, assignmentId)
	if err != nil {
		sentry.CaptureException(err)
		return apis.NewNotFoundError("Error fetching assignees", nil)
	}

	// === RESPONSE PREPARATION ===
	// Parse map aggregates
	var aggregates MapAggregates
	if bestMap.Aggregates != "" {
		if err := json.Unmarshal([]byte(bestMap.Aggregates), &aggregates); err != nil {
			log.Printf("Error parsing aggregates for map %s: %v", bestMap.ID, err)
			aggregates = MapAggregates{NotDone: 0, NotHome: 0}
		}
	} else {
		aggregates = MapAggregates{NotDone: 0, NotHome: 0}
	}

	// Parse map coordinates
	var coords Coordinates
	if bestMap.Coordinates != "" {
		if err := json.Unmarshal([]byte(bestMap.Coordinates), &coords); err != nil {
			log.Printf("Error parsing coordinates for map %s: %v", bestMap.ID, err)
			coords = Coordinates{Lat: 0, Lng: 0}
		}
	} else {
		coords = Coordinates{Lat: 0, Lng: 0}
	}

	// Return assignment details and map information
	return c.JSON(200, map[string]interface{}{
		"linkId":      assignmentId,
		"mapName":     bestMap.Description,
		"progress":    bestMap.Progress,
		"not_done":    aggregates.NotDone,
		"not_home":    aggregates.NotHome,
		"coordinates": coords,
		"assignees":   assignees,
	})
}

// getMapsWithAssignmentCount gets all maps for a territory with their current assignment counts.
// Only counts active (non-expired) "normal" type assignments.
func getMapsWithAssignmentCount(app *pocketbase.PocketBase, territoryId string) ([]MapWithDistance, error) {
	maps := []MapWithDistance{}

	query := `
		SELECT 
			m.id,
			COALESCE(m.description, '') as description,
			COALESCE(m.progress, 0) as progress,
			COALESCE(m.coordinates, '{}') as coordinates,
			COALESCE(m.aggregates, '{}') as aggregates,
			COALESCE(COUNT(CASE WHEN a.type = 'normal' AND a.expiry_date > datetime('now') THEN a.id END), 0) as assignment_count
		FROM maps m
		LEFT JOIN assignments a ON m.id = a.map
		WHERE m.territory = {:territory}
		GROUP BY m.id, m.description, m.progress, m.coordinates, m.aggregates
		ORDER BY assignment_count ASC, progress ASC
	`

	err := app.DB().NewQuery(query).Bind(dbx.Params{
		"territory": territoryId,
	}).All(&maps)

	return maps, err
}

// haversineDistance calculates the distance between two geographic points in meters.
func haversineDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371000 // Earth radius in meters

	// Convert to radians
	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180

	// Apply Haversine formula
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
			math.Sin(dLon/2)*math.Sin(dLon/2)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return R * c
}

// findBestMap selects the optimal map based on assignment count, distance, and progress.
// Priority: fewest assignments > proximity (50m threshold) > lowest progress.
func findBestMap(maps []MapWithDistance, currentLat, currentLong float64) *MapWithDistance {
	if len(maps) == 0 {
		return nil
	}

	// Initialize tracking variables
	var bestMap *MapWithDistance
	bestAssignCount := math.MaxInt32
	bestDistance := math.Inf(1)
	bestProgress := math.MaxInt32

	// Find the best map in a single pass
	for i := range maps {
		// Parse coordinates
		var coords Coordinates
		if err := json.Unmarshal([]byte(maps[i].Coordinates), &coords); err != nil {
			continue
		}

		// Calculate distance
		distance := haversineDistance(currentLat, currentLong, coords.Lat, coords.Lng)
		maps[i].Distance = distance

		// Determine if this map is better
		isBetter := false

		if maps[i].AssignCount < bestAssignCount {
			// Fewer assignments is always better
			isBetter = true
		} else if maps[i].AssignCount == bestAssignCount {
			// Same assignment count, check distance
			if distance < bestDistance-50 {
				// Significantly closer (50+ meters)
				isBetter = true
			} else if math.Abs(distance-bestDistance) <= 50 {
				// Similar distance, check progress
				if maps[i].Progress < bestProgress {
					isBetter = true
				}
			}
		}

		// Update best map if superior
		if isBetter {
			bestMap = &maps[i]
			bestAssignCount = maps[i].AssignCount
			bestDistance = distance
			bestProgress = maps[i].Progress
		}
	}

	return bestMap
}

// getCongregationIdFromTerritory retrieves the congregation ID associated with a territory.
func getCongregationIdFromTerritory(app *pocketbase.PocketBase, territoryId string) (string, error) {
	territory, err := app.FindRecordById("territories", territoryId)
	if err != nil {
		return "", err
	}
	return territory.Get("congregation").(string), nil
}

// getCongregationExpiryHours gets assignment expiry hours from congregation settings.
// Defaults to 24 hours if not set.
func getCongregationExpiryHours(app *pocketbase.PocketBase, congregationId string) (float64, error) {
	congregation, err := app.FindRecordById("congregations", congregationId)
	if err != nil {
		return 0, err
	}

	expiryHours := congregation.Get("expiry_hours")
	if expiryHours == nil {
		return 24, nil // Default to 24 hours
	}

	return expiryHours.(float64), nil
}

// createAssignment creates a new assignment record linking a user to a map with expiry.
func createAssignment(app *pocketbase.PocketBase, mapId, userId, publisher, congId string, expiryHours float64) (string, error) {
	collection, err := app.FindCollectionByNameOrId("assignments")
	if err != nil {
		return "", err
	}

	assignment := core.NewRecord(collection)
	assignment.Set("map", mapId)
	assignment.Set("user", userId)
	assignment.Set("type", "normal")
	assignment.Set("publisher", publisher)
	assignment.Set("congregation", congId)

	// Calculate expiry date
	expiryDate := time.Now().UTC().Add(time.Duration(expiryHours) * time.Hour)
	assignment.Set("expiry_date", expiryDate)

	if err := app.Save(assignment); err != nil {
		return "", err
	}

	return assignment.Id, nil
}

// getMapAssignees gets all publishers currently assigned to a map.
// Excludes the specified assignment and generates "slip-XXXX" names for empty publishers.
func getMapAssignees(app *pocketbase.PocketBase, mapId, excludeAssignmentId string) ([]string, error) {
	assignees := []struct {
		Publisher string `db:"publisher"`
	}{}

	query := `
		SELECT publisher
		FROM assignments 
		WHERE map = {:map_id}
		AND type = 'normal'
		AND id != {:exclude_assignment_id}
		AND expiry_date > datetime('now', 'utc')
	`

	err := app.DB().NewQuery(query).Bind(dbx.Params{
		"map_id":                mapId,
		"exclude_assignment_id": excludeAssignmentId,
	}).All(&assignees)

	if err != nil {
		return nil, err
	}

	publishers := make([]string, len(assignees))
	for i, assignee := range assignees {
		publishers[i] = assignee.Publisher
	}

	return publishers, nil
}
