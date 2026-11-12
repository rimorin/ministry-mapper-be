package handlers

import (
	"cmp"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"strings"
	"sync"
	"time"

	"ganigeorgiev.com/pocketbase/dbx"
	"ganigeorgiev.com/pocketbase/pocketbase/apis"
	"ganigeorgiev.com/pocketbase/pocketbase/core"
	"ganigeorgiev.com/pocketbase/pocketbase/tools/types"
)

type MapWithDistance struct {
	ID           string       `db:"id"`
	Description  string       `db:"description"`
	Progress     int          `db:"progress"`
	Sequence     int          `db:"sequence"`
	Coordinates  string       `db:"coordinates"`
	Aggregates   string       `db:"aggregates"`
	AssignCount  int          `db:"assignment_count"`
	Distance     float64      // set by findBestMap; +Inf when the map has no usable coordinates
	ParsedCoords *Coordinates // set by findBestMap; nil when the map has no usable coordinates
	Remaining    int          // set by findBestMap; addresses left to work, 0 when unknown
	tier         int          // set by findBestMap; see mapTier
}

// quicklinkSettings are the congregation settings a quicklink request depends on.
type quicklinkSettings struct {
	expiryHours         float64
	completionThreshold int
}

// quicklinkSettingsCache stores quicklinkSettings per congregation ID, saving a DB
// round trip on every quicklink request. Congregation settings change rarely, but
// "rarely" is not "never": InvalidateQuicklinkSettingsCache must be called
// whenever a congregation record changes, or an admin's edit has no effect until
// the process restarts.
var quicklinkSettingsCache sync.Map

// InvalidateQuicklinkSettingsCache drops the cached settings for a congregation.
// Wired to the congregations update hook in setup.RegisterDomainHooks.
func InvalidateQuicklinkSettingsCache(congregationId string) {
	quicklinkSettingsCache.Delete(congregationId)
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
	if !ok || currentLat < -90 || currentLat > 90 {
		return apis.NewBadRequestError("Invalid latitude value", nil)
	}

	currentLong, ok := longValue.(float64)
	if !ok || currentLong < -180 || currentLong > 180 {
		return apis.NewBadRequestError("Invalid longitude value", nil)
	}

	publisher, _ := data["publisher"].(string)
	publisher = strings.TrimSpace(publisher)
	if publisher == "" {
		return apis.NewBadRequestError("Publisher is required", nil)
	}

	userId := c.Auth.Id

	// The assignment minted below grants publisher access to the selected map, so
	// the caller must hold a role in the territory's congregation. Any role is
	// enough — read_only publishers legitimately use quicklinks.
	congregationId := getTerritoryCongregation(app, territoryId)
	if congregationId == "" {
		return apis.NewNotFoundError("Territory not found", nil)
	}
	if !AuthorizeByRole(app, userId, congregationId) {
		return apis.NewForbiddenError("Unauthorized", nil)
	}

	settings, err := getQuicklinkSettings(app, congregationId)
	if err != nil {
		return newServerError(err)
	}

	// A link stops counting towards a map's workload after half its lifetime
	// without activity.
	activeSince := time.Now().Add(-min(time.Duration(settings.expiryHours/2*float64(time.Hour)), maxActivityWindow))

	// Counting and minting share a transaction, and PocketBase runs transactions
	// one at a time, so each request sees the links minted before it. Otherwise a
	// group tapping at the same moment reads the same counts and piles onto one map.
	var bestMap *MapWithDistance
	var assignmentId string
	var assignees []string
	err = app.RunInTransaction(func(txApp core.App) error {
		maps, err := getMapsWithAssignmentCount(txApp, territoryId, congregationId, activeSince)
		if err != nil {
			return err
		}
		if len(maps) == 0 {
			return apis.NewNotFoundError("No maps found for territory", nil)
		}

		bestMap = findBestMap(maps, currentLat, currentLong, settings.completionThreshold)

		assignmentId, err = createAssignment(txApp, bestMap.ID, c.Auth, publisher, congregationId, settings.expiryHours)
		if err != nil {
			return err
		}

		assignees, err = getMapAssignees(txApp, bestMap.ID, assignmentId)
		return err
	})
	if err != nil {
		return wrapTransactionError(err)
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
// Only counts non-expired "normal" type assignments still in use: minted since
// activeSince, or on a map with an address worked since then. A link outlives
// the publisher's session, so an unexpired one alone does not mean the map is busy.
//
// Activity is judged per map, not per link: an address change records the
// publisher's name, not the link it came through.
//
// Restricted to congregationId — the one the caller was authorized against — so a
// map that ended up under a foreign congregation's territory can never be selected
// and minted into an assignment the caller has no role for.
func getMapsWithAssignmentCount(app core.App, territoryId string, congregationId string, activeSince time.Time) ([]MapWithDistance, error) {
	maps := []MapWithDistance{}

	query := `
		SELECT
			m.id,
			COALESCE(m.description, '') as description,
			COALESCE(m.progress, 0) as progress,
			COALESCE(m.sequence, 0) as sequence,
			COALESCE(m.coordinates, '{}') as coordinates,
			COALESCE(m.aggregates, '{}') as aggregates,
			COUNT(CASE WHEN a.type = 'normal' AND a.expiry_date > datetime('now') AND (
				a.created > {:activeSince}
				OR EXISTS (SELECT 1 FROM addresses_log l WHERE l.map = m.id AND l.created > {:activeSince})
			) THEN a.id END) as assignment_count
		FROM maps m
		LEFT JOIN assignments a ON m.id = a.map
		WHERE m.territory = {:territory} AND m.congregation = {:congregation}
		GROUP BY m.id
	`

	err := app.DB().NewQuery(query).Bind(dbx.Params{
		"territory":    territoryId,
		"congregation": congregationId,
		"activeSince":  activeSince.UTC().Format(types.DefaultDateLayout),
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

// maxActivityWindow caps how long a link counts towards its map's workload
// without activity: long enough for one session, short enough that a morning
// link is not still counted in the evening.
const maxActivityWindow = 6 * time.Hour

// maxDetourMeters is how much further than the nearest map a caller may be sent
// to reach a less busy one. Beyond it they share the nearer map instead.
const maxDetourMeters = 300

// Maps are offered from the lowest tier present in the territory.
const (
	tierUnfinished = iota // below the completion threshold, with addresses left
	tierLeftovers         // at or past the threshold, with addresses left
	tierDone              // no addresses left
)

// mapTier classifies a map and returns how many addresses are left to work.
//
// What is left is total minus completed, the pair progress is derived from; the
// per-status counts stored beside them are stale on older maps. Aggregates
// without that pair (a new map has none) say nothing, so such a map is never
// done. Progress alone cannot tell: a map with no countable addresses sits at 0%.
func mapTier(m *MapWithDistance, completionThreshold int) (tier, remaining int) {
	var counts struct {
		Completed *int `json:"completed"`
		Total     *int `json:"total"`
	}
	if err := json.Unmarshal([]byte(m.Aggregates), &counts); err == nil && counts.Completed != nil && counts.Total != nil {
		remaining = max(*counts.Total-*counts.Completed, 0)
		if remaining == 0 {
			return tierDone, 0
		}
	}
	if m.Progress >= completionThreshold {
		return tierLeftovers, remaining
	}
	return tierUnfinished, remaining
}

// findBestMap selects the map to hand out, or nil when there are none.
// Priority: fewest assignments > proximity (50m threshold) > most addresses left
// > lowest progress > territory map order > id, among the maps of the lowest
// tier within maxDetourMeters of the nearest one. Full ties are common, since
// the maps of one block often share a pin and a size, and the result must not
// depend on the order the query returns rows in.
//
// A map without usable coordinates is the farthest candidate but stays eligible:
// with no distance to measure, the detour limit cannot rule it out.
//
// Uses separate passes to avoid order-dependent results that a single greedy pass
// produces when the 50m proximity window straddles multiple maps at different distances.
func findBestMap(maps []MapWithDistance, currentLat, currentLong float64, completionThreshold int) *MapWithDistance {
	// Pass 1: locate and classify each map.
	lowestTier := tierDone
	for i := range maps {
		m := &maps[i]

		// Pointers tell a missing key apart from a real pin at lat 0, lng 0;
		// `{}` and `null` both unmarshal without error.
		var coords struct {
			Lat *float64 `json:"lat"`
			Lng *float64 `json:"lng"`
		}
		m.Distance = math.Inf(1)
		if err := json.Unmarshal([]byte(m.Coordinates), &coords); err == nil && coords.Lat != nil && coords.Lng != nil {
			m.Distance = haversineDistance(currentLat, currentLong, *coords.Lat, *coords.Lng)
			m.ParsedCoords = &Coordinates{Lat: *coords.Lat, Lng: *coords.Lng}
		}

		m.tier, m.Remaining = mapTier(m, completionThreshold)
		lowestTier = min(lowestTier, m.tier)
	}

	// Pass 2: find the nearest map on offer, which sets how far the caller can be sent.
	nearest := math.Inf(1)
	for i := range maps {
		if m := &maps[i]; m.tier == lowestTier && m.Distance < nearest {
			nearest = m.Distance
		}
	}
	eligible := func(m *MapWithDistance) bool {
		return m.tier == lowestTier && (math.IsInf(m.Distance, 1) || m.Distance <= nearest+maxDetourMeters)
	}

	// Pass 3: find the minimum assignment count.
	minCount := math.MaxInt
	for i := range maps {
		if m := &maps[i]; eligible(m) && m.AssignCount < minCount {
			minCount = m.AssignCount
		}
	}

	// Pass 4: find the minimum distance within the minimum-count cohort. Stays
	// +Inf when none of them has coordinates, which puts them all in the band below.
	minDist := math.Inf(1)
	for i := range maps {
		if m := &maps[i]; eligible(m) && m.AssignCount == minCount && m.Distance < minDist {
			minDist = m.Distance
		}
	}

	// Pass 5: among maps within 50m of the minimum distance, which count as
	// equally close, pick the one with the most left to work.
	var best *MapWithDistance
	for i := range maps {
		m := &maps[i]
		if !eligible(m) || m.AssignCount != minCount || m.Distance > minDist+50 {
			continue
		}
		if best == nil || cmp.Or(
			cmp.Compare(best.Remaining, m.Remaining),
			cmp.Compare(m.Progress, best.Progress),
			cmp.Compare(m.Sequence, best.Sequence),
			cmp.Compare(m.ID, best.ID),
		) < 0 {
			best = m
		}
	}

	return best
}

// getQuicklinkSettings reads the congregation settings a quicklink depends on.
// Results are cached for the process lifetime since they change only via admin action.
// expiry_hours defaults to 24 if not set or not positive.
func getQuicklinkSettings(app core.App, congregationId string) (quicklinkSettings, error) {
	if cached, ok := quicklinkSettingsCache.Load(congregationId); ok {
		return cached.(quicklinkSettings), nil
	}

	congregation, err := app.FindRecordById("congregations", congregationId)
	if err != nil {
		return quicklinkSettings{}, err
	}

	settings := quicklinkSettings{
		expiryHours:         congregation.GetFloat("expiry_hours"),
		completionThreshold: completionThreshold(congregation),
	}
	if settings.expiryHours <= 0 {
		settings.expiryHours = 24
	}

	quicklinkSettingsCache.Store(congregationId, settings)
	return settings, nil
}

// createAssignment creates a new assignment record linking a user to a map with expiry.
func createAssignment(app core.App, mapId string, user *core.Record, publisher, congId string, expiryHours float64) (string, error) {
	collection, err := app.FindCollectionByNameOrId("assignments")
	if err != nil {
		return "", err
	}

	assignment := core.NewRecord(collection)
	assignment.Set("map", mapId)
	assignment.Set("user", user.Id)
	assignment.Set("type", "normal")
	assignment.Set("publisher", publisher)
	assignment.Set("congregation", congId)

	// Multiply before converting: expiry_hours may be fractional.
	expiryDate := time.Now().UTC().Add(time.Duration(expiryHours * float64(time.Hour)))
	assignment.Set("expiry_date", expiryDate)

	if err := app.Save(assignment); err != nil {
		return "", err
	}
	writeAssignmentLog(app, assignment, user, "assigned")

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
