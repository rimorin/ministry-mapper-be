package handlers

import (
	"database/sql"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// TestQuicklinkReplay runs the map selection for a caller at every map's pin in
// a real database and checks the rules that must hold whatever the data looks
// like. It opens the file read-only and is skipped unless pointed at one, by a
// path relative to the repository root:
//
//	QUICKLINK_REPLAY_DB=data_v2/data.db go test ./internal/handlers/ -run TestQuicklinkReplay -v
func TestQuicklinkReplay(t *testing.T) {
	path := os.Getenv("QUICKLINK_REPLAY_DB")
	if path == "" {
		t.Skip("QUICKLINK_REPLAY_DB not set")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join("..", "..", path)
	}
	// The driver is registered by PocketBase's core package.
	db, err := sql.Open("sqlite", "file:"+path+"?immutable=1")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// The columns and assignment count the handler reads, as of the snapshot's
	// last recorded work rather than the wall clock.
	type territory struct {
		threshold int
		maps      []MapWithDistance
	}
	territories := map[string]*territory{}
	rows, err := db.Query(`
		SELECT m.territory, COALESCE(c.completion_threshold, 0), m.id, COALESCE(m.sequence, 0),
			COALESCE(m.progress, 0), COALESCE(m.coordinates, '{}'), COALESCE(m.aggregates, '{}'),
			(SELECT COUNT(*) FROM assignments a WHERE a.map = m.id AND a.type = 'normal'
				AND a.expiry_date > (SELECT MAX(created) FROM addresses_log))
		FROM maps m JOIN congregations c ON c.id = m.congregation
		WHERE m.territory != ''`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var terrID string
		var threshold int
		var m MapWithDistance
		if err := rows.Scan(&terrID, &threshold, &m.ID, &m.Sequence, &m.Progress, &m.Coordinates, &m.Aggregates, &m.AssignCount); err != nil {
			t.Fatal(err)
		}
		if threshold <= 0 {
			threshold = DefaultCompletionThreshold
		}
		if territories[terrID] == nil {
			territories[terrID] = &territory{threshold: threshold}
		}
		territories[terrID].maps = append(territories[terrID].maps, m)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(territories) == 0 {
		t.Fatal("no territories found")
	}

	// The map's tier read independently of mapTier, so the check does not reuse
	// the code under test.
	tierOf := func(m MapWithDistance, threshold int) int {
		var agg map[string]float64
		if json.Unmarshal([]byte(m.Aggregates), &agg) == nil {
			total, hasTotal := agg["total"]
			completed, hasCompleted := agg["completed"]
			if hasTotal && hasCompleted && total-completed <= 0 {
				return tierDone
			}
		}
		if m.Progress >= threshold {
			return tierLeftovers
		}
		return tierUnfinished
	}

	callers, violations := 0, 0
	var distances []float64
	fail := func(format string, args ...any) {
		violations++
		if violations <= 20 {
			t.Errorf(format, args...)
		}
	}

	for terrID, terr := range territories {
		lowest := tierDone
		for _, m := range terr.maps {
			lowest = min(lowest, tierOf(m, terr.threshold))
		}
		offered := func(m MapWithDistance) bool { return tierOf(m, terr.threshold) == lowest }

		for _, at := range terr.maps {
			var here Coordinates
			_ = json.Unmarshal([]byte(at.Coordinates), &here)
			callers++

			maps := slices.Clone(terr.maps)
			got := findBestMap(maps, here.Lat, here.Lng, terr.threshold)
			if got == nil {
				fail("territory %s: no map selected", terrID)
				continue
			}
			winner := *got
			distances = append(distances, winner.Distance)

			nearest := math.Inf(1)
			for _, m := range maps {
				if offered(m) {
					nearest = min(nearest, m.Distance)
				}
			}
			reachable := func(m MapWithDistance) bool { return offered(m) && m.Distance <= nearest+maxDetourMeters }
			minCount := math.MaxInt
			for _, m := range maps {
				if reachable(m) {
					minCount = min(minCount, m.AssignCount)
				}
			}
			minDist := math.Inf(1)
			for _, m := range maps {
				if reachable(m) && m.AssignCount == minCount {
					minDist = min(minDist, m.Distance)
				}
			}

			switch {
			case !offered(winner):
				fail("territory %s: map %s offered while a map of a lower tier exists", terrID, winner.ID)
			case winner.Distance > nearest+maxDetourMeters:
				fail("territory %s: sent %.0fm, nearest offered map is %.0fm", terrID, winner.Distance, nearest)
			case winner.AssignCount != minCount:
				fail("territory %s: map %s has %d links, a reachable map has %d", terrID, winner.ID, winner.AssignCount, minCount)
			case winner.Distance > minDist+50:
				fail("territory %s: map %s is %.0fm away, an equally busy one is %.0fm", terrID, winner.ID, winner.Distance, minDist)
			}
			for _, m := range maps {
				if reachable(m) && m.AssignCount == minCount && m.Distance <= minDist+50 && m.Remaining > winner.Remaining {
					fail("territory %s: map %s has %d left, map %s in the same band has %d", terrID, winner.ID, winner.Remaining, m.ID, m.Remaining)
					break
				}
			}

			reversed := slices.Clone(terr.maps)
			slices.Reverse(reversed)
			if again := findBestMap(reversed, here.Lat, here.Lng, terr.threshold); again.ID != winner.ID {
				fail("territory %s: %s or %s is selected depending on the order the maps are read in", terrID, winner.ID, again.ID)
			}
		}
	}

	slices.Sort(distances)
	at := func(p float64) float64 { return distances[min(len(distances)-1, int(float64(len(distances))*p))] }
	t.Logf("%d territories, %d callers, %d rule violations", len(territories), callers, violations)
	t.Logf("distance to the assigned map: p50 %.0fm, p90 %.0fm, p99 %.0fm, max %.0fm", at(.5), at(.9), at(.99), at(1))
}
