package handlers

import (
	"database/sql"
	"encoding/json"
	"math"
	"os"
	"slices"
	"sort"
	"testing"
)

// TestQuicklinkReplay runs the map selection over every territory of a real
// database and checks the rules that must hold whatever the data looks like.
// It reads the file without writing to it and is skipped unless pointed at one:
//
//	QUICKLINK_REPLAY_DB=data_v2/data.db go test ./internal/handlers/ -run TestQuicklinkReplay -v
func TestQuicklinkReplay(t *testing.T) {
	path := os.Getenv("QUICKLINK_REPLAY_DB")
	if path == "" {
		t.Skip("QUICKLINK_REPLAY_DB not set")
	}
	if !slices.Contains([]string{"/", "."}, path[:1]) {
		path = "../../" + path
	}
	// The driver is registered by PocketBase's core package.
	db, err := sql.Open("sqlite", "file:"+path+"?immutable=1")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	thresholds := map[string]int{}
	rows, err := db.Query(`SELECT id, COALESCE(completion_threshold, 0) FROM congregations`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id string
		var threshold int
		if err := rows.Scan(&id, &threshold); err != nil {
			t.Fatal(err)
		}
		if threshold <= 0 {
			threshold = DefaultCompletionThreshold
		}
		thresholds[id] = threshold
	}
	rows.Close()

	// The same columns and assignment count the handler reads, as of the
	// snapshot's last recorded work rather than the wall clock.
	type territory struct {
		threshold int
		maps      []MapWithDistance
	}
	territories := map[string]*territory{}
	var snapshotAt string
	if err := db.QueryRow(`SELECT MAX(created) FROM addresses_log`).Scan(&snapshotAt); err != nil {
		t.Fatal(err)
	}
	rows, err = db.Query(`
		SELECT m.territory, m.congregation, m.id, COALESCE(m.sequence, 0),
			COALESCE(m.progress, 0), COALESCE(m.coordinates, '{}'), COALESCE(m.aggregates, '{}'),
			(SELECT COUNT(*) FROM assignments a WHERE a.map = m.id AND a.type = 'normal' AND a.expiry_date > ?)
		FROM maps m WHERE m.territory != ''
		ORDER BY m.territory, m.sequence`, snapshotAt)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var terr, cong string
		var m MapWithDistance
		if err := rows.Scan(&terr, &cong, &m.ID, &m.Sequence, &m.Progress, &m.Coordinates, &m.Aggregates, &m.AssignCount); err != nil {
			t.Fatal(err)
		}
		if territories[terr] == nil {
			territories[terr] = &territory{threshold: thresholds[cong]}
		}
		territories[terr].maps = append(territories[terr].maps, m)
	}
	rows.Close()
	if len(territories) == 0 {
		t.Fatal("no territories found")
	}

	// Independent reading of the map's state, so the check does not reuse the
	// code under test: whether any address is left, and whether the map is unfinished.
	hasLeft := func(m MapWithDistance) bool {
		var agg map[string]float64
		if json.Unmarshal([]byte(m.Aggregates), &agg) != nil {
			return true
		}
		total, okT := agg["total"]
		completed, okC := agg["completed"]
		return !okT || !okC || total-completed > 0
	}
	hasWork := func(m MapWithDistance, threshold int) bool {
		return m.Progress < threshold && hasLeft(m)
	}
	pin := func(m MapWithDistance) (lat, lng float64) {
		var c Coordinates
		_ = json.Unmarshal([]byte(m.Coordinates), &c)
		return c.Lat, c.Lng
	}

	var (
		callers, violations                              int
		changed, legacyFinished, legacyFar, tieBreakOnly int
		newDist, oldDist                                 []float64
		sessions, sessionMaps, sessionMaxShare           int
		sessionFurthest                                  []float64
		territoriesAllFinished, territoriesSomeSetAside  int
	)
	fail := func(format string, args ...any) {
		violations++
		if violations <= 20 {
			t.Errorf(format, args...)
		}
	}

	for terrID, terr := range territories {
		anyWork, anyLeft, anyFinished := false, false, false
		for _, m := range terr.maps {
			anyLeft = anyLeft || hasLeft(m)
			if hasWork(m, terr.threshold) {
				anyWork = true
			} else {
				anyFinished = true
			}
		}
		if !anyWork {
			territoriesAllFinished++
		} else if anyFinished {
			territoriesSomeSetAside++
		}
		offered := func(m MapWithDistance) bool {
			switch {
			case anyWork:
				return hasWork(m, terr.threshold)
			case anyLeft:
				return hasLeft(m)
			}
			return true
		}

		// One caller standing at each map's pin.
		for _, at := range terr.maps {
			lat, lng := pin(at)
			callers++

			maps := slices.Clone(terr.maps)
			got := findBestMap(maps, lat, lng, terr.threshold)
			if got == nil {
				fail("territory %s: no map selected", terrID)
				continue
			}
			winner := *got

			nearest := math.Inf(1)
			for _, m := range maps {
				if offered(m) && m.Distance < nearest {
					nearest = m.Distance
				}
			}
			minCount, minDist := math.MaxInt, math.Inf(1)
			for _, m := range maps {
				if offered(m) && m.Distance <= nearest+maxDetourMeters && m.AssignCount < minCount {
					minCount = m.AssignCount
				}
			}
			for _, m := range maps {
				if offered(m) && m.Distance <= nearest+maxDetourMeters && m.AssignCount == minCount && m.Distance < minDist {
					minDist = m.Distance
				}
			}
			switch {
			case !offered(winner):
				fail("territory %s: finished map %s offered while another has work", terrID, winner.ID)
			case winner.Distance > nearest+maxDetourMeters:
				fail("territory %s: sent %.0fm, nearest offered map is %.0fm", terrID, winner.Distance, nearest)
			case winner.AssignCount != minCount:
				fail("territory %s: map %s has %d links, a reachable map has %d", terrID, winner.ID, winner.AssignCount, minCount)
			case winner.Distance > minDist+50:
				fail("territory %s: map %s is %.0fm away, an equally busy one is %.0fm", terrID, winner.ID, winner.Distance, minDist)
			}
			for _, m := range maps {
				if offered(m) && m.Distance <= minDist+50 && m.Distance <= nearest+maxDetourMeters && m.AssignCount == minCount && m.Remaining > winner.Remaining {
					fail("territory %s: map %s has %d left, map %s in the same band has %d", terrID, winner.ID, winner.Remaining, m.ID, m.Remaining)
					break
				}
			}

			reversed := slices.Clone(terr.maps)
			slices.Reverse(reversed)
			if again := findBestMap(reversed, lat, lng, terr.threshold); again.ID != winner.ID {
				fail("territory %s: %s or %s is selected depending on the order the maps are read in", terrID, winner.ID, again.ID)
			}

			// What the selection did before this work, for the size of the change.
			legacy := legacyFindBestMap(slices.Clone(terr.maps), lat, lng)
			newDist = append(newDist, winner.Distance)
			oldDist = append(oldDist, legacy.Distance)
			if legacy.ID != winner.ID {
				changed++
				switch {
				case !offered(*legacy):
					legacyFinished++
				case legacy.Distance > nearest+maxDetourMeters:
					legacyFar++
				default:
					tieBreakOnly++
				}
			}
		}

		// A group of eight (the 90th percentile session) served one after
		// another from the first map's pin.
		lat, lng := pin(terr.maps[0])
		maps := slices.Clone(terr.maps)
		used := map[string]int{}
		furthest := 0.0
		for range 8 {
			got := findBestMap(maps, lat, lng, terr.threshold)
			if got == nil || !offered(*got) {
				fail("territory %s: session got an unusable map", terrID)
				break
			}
			for _, m := range maps {
				if offered(m) && m.Distance <= got.Distance && m.AssignCount < got.AssignCount && !math.IsInf(m.Distance, 1) {
					fail("territory %s: session skipped nearer, less busy map %s for %s", terrID, m.ID, got.ID)
				}
			}
			used[got.ID]++
			furthest = math.Max(furthest, got.Distance)
			got.AssignCount++
		}
		sessions++
		sessionMaps += len(used)
		for _, n := range used {
			if n > sessionMaxShare {
				sessionMaxShare = n
			}
		}
		sessionFurthest = append(sessionFurthest, furthest)
	}

	pct := func(xs []float64, p float64) float64 {
		s := slices.Clone(xs)
		sort.Float64s(s)
		return s[min(len(s)-1, int(float64(len(s))*p))]
	}
	t.Logf("territories: %d (all finished: %d, some maps set aside: %d)", len(territories), territoriesAllFinished, territoriesSomeSetAside)
	t.Logf("callers replayed: %d, rule violations: %d", callers, violations)
	t.Logf("different map than before this work: %d (%.1f%%) - previous choice was finished: %d, was a detour over %dm: %d, tie-break only: %d",
		changed, 100*float64(changed)/float64(callers), legacyFinished, maxDetourMeters, legacyFar, tieBreakOnly)
	t.Logf("distance to assigned map, before: p50 %.0fm p90 %.0fm p99 %.0fm max %.0fm", pct(oldDist, .5), pct(oldDist, .9), pct(oldDist, .99), pct(oldDist, 1))
	t.Logf("distance to assigned map, now:    p50 %.0fm p90 %.0fm p99 %.0fm max %.0fm", pct(newDist, .5), pct(newDist, .9), pct(newDist, .99), pct(newDist, 1))
	t.Logf("group of 8 from one spot: %.1f maps used on average, most links on one map: %d, furthest walk p50 %.0fm p90 %.0fm max %.0fm",
		float64(sessionMaps)/float64(sessions), sessionMaxShare, pct(sessionFurthest, .5), pct(sessionFurthest, .9), pct(sessionFurthest, 1))
}

// legacyFindBestMap is the selection as it was before finished maps, the detour
// limit and remaining work were considered. Kept only so the replay can report
// how many outcomes change.
func legacyFindBestMap(maps []MapWithDistance, currentLat, currentLong float64) *MapWithDistance {
	for i := range maps {
		var coords Coordinates
		if err := json.Unmarshal([]byte(maps[i].Coordinates), &coords); err != nil {
			maps[i].Distance = math.Inf(1)
			continue
		}
		maps[i].Distance = haversineDistance(currentLat, currentLong, coords.Lat, coords.Lng)
	}
	minCount := math.MaxInt32
	for _, m := range maps {
		if !math.IsInf(m.Distance, 1) && m.AssignCount < minCount {
			minCount = m.AssignCount
		}
	}
	minDist := math.Inf(1)
	for _, m := range maps {
		if m.AssignCount == minCount && m.Distance < minDist {
			minDist = m.Distance
		}
	}
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
