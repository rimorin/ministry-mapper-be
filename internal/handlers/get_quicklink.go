package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

type MapWithDistance struct {
	ID           string `db:"id"`
	Description  string `db:"description"`
	Progress     int    `db:"progress"`
	Coordinates  string `db:"coordinates"`
	Aggregates   string `db:"aggregates"`
	AssignCount  int    `db:"assignment_count"`
	Congregation string `db:"congregation"`
	Distance     float64
	ParsedCoords *Coordinates // set by findBestMap; nil when coordinates are invalid
}

// congregationExpiryCache stores expiry_hours per congregation ID.
// Congregation settings change rarely (admin action required), so a process-lifetime
// cache is safe and eliminates one DB round trip per quicklink request.
var congregationExpiryCache sync.Map

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
func HandleTerritoryQuicklink(c *core.RequestEvent, app core.App) error {
	requestInfo, _ := c.RequestInfo()
	data := requestInfo.Body

	territoryId, ok := data["territory"].(string)
	if !ok || territoryId == "" {
		return apis.NewBadRequestError("Territory ID is required", nil)
	}

	coordinates, ok := data["coordinates"].(map[string]interface{})
	if !ok || coordinates == nil {
		return apis.NewBadRequestError("Coordinates are required", nil)
	}

	latValue, latExists := coordinates["lat"]
	longValue, longExists := coordinates["lng"]

	if !latExists || !longExists {
		return apis.NewBadRequestError("Both lat and long coordinates are required", nil)
	}

	currentLat, ok := latValue.(float64)
	if !ok {
		return apis.NewBadRequestError("Invalid latitude value", nil)
	}

	currentLong, ok := longValue.(float64)
	if !ok {
		return apis.NewBadRequestError("Invalid longitude value", nil)
	}

	publisher, ok := data["publisher"].(string)
	if !ok || publisher == "" {
		return apis.NewBadRequestError("Publisher is required", nil)
	}

	userId := c.Auth.Id

	maps, err := getMapsWithAssignmentCount(app, territoryId)
	if err != nil {
		return apis.NewNotFoundError("Error fetching maps", nil)
	}

	if len(maps) == 0 {
		return apis.NewNotFoundError("No maps found for territory", nil)
	}

	bestMap := findBestMap(maps, currentLat, currentLong)
	if bestMap == nil {
		return apis.NewNotFoundError("No suitable map found", nil)
	}

	congregationId := bestMap.Congregation

	expiryHours, err := getCongregationExpiryHours(app, congregationId)
	if err != nil {
		return apis.NewNotFoundError("Error fetching expiry hours", nil)
	}

	assignmentId, err := createAssignment(app, bestMap.ID, userId, publisher, congregationId, expiryHours)
	if err != nil {
		return apis.NewBadRequestError("Error creating assignment", nil)
	}

	assignees, err := getMapAssignees(app, bestMap.ID, assignmentId)
	if err != nil {
		return apis.NewNotFoundError("Error fetching assignees", nil)
	}

	var aggregates MapAggregates
	if bestMap.Aggregates != "" {
		if err := json.Unmarshal([]byte(bestMap.Aggregates), &aggregates); err != nil {
			log.Printf("Error parsing aggregates for map %s: %v", bestMap.ID, err)
			aggregates = MapAggregates{}
		}
	}

	// Coordinates were already parsed in findBestMap — reuse the cached result.
	var coords Coordinates
	if bestMap.ParsedCoords != nil {
		coords = *bestMap.ParsedCoords
	}

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
func getMapsWithAssignmentCount(app core.App, territoryId string) ([]MapWithDistance, error) {
	maps := []MapWithDistance{}

	query := `
		SELECT
			m.id,
			m.congregation,
			COALESCE(m.description, '') as description,
			COALESCE(m.progress, 0) as progress,
			COALESCE(m.coordinates, '{}') as coordinates,
			COALESCE(m.aggregates, '{}') as aggregates,
			COUNT(CASE WHEN a.type = 'normal' AND a.expiry_date > datetime('now') THEN a.id END) as assignment_count
		FROM maps m
		LEFT JOIN assignments a ON m.id = a.map
		WHERE m.territory = {:territory}
		GROUP BY m.id
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

	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
			math.Sin(dLon/2)*math.Sin(dLon/2)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return R * c
}

// findBestMap selects the optimal map based on assignment count, distance, and progress.
// Priority: fewest assignments > proximity (50m threshold) > lowest progress.
//
// Uses separate passes to avoid order-dependent results that a single greedy pass
// produces when the 50m proximity window straddles multiple maps at different distances.
func findBestMap(maps []MapWithDistance, currentLat, currentLong float64) *MapWithDistance {
	if len(maps) == 0 {
		return nil
	}

	// Pass 1: compute all distances and cache parsed coordinates.
	for i := range maps {
		var coords Coordinates
		if err := json.Unmarshal([]byte(maps[i].Coordinates), &coords); err != nil {
			maps[i].Distance = math.Inf(1)
			continue
		}
		maps[i].Distance = haversineDistance(currentLat, currentLong, coords.Lat, coords.Lng)
		c := coords
		maps[i].ParsedCoords = &c
	}

	// Pass 2: find minimum assignment count among maps with valid coordinates.
	minCount := math.MaxInt32
	for _, m := range maps {
		if !math.IsInf(m.Distance, 1) && m.AssignCount < minCount {
			minCount = m.AssignCount
		}
	}
	if minCount == math.MaxInt32 {
		return nil
	}

	// Pass 3: find minimum distance within the minimum-count cohort.
	minDist := math.Inf(1)
	for _, m := range maps {
		if m.AssignCount == minCount && !math.IsInf(m.Distance, 1) && m.Distance < minDist {
			minDist = m.Distance
		}
	}

	// Pass 4: pick lowest progress among maps within 50m of the minimum distance.
	// Maps within this band are considered equally close; progress breaks the tie.
	var best *MapWithDistance
	for i := range maps {
		m := &maps[i]
		if m.AssignCount != minCount || math.IsInf(m.Distance, 1) || m.Distance > minDist+50 {
			continue
		}
		if best == nil || m.Progress < best.Progress {
			best = m
		}
	}

	return best
}

// getCongregationExpiryHours gets assignment expiry hours from congregation settings.
// Results are cached for the process lifetime since expiry_hours changes only via admin action.
// Defaults to 24 hours if not set.
func getCongregationExpiryHours(app core.App, congregationId string) (float64, error) {
	if cached, ok := congregationExpiryCache.Load(congregationId); ok {
		return cached.(float64), nil
	}

	congregation, err := app.FindRecordById("congregations", congregationId)
	if err != nil {
		return 0, err
	}

	hours := congregation.GetFloat("expiry_hours")
	if hours == 0 {
		hours = 24
	}

	congregationExpiryCache.Store(congregationId, hours)
	return hours, nil
}

// createAssignment creates a new assignment record linking a user to a map with expiry.
func createAssignment(app core.App, mapId, userId, publisher, congId string, expiryHours float64) (string, error) {
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
// Excludes the specified assignment. Returns "slip-XXXX" for anonymous publishers.
func getMapAssignees(app core.App, mapId, excludeAssignmentId string) ([]string, error) {
	assignees := []struct {
		Id        string `db:"id"`
		Publisher string `db:"publisher"`
	}{}

	query := `
		SELECT id, publisher
		FROM assignments
		WHERE map = {:map_id}
		AND type = 'normal'
		AND id != {:exclude_assignment_id}
		AND expiry_date > datetime('now')
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
		if assignee.Publisher == "" {
			publishers[i] = fmt.Sprintf("slip-%s", assignee.Id[:4])
		} else {
			publishers[i] = assignee.Publisher
		}
	}

	return publishers, nil
}
