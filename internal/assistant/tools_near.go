package assistant

import (
	"encoding/json"
	"math"
	"sort"
	"strings"

	"ganigeorgiev.com/pocketbase/dbx"
)

// maxNearMeters bounds a search radius: a congregation's territories span a
// few kilometres, so anything wider is a mistake in the arguments.
const maxNearMeters = 20000

// needLocationError means the admin asked about their own position without
// sharing it. The model is told to ask for it, and the response says so, so
// the panel can offer a button that shares the location and asks again.
type needLocationError struct{}

func (needLocationError) Error() string {
	return "the admin's location is not available; tell the admin to share their location and ask again"
}

// distanceMeters is the great-circle distance between two points.
func distanceMeters(a, b point) float64 {
	const earthRadius = 6371000.0
	lat1, lat2 := a.Lat*math.Pi/180, b.Lat*math.Pi/180
	dLat := lat2 - lat1
	dLng := (b.Lng - a.Lng) * math.Pi / 180
	h := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1)*math.Cos(lat2)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * earthRadius * math.Asin(math.Sqrt(h))
}

// placeOrigin resolves a named place to a point: a map's location, or a
// territory's centre (its boundary's centre, else the average of its maps).
// mapID is set when the place is a map.
func placeOrigin(s Scope, name string) (origin point, mapID string, err error) {
	name = strings.TrimSpace(name)
	if mapID, err := mapIDByName(s, "", &name); err == nil {
		var location string
		if err := s.App.DB().NewQuery(`SELECT COALESCE(coordinates, '') FROM maps WHERE id = {:id}`).
			Bind(dbx.Params{"id": mapID}).Row(&location); err != nil {
			return point{}, "", err
		}
		if p, ok := parsePoint(location); ok {
			return p, mapID, nil
		}
		return point{}, "", badArgs("the map %q has no location", name)
	}
	territoryID, err := territoryIDByCode(s, &name)
	if err != nil {
		return point{}, "", badArgs("no map or territory is named %q; use find_places to look it up", name)
	}
	var rows []struct {
		Boundary string `db:"boundary"`
		Location string `db:"location"`
	}
	if err := s.App.DB().NewQuery(`
		SELECT COALESCE(t.coordinates, '') AS boundary, COALESCE(m.coordinates, '') AS location
		FROM territories t LEFT JOIN maps m ON m.territory = t.id
		WHERE t.id = {:id}`).Bind(dbx.Params{"id": territoryID}).All(&rows); err != nil {
		return point{}, "", err
	}
	if len(rows) > 0 {
		if boundary := parsePolygon(rows[0].Boundary); len(boundary) >= 3 {
			return polygonCentre(boundary), "", nil
		}
	}
	var sum point
	n := 0
	for _, r := range rows {
		if p, ok := parsePoint(r.Location); ok {
			sum.Lat, sum.Lng, n = sum.Lat+p.Lat, sum.Lng+p.Lng, n+1
		}
	}
	if n == 0 {
		return point{}, "", badArgs("the territory %q has no location", name)
	}
	return point{sum.Lat / float64(n), sum.Lng / float64(n)}, "", nil
}

// polygonCentre averages the vertices; good enough to aim from for a
// territory a few hundred metres across.
func polygonCentre(polygon []point) point {
	var sum point
	for _, p := range polygon {
		sum.Lat += p.Lat
		sum.Lng += p.Lng
	}
	return point{sum.Lat / float64(len(polygon)), sum.Lng / float64(len(polygon))}
}

var mapsNear = Tool{
	Name: "maps_near",
	Description: "Maps closest to a place or to the admin, nearest first, with the straight-line distance in " +
		"metres. Use it for \"maps near Blk 412\", \"what is free near me\" or \"closest maps to M06\". " +
		"For any question about the admin's own position (\"near me\", \"closest to me\") call it with " +
		"from my_location even if no location has been shared: the call is what lets the admin share it.",
	Params: object(map[string]any{
		"from":            enumOf("my_location for the admin's position, place for a named map or territory.", "my_location", "place"),
		"place":           nullable("string", "With place: an exact map name or territory code. Use find_places first if unsure."),
		"within_meters":   nullable("integer", "Only maps within this straight-line distance, at most 20000."),
		"unassigned_only": nullable("boolean", "Only maps with no active share link."),
		"min_not_done":    nullable("integer", "Only maps with at least this many homes not done."),
		"limit":           limitParam(),
	}),
	Run: func(s Scope, raw json.RawMessage) (Result, error) {
		var args struct {
			From           string  `json:"from"`
			Place          *string `json:"place"`
			WithinMeters   *int    `json:"within_meters"`
			UnassignedOnly *bool   `json:"unassigned_only"`
			MinNotDone     *int    `json:"min_not_done"`
			Limit          *int    `json:"limit"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return Result{}, err
		}
		if args.WithinMeters != nil && (*args.WithinMeters < 1 || *args.WithinMeters > maxNearMeters) {
			return Result{}, badArgs("within_meters must be between 1 and %d", maxNearMeters)
		}

		var origin point
		var excludeMap string
		switch args.From {
		case "my_location":
			if s.Origin == nil {
				return Result{}, needLocationError{}
			}
			origin = *s.Origin
		case "place":
			if args.Place == nil || strings.TrimSpace(*args.Place) == "" {
				return Result{}, badArgs("place is required with from place")
			}
			// "Maps near Blk 412" should not list Blk 412 itself first.
			p, mapID, err := placeOrigin(s, *args.Place)
			if err != nil {
				return Result{}, err
			}
			origin, excludeMap = p, mapID
		default:
			return Result{}, badArgs("from must be my_location or place")
		}

		var maps []struct {
			ID          string `db:"id"`
			Map         string `db:"map"`
			TerritoryID string `db:"territory_id"`
			Territory   string `db:"territory"`
			Location    string `db:"location"`
			Progress    int    `db:"progress"`
			NotDone     int    `db:"not_done"`
			ActiveLinks int    `db:"active_links"`
		}
		err := s.App.DB().NewQuery(`
			SELECT m.id, m.description AS map, t.id AS territory_id, t.code AS territory,
			       COALESCE(m.coordinates, '') AS location, COALESCE(m.progress, 0) AS progress,
			       COALESCE(` + agg("notDone") + `, 0) AS not_done,
			       (SELECT COUNT(*) FROM assignments a WHERE a.map = m.id AND a.expiry_date > {:now}) AS active_links
			FROM maps m JOIN territories t ON t.id = m.territory
			WHERE m.congregation = {:congregation}
		`).Bind(dbx.Params{"congregation": s.CongregationID, "now": dbTime(s.Now)}).All(&maps)
		if err != nil {
			return Result{}, err
		}

		r := Result{
			Columns: []string{"territory", "map", "distance_m", "progress", "not_done", "active_links"},
			Note:    "Distances are in a straight line, not walking distance.",
		}
		for _, m := range maps {
			loc, ok := parsePoint(m.Location)
			if !ok || m.ID == excludeMap {
				continue
			}
			distance := int(math.Round(distanceMeters(origin, loc)/10) * 10)
			switch {
			case args.WithinMeters != nil && distance > *args.WithinMeters,
				args.UnassignedOnly != nil && *args.UnassignedOnly && m.ActiveLinks > 0,
				args.MinNotDone != nil && m.NotDone < *args.MinNotDone:
				continue
			}
			r.Rows = append(r.Rows, map[string]any{
				"map_id": m.ID, "map": m.Map, "territory_id": m.TerritoryID, "territory": m.Territory,
				"distance_m": distance, "progress": m.Progress, "not_done": m.NotDone, "active_links": m.ActiveLinks,
				// For the panel's map only; not in Columns, so the model never sees them.
				"lat": loc.Lat, "lng": loc.Lng,
			})
		}
		sort.SliceStable(r.Rows, func(i, j int) bool { return r.Rows[i]["distance_m"].(int) < r.Rows[j]["distance_m"].(int) })
		limit := 10
		if args.Limit != nil {
			limit = clampLimit(args.Limit)
		}
		return truncate(r, limit), nil
	},
}
