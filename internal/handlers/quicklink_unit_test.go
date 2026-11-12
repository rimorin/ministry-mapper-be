package handlers

import (
	"fmt"
	"math"
	"slices"
	"testing"
)

func TestHaversineDistance(t *testing.T) {
	tests := []struct {
		name     string
		lat1     float64
		lon1     float64
		lat2     float64
		lon2     float64
		wantApprox float64 // expected distance in metres (±5% tolerance)
	}{
		{
			name:       "same point returns zero",
			lat1:       1.3521,
			lon1:       103.8198,
			lat2:       1.3521,
			lon2:       103.8198,
			wantApprox: 0,
		},
		{
			name:       "Singapore to Kuala Lumpur (~313 km)",
			lat1:       1.3521,
			lon1:       103.8198,
			lat2:       3.1390,
			lon2:       101.6869,
			wantApprox: 313700,
		},
		{
			name:       "short distance ~100 m",
			lat1:       1.3521,
			lon1:       103.8198,
			lat2:       1.3530,
			lon2:       103.8198,
			wantApprox: 100,
		},
	}

	const tolerance = 0.05 // 5%

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := haversineDistance(tc.lat1, tc.lon1, tc.lat2, tc.lon2)

			if tc.wantApprox == 0 {
				if got != 0 {
					t.Errorf("expected 0, got %f", got)
				}
				return
			}

			diff := math.Abs(got-tc.wantApprox) / tc.wantApprox
			if diff > tolerance {
				t.Errorf("haversineDistance(%.4f, %.4f, %.4f, %.4f) = %.1f, want ~%.1f (within %.0f%%)",
					tc.lat1, tc.lon1, tc.lat2, tc.lon2, got, tc.wantApprox, tolerance*100)
			}
		})
	}
}

func TestFindBestMap_EmptyList(t *testing.T) {
	result := findBestMap(nil, 1.3521, 103.8198, 100)
	if result != nil {
		t.Errorf("expected nil for empty maps list, got %+v", result)
	}
}

func TestFindBestMap_SingleMap(t *testing.T) {
	maps := []MapWithDistance{
		{ID: "map1", Progress: 50, AssignCount: 0, Coordinates: `{"lat":1.3521,"lng":103.8198}`},
	}
	result := findBestMap(maps, 1.3521, 103.8198, 100)
	if result == nil {
		t.Fatal("expected non-nil result for single map")
	}
	if result.ID != "map1" {
		t.Errorf("expected map1, got %s", result.ID)
	}
}

func TestFindBestMap_FewerAssignmentsWins(t *testing.T) {
	maps := []MapWithDistance{
		{ID: "busy", Progress: 10, AssignCount: 3, Coordinates: `{"lat":1.3521,"lng":103.8198}`},
		{ID: "free", Progress: 80, AssignCount: 0, Coordinates: `{"lat":1.3521,"lng":103.8199}`},
	}
	result := findBestMap(maps, 1.3521, 103.8198, 100)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.ID != "free" {
		t.Errorf("expected 'free' (fewer assignments), got %s", result.ID)
	}
}

func TestFindBestMap_SameAssignmentsCloserDistanceWins(t *testing.T) {
	// Place maps more than 50 m apart to trigger the proximity preference
	maps := []MapWithDistance{
		{ID: "far", Progress: 10, AssignCount: 1, Coordinates: `{"lat":1.3521,"lng":103.8198}`},
		{ID: "near", Progress: 90, AssignCount: 1, Coordinates: `{"lat":1.3545,"lng":103.8198}`},
	}
	// User is close to "near"
	result := findBestMap(maps, 1.3544, 103.8198, 100)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.ID != "near" {
		t.Errorf("expected 'near' (closer by >50 m), got %s", result.ID)
	}
}

func TestFindBestMap_SameAssignmentsSameDistanceLowerProgressWins(t *testing.T) {
	// Place maps within 50 m of each other so distance tie-break doesn't trigger
	maps := []MapWithDistance{
		{ID: "high_progress", Progress: 80, AssignCount: 1, Coordinates: `{"lat":1.3521,"lng":103.8198}`},
		{ID: "low_progress", Progress: 20, AssignCount: 1, Coordinates: `{"lat":1.3522,"lng":103.8198}`},
	}
	// User is equidistant (roughly)
	result := findBestMap(maps, 1.35215, 103.8198, 100)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.ID != "low_progress" {
		t.Errorf("expected 'low_progress' (lower progress wins), got %s", result.ID)
	}
}

// TestFindBestMap_OrderIndependent exercises a three-map chain where
// |A-B| ≤ 50m, |B-C| ≤ 50m, but |A-C| > 50m.  The correct winner is B
// (lowest progress within the 50m band around the closest map C), but the
// old single-pass algorithm returned different maps depending on iteration
// order (C for A→B→C, A for C→B→A).
func TestFindBestMap_OrderIndependent(t *testing.T) {
	// All maps due north of the user at increasing distances (~215m, ~260m, ~300m).
	// Distances chosen so |B-C|=45m ≤ 50, |A-B|=40m ≤ 50, |A-C|=85m > 50.
	const (
		userLat = 1.3521
		userLng = 103.8198
	)
	mapA := MapWithDistance{ID: "A", Progress: 10, AssignCount: 0, Coordinates: `{"lat":1.354803,"lng":103.8198}`} // ~300m
	mapB := MapWithDistance{ID: "B", Progress: 30, AssignCount: 0, Coordinates: `{"lat":1.354442,"lng":103.8198}`} // ~260m
	mapC := MapWithDistance{ID: "C", Progress: 50, AssignCount: 0, Coordinates: `{"lat":1.354037,"lng":103.8198}`} // ~215m

	orderings := [][]MapWithDistance{
		{mapA, mapB, mapC},
		{mapA, mapC, mapB},
		{mapB, mapA, mapC},
		{mapB, mapC, mapA},
		{mapC, mapA, mapB},
		{mapC, mapB, mapA},
	}

	for _, maps := range orderings {
		result := findBestMap(maps, userLat, userLng, 100)
		if result == nil {
			t.Fatal("expected non-nil result")
		}
		// C is closest; B and C are within 50m of each other; B has lower progress → B wins.
		if result.ID != "B" {
			t.Errorf("order [%s %s %s]: expected B, got %s",
				maps[0].ID, maps[1].ID, maps[2].ID, result.ID)
		}
	}
}

// Every way a map can lack a usable pin. `{}` and `null` parse without error,
// so they must be told apart from a real pin at lat 0, lng 0.
var unlocatedCoordinates = []string{``, `null`, `{}`, `{"lat":1.3521}`, `not-json`}

func TestFindBestMap_UnlocatedRanksBehindLocated(t *testing.T) {
	for _, raw := range unlocatedCoordinates {
		maps := []MapWithDistance{
			{ID: "unlocated", Progress: 10, AssignCount: 0, Coordinates: raw},
			{ID: "located", Progress: 50, AssignCount: 0, Coordinates: `{"lat":1.3600,"lng":103.8198}`}, // ~880m
		}
		result := findBestMap(maps, 1.3521, 103.8198, 100)
		if result == nil {
			t.Fatalf("coordinates %q: expected non-nil result", raw)
		}
		if result.ID != "located" {
			t.Errorf("coordinates %q: expected 'located', got %s", raw, result.ID)
		}
	}
}

// A missing pin must not block assignment: with no located map to prefer, the
// usual tie-break decides.
func TestFindBestMap_AllUnlocated(t *testing.T) {
	maps := []MapWithDistance{
		{ID: "more_done", Progress: 20, AssignCount: 0, Coordinates: `not-json`},
		{ID: "less_done", Progress: 10, AssignCount: 0, Coordinates: `null`},
	}
	result := findBestMap(maps, 1.3521, 103.8198, 100)
	if result == nil {
		t.Fatal("expected a map even though none has coordinates")
	}
	if result.ID != "less_done" {
		t.Errorf("expected 'less_done', got %s", result.ID)
	}
	if result.ParsedCoords != nil {
		t.Errorf("expected nil ParsedCoords for an unlocated map, got %+v", result.ParsedCoords)
	}
}

// Workload still outranks location: an idle map without a pin beats a located
// map that already has a publisher.
func TestFindBestMap_UnlocatedIdleBeatsLocatedBusy(t *testing.T) {
	for _, raw := range unlocatedCoordinates {
		maps := []MapWithDistance{
			{ID: "unlocated_idle", Progress: 10, AssignCount: 0, Coordinates: raw},
			{ID: "located_busy", Progress: 50, AssignCount: 1, Coordinates: `{"lat":1.3521,"lng":103.8198}`},
		}
		result := findBestMap(maps, 1.3521, 103.8198, 100)
		if result == nil {
			t.Fatalf("coordinates %q: expected non-nil result", raw)
		}
		if result.ID != "unlocated_idle" {
			t.Errorf("coordinates %q: expected 'unlocated_idle', got %s", raw, result.ID)
		}
	}
}

func TestFindBestMap_ZeroZeroIsARealLocation(t *testing.T) {
	maps := []MapWithDistance{
		{ID: "origin", AssignCount: 0, Coordinates: `{"lat":0,"lng":0}`},
		{ID: "unlocated", AssignCount: 0, Coordinates: `{}`},
	}
	result := findBestMap(maps, 0.0001, 0.0001, 100)
	if result == nil || result.ID != "origin" {
		t.Fatalf("expected 'origin', got %+v", result)
	}
	if result.ParsedCoords == nil {
		t.Error("expected ParsedCoords for a pin at 0,0")
	}
}

// TestFindBestMap_ProximityBandIncludes verifies that a map within 50m of the
// closest map enters the progress comparison and wins when it has lower progress.
// Map A is at the user's position (0m); Map B is ~30m away with lower progress.
// Both are within the 50m band → B should win.
func TestFindBestMap_ProximityBandIncludes(t *testing.T) {
	maps := []MapWithDistance{
		{ID: "closest", Progress: 80, AssignCount: 0, Coordinates: `{"lat":1.3521,"lng":103.8198}`},   // 0m
		{ID: "in_band", Progress: 20, AssignCount: 0, Coordinates: `{"lat":1.35237,"lng":103.8198}`}, // ~30m
	}
	result := findBestMap(maps, 1.3521, 103.8198, 100)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.ID != "in_band" {
		t.Errorf("expected 'in_band' (within 50m, lower progress wins), got %s", result.ID)
	}
}

// TestFindBestMap_ProximityBandExcludes verifies that a map beyond 50m of the
// closest map is excluded from the progress comparison.
// Map A is at the user's position (0m, high progress); Map B is ~100m away (low progress).
// B is outside the 50m band → A wins despite higher progress.
func TestFindBestMap_ProximityBandExcludes(t *testing.T) {
	maps := []MapWithDistance{
		{ID: "closest",   Progress: 80, AssignCount: 0, Coordinates: `{"lat":1.3521,"lng":103.8198}`},  // 0m
		{ID: "out_band",  Progress: 20, AssignCount: 0, Coordinates: `{"lat":1.35300,"lng":103.8198}`}, // ~100m
	}
	result := findBestMap(maps, 1.3521, 103.8198, 100)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.ID != "closest" {
		t.Errorf("expected 'closest' (out-of-band map excluded), got %s", result.ID)
	}
}

func aggregatesJSON(notDone, notHome int) string {
	return fmt.Sprintf(`{"notDone":%d,"notHome":%d,"done":5,"completed":5,"total":%d}`, notDone, notHome, notDone+notHome+5)
}

const (
	atUser   = `{"lat":1.3521,"lng":103.8198}`  // 0m
	near30m  = `{"lat":1.35237,"lng":103.8198}` // ~30m
	away100m = `{"lat":1.3530,"lng":103.8198}`  // ~100m
)

// A map with nothing left to do is not offered while another still has work,
// even when it is idle, closest and the other already has publishers.
func TestFindBestMap_FinishedMapNotOffered(t *testing.T) {
	maps := []MapWithDistance{
		{ID: "finished", Progress: 100, AssignCount: 0, Coordinates: atUser, Aggregates: aggregatesJSON(0, 0)},
		{ID: "has_work", Progress: 40, AssignCount: 2, Coordinates: away100m, Aggregates: aggregatesJSON(3, 1)},
	}
	result := findBestMap(maps, 1.3521, 103.8198, 100)
	if result == nil || result.ID != "has_work" {
		t.Fatalf("expected 'has_work', got %+v", result)
	}
}

// Addresses still to be revisited (not home, tries left) count as work.
func TestFindBestMap_NotHomeOnlyCountsAsWork(t *testing.T) {
	maps := []MapWithDistance{
		{ID: "finished", Progress: 100, AssignCount: 0, Coordinates: atUser, Aggregates: aggregatesJSON(0, 0)},
		{ID: "revisits", Progress: 90, AssignCount: 1, Coordinates: away100m, Aggregates: aggregatesJSON(0, 2)},
	}
	result := findBestMap(maps, 1.3521, 103.8198, 100)
	if result == nil || result.ID != "revisits" {
		t.Fatalf("expected 'revisits', got %+v", result)
	}
}

// When the whole territory is finished the request still gets a map, ranked the
// usual way.
func TestFindBestMap_AllFinishedFallsBackToAll(t *testing.T) {
	maps := []MapWithDistance{
		{ID: "busy", Progress: 100, AssignCount: 1, Coordinates: atUser, Aggregates: aggregatesJSON(0, 0)},
		{ID: "far", Progress: 100, AssignCount: 0, Coordinates: away100m, Aggregates: aggregatesJSON(0, 0)},
		{ID: "close", Progress: 100, AssignCount: 0, Coordinates: near30m, Aggregates: aggregatesJSON(0, 0)},
	}
	result := findBestMap(maps, 1.3521, 103.8198, 100)
	if result == nil || result.ID != "close" {
		t.Fatalf("expected 'close', got %+v", result)
	}
}

// Aggregates that were never calculated say nothing about remaining work, so
// such a map must not be mistaken for a finished one.
func TestFindBestMap_UncountedMapIsNotTreatedAsFinished(t *testing.T) {
	for _, raw := range []string{``, `null`, `{}`, `not-json`} {
		maps := []MapWithDistance{
			{ID: "uncounted", Progress: 0, AssignCount: 0, Coordinates: atUser, Aggregates: raw},
			{ID: "has_work", Progress: 40, AssignCount: 1, Coordinates: atUser, Aggregates: aggregatesJSON(3, 0)},
		}
		result := findBestMap(maps, 1.3521, 103.8198, 100)
		if result == nil || result.ID != "uncounted" {
			t.Errorf("aggregates %q: expected 'uncounted' (fewer assignments), got %+v", raw, result)
		}
	}
}

// Within the 50m band the map with more left to do wins, whatever its
// percentage: a large map at 50% holds more work than a small untouched one.
func TestFindBestMap_MoreRemainingWinsWithinBand(t *testing.T) {
	maps := []MapWithDistance{
		{ID: "small_untouched", Progress: 0, AssignCount: 0, Coordinates: atUser, Aggregates: aggregatesJSON(10, 0)},
		{ID: "large_half_done", Progress: 50, AssignCount: 0, Coordinates: near30m, Aggregates: aggregatesJSON(90, 10)},
	}
	result := findBestMap(maps, 1.3521, 103.8198, 100)
	if result == nil || result.ID != "large_half_done" {
		t.Fatalf("expected 'large_half_done', got %+v", result)
	}
}

func TestFindBestMap_RemainingDoesNotOverrideDistance(t *testing.T) {
	maps := []MapWithDistance{
		{ID: "close_small", Progress: 0, AssignCount: 0, Coordinates: atUser, Aggregates: aggregatesJSON(10, 0)},
		{ID: "far_large", Progress: 0, AssignCount: 0, Coordinates: away100m, Aggregates: aggregatesJSON(200, 0)},
	}
	result := findBestMap(maps, 1.3521, 103.8198, 100)
	if result == nil || result.ID != "close_small" {
		t.Fatalf("expected 'close_small', got %+v", result)
	}
}

func TestFindBestMap_RemainingDoesNotOverrideWorkload(t *testing.T) {
	maps := []MapWithDistance{
		{ID: "idle_small", Progress: 0, AssignCount: 0, Coordinates: away100m, Aggregates: aggregatesJSON(10, 0)},
		{ID: "busy_large", Progress: 0, AssignCount: 1, Coordinates: atUser, Aggregates: aggregatesJSON(200, 0)},
	}
	result := findBestMap(maps, 1.3521, 103.8198, 100)
	if result == nil || result.ID != "idle_small" {
		t.Fatalf("expected 'idle_small', got %+v", result)
	}
}

func TestFindBestMap_EqualRemainingLowerProgressWins(t *testing.T) {
	maps := []MapWithDistance{
		{ID: "further_along", Progress: 80, AssignCount: 0, Coordinates: atUser, Aggregates: aggregatesJSON(10, 0)},
		{ID: "less_done", Progress: 20, AssignCount: 0, Coordinates: near30m, Aggregates: aggregatesJSON(8, 2)},
	}
	result := findBestMap(maps, 1.3521, 103.8198, 100)
	if result == nil || result.ID != "less_done" {
		t.Fatalf("expected 'less_done', got %+v", result)
	}
}

// The winner must not depend on the order the query returns the maps in.
func TestFindBestMap_OrderIndependentAcrossAllRules(t *testing.T) {
	base := []MapWithDistance{
		{ID: "finished_idle", Progress: 100, AssignCount: 0, Coordinates: atUser, Aggregates: aggregatesJSON(0, 0)},
		{ID: "busy", Progress: 10, AssignCount: 2, Coordinates: atUser, Aggregates: aggregatesJSON(50, 0)},
		{ID: "unlocated", Progress: 0, AssignCount: 1, Coordinates: `null`, Aggregates: aggregatesJSON(99, 0)},
		{ID: "far", Progress: 0, AssignCount: 1, Coordinates: away100m, Aggregates: aggregatesJSON(99, 0)},
		{ID: "close_small", Progress: 0, AssignCount: 1, Coordinates: atUser, Aggregates: aggregatesJSON(5, 0)},
		{ID: "winner", Progress: 60, AssignCount: 1, Coordinates: near30m, Aggregates: aggregatesJSON(20, 5)},
	}
	for start := range base {
		for _, reverse := range []bool{false, true} {
			maps := make([]MapWithDistance, 0, len(base))
			for i := range base {
				maps = append(maps, base[(start+i)%len(base)])
			}
			if reverse {
				slices.Reverse(maps)
			}
			result := findBestMap(maps, 1.3521, 103.8198, 100)
			if result == nil || result.ID != "winner" {
				t.Errorf("rotation %d reverse=%v: expected 'winner', got %+v", start, reverse, result)
			}
		}
	}
}

// With the threshold at 90, a map at 90% or above is finished even though it
// still has addresses left, and one at 89% is not.
func TestFindBestMap_ThresholdBoundary(t *testing.T) {
	cases := []struct {
		progress int
		want     string
	}{
		{89, "nearly_done"},
		{90, "has_work"},
		{97, "has_work"},
		{100, "has_work"},
	}
	for _, tc := range cases {
		maps := []MapWithDistance{
			{ID: "nearly_done", Progress: tc.progress, AssignCount: 0, Coordinates: atUser, Aggregates: aggregatesJSON(6, 0)},
			{ID: "has_work", Progress: 40, AssignCount: 2, Coordinates: away100m, Aggregates: aggregatesJSON(30, 0)},
		}
		result := findBestMap(maps, 1.3521, 103.8198, 90)
		if result == nil || result.ID != tc.want {
			t.Errorf("progress %d: expected %q, got %+v", tc.progress, tc.want, result)
		}
	}
}

// A congregation that sets the threshold to 100 keeps offering a map until it
// reaches 100%.
func TestFindBestMap_Threshold100OffersNearlyDoneMap(t *testing.T) {
	maps := []MapWithDistance{
		{ID: "nearly_done", Progress: 97, AssignCount: 0, Coordinates: atUser, Aggregates: aggregatesJSON(6, 0)},
		{ID: "has_work", Progress: 40, AssignCount: 2, Coordinates: away100m, Aggregates: aggregatesJSON(30, 0)},
	}
	result := findBestMap(maps, 1.3521, 103.8198, 100)
	if result == nil || result.ID != "nearly_done" {
		t.Fatalf("expected 'nearly_done', got %+v", result)
	}
}

// Once every map is past the threshold their leftover addresses are offered
// again, ranked the usual way.
func TestFindBestMap_AllPastThresholdFallsBackToAll(t *testing.T) {
	maps := []MapWithDistance{
		{ID: "busy", Progress: 91, AssignCount: 1, Coordinates: atUser, Aggregates: aggregatesJSON(40, 0)},
		{ID: "far", Progress: 92, AssignCount: 0, Coordinates: away100m, Aggregates: aggregatesJSON(40, 0)},
		{ID: "close_less_left", Progress: 99, AssignCount: 0, Coordinates: atUser, Aggregates: aggregatesJSON(1, 0)},
		{ID: "close_more_left", Progress: 95, AssignCount: 0, Coordinates: near30m, Aggregates: aggregatesJSON(8, 2)},
	}
	result := findBestMap(maps, 1.3521, 103.8198, 90)
	if result == nil || result.ID != "close_more_left" {
		t.Fatalf("expected 'close_more_left', got %+v", result)
	}
}

// A map with no countable addresses sits at 0%, below any threshold, but has
// nothing to offer.
func TestFindBestMap_EmptyMapIsFinishedDespiteZeroProgress(t *testing.T) {
	maps := []MapWithDistance{
		{ID: "empty", Progress: 0, AssignCount: 0, Coordinates: atUser, Aggregates: `{"notDone":0,"notHome":0,"done":0,"completed":0,"total":0}`},
		{ID: "has_work", Progress: 40, AssignCount: 1, Coordinates: away100m, Aggregates: aggregatesJSON(30, 0)},
	}
	result := findBestMap(maps, 1.3521, 103.8198, 90)
	if result == nil || result.ID != "has_work" {
		t.Fatalf("expected 'has_work', got %+v", result)
	}
}

// Uncounted aggregates still do not make a map finished under a threshold.
func TestFindBestMap_UncountedMapBelowThresholdHasWork(t *testing.T) {
	maps := []MapWithDistance{
		{ID: "uncounted", Progress: 0, AssignCount: 0, Coordinates: atUser, Aggregates: `null`},
		{ID: "has_work", Progress: 40, AssignCount: 1, Coordinates: atUser, Aggregates: aggregatesJSON(30, 0)},
	}
	result := findBestMap(maps, 1.3521, 103.8198, 90)
	if result == nil || result.ID != "uncounted" {
		t.Fatalf("expected 'uncounted', got %+v", result)
	}
}

// 1 degree of latitude is ~111,195m, so these sit due north of the user at the
// stated distances.
const (
	away250m = `{"lat":1.354348,"lng":103.8198}`
	away295m = `{"lat":1.354753,"lng":103.8198}`
	away305m = `{"lat":1.354843,"lng":103.8198}`
	away1km  = `{"lat":1.361093,"lng":103.8198}`
	away1200 = `{"lat":1.362892,"lng":103.8198}`
	away2km  = `{"lat":1.370086,"lng":103.8198}`
)

// Nobody is sent more than 300m past the nearest map just to avoid sharing it.
func TestFindBestMap_SharesNearbyMapRatherThanLongDetour(t *testing.T) {
	maps := []MapWithDistance{
		{ID: "near_busy", Progress: 10, AssignCount: 1, Coordinates: atUser, Aggregates: aggregatesJSON(50, 0)},
		{ID: "far_idle", Progress: 0, AssignCount: 0, Coordinates: away2km, Aggregates: aggregatesJSON(50, 0)},
	}
	result := findBestMap(maps, 1.3521, 103.8198, 90)
	if result == nil || result.ID != "near_busy" {
		t.Fatalf("expected 'near_busy', got %+v", result)
	}
}

func TestFindBestMap_DetourBoundary(t *testing.T) {
	cases := []struct {
		idleAt string
		want   string
	}{
		{away250m, "idle"},
		{away295m, "idle"},
		{away305m, "near_busy"},
		{away1km, "near_busy"},
	}
	for _, tc := range cases {
		maps := []MapWithDistance{
			{ID: "near_busy", Progress: 10, AssignCount: 3, Coordinates: atUser, Aggregates: aggregatesJSON(50, 0)},
			{ID: "idle", Progress: 0, AssignCount: 0, Coordinates: tc.idleAt, Aggregates: aggregatesJSON(50, 0)},
		}
		result := findBestMap(maps, 1.3521, 103.8198, 90)
		if result == nil || result.ID != tc.want {
			t.Errorf("idle map at %s: expected %q, got %+v", tc.idleAt, tc.want, result)
		}
	}
}

// The 300m is measured from the nearest map that can still be offered, so a
// finished map next to the caller does not pin them to it.
func TestFindBestMap_DetourMeasuredFromNearestUnfinishedMap(t *testing.T) {
	maps := []MapWithDistance{
		{ID: "finished_here", Progress: 100, AssignCount: 0, Coordinates: atUser, Aggregates: aggregatesJSON(0, 0)},
		{ID: "busy_1km", Progress: 10, AssignCount: 1, Coordinates: away1km, Aggregates: aggregatesJSON(50, 0)},
		{ID: "idle_1200m", Progress: 10, AssignCount: 0, Coordinates: away1200, Aggregates: aggregatesJSON(50, 0)},
		{ID: "idle_2km", Progress: 10, AssignCount: 0, Coordinates: away2km, Aggregates: aggregatesJSON(500, 0)},
	}
	result := findBestMap(maps, 1.3521, 103.8198, 90)
	if result == nil || result.ID != "idle_1200m" {
		t.Fatalf("expected 'idle_1200m', got %+v", result)
	}
}

// Within reach, the least busy map still wins; beyond reach it is ignored.
func TestFindBestMap_LeastBusyWithinReach(t *testing.T) {
	maps := []MapWithDistance{
		{ID: "here_two", Progress: 10, AssignCount: 2, Coordinates: atUser, Aggregates: aggregatesJSON(50, 0)},
		{ID: "250m_one", Progress: 10, AssignCount: 1, Coordinates: away250m, Aggregates: aggregatesJSON(50, 0)},
		{ID: "1km_none", Progress: 10, AssignCount: 0, Coordinates: away1km, Aggregates: aggregatesJSON(50, 0)},
	}
	result := findBestMap(maps, 1.3521, 103.8198, 90)
	if result == nil || result.ID != "250m_one" {
		t.Fatalf("expected '250m_one', got %+v", result)
	}
}

// A map without a pin cannot be measured, so the limit does not rule it out:
// otherwise it would never be offered while any located map has work.
func TestFindBestMap_DetourLimitDoesNotExcludeUnlocatedMap(t *testing.T) {
	maps := []MapWithDistance{
		{ID: "near_busy", Progress: 10, AssignCount: 1, Coordinates: atUser, Aggregates: aggregatesJSON(50, 0)},
		{ID: "far_idle", Progress: 0, AssignCount: 0, Coordinates: away2km, Aggregates: aggregatesJSON(50, 0)},
		{ID: "unlocated_idle", Progress: 0, AssignCount: 0, Coordinates: `null`, Aggregates: aggregatesJSON(50, 0)},
	}
	result := findBestMap(maps, 1.3521, 103.8198, 90)
	if result == nil || result.ID != "unlocated_idle" {
		t.Fatalf("expected 'unlocated_idle', got %+v", result)
	}
}

// Production stores aggregates in several shapes, and on older maps the
// per-status counts are stale while completed/total stay correct: those are what
// progress and the territory rollup are derived from. Remaining work must be
// read from completed/total.
func TestFindBestMap_RemainingComesFromCompletedAndTotal(t *testing.T) {
	cases := []struct {
		name       string
		aggregates string
		progress   int
		wantWork   bool
		wantLeft   int
	}{
		{"stale status counts say none left, 360 actually left",
			`{"notDone":0,"notHome":0,"dnc":0,"invalid":0,"done":0,"completed":0,"total":360}`, 0, true, 360},
		{"status counts overstate what is left",
			`{"dnc":0,"done":0,"invalid":10,"notDone":98,"notHome":0,"completed":0,"total":97}`, 0, true, 97},
		{"only completed and total stored",
			`{"completed":12,"total":40}`, 30, true, 28},
		{"only completed and total stored, all completed",
			`{"completed":40,"total":40}`, 100, false, 0},
		{"status counts positive but nothing countable left",
			`{"notDone":3,"notHome":0,"dnc":0,"invalid":0,"done":20,"completed":20,"total":20}`, 100, false, 0},
		{"legacy shape without completed/total is unknown, not finished",
			`{"dnc":0,"done":0,"invalid":0,"notDone":0,"notHome":0}`, 0, true, 0},
		{"no aggregates at all, as on a newly created map",
			``, 0, true, 0},
		{"no countable addresses",
			`{"completed":0,"dnc":0,"done":0,"invalid":1,"notDone":0,"notHome":0,"total":0}`, 0, false, 0},
	}
	for _, tc := range cases {
		maps := []MapWithDistance{
			{ID: "subject", Progress: tc.progress, AssignCount: 0, Coordinates: atUser, Aggregates: tc.aggregates},
			{ID: "other", Progress: 40, AssignCount: 1, Coordinates: away100m, Aggregates: `{"completed":20,"total":50}`},
		}
		result := findBestMap(maps, 1.3521, 103.8198, 90)
		want := "other"
		if tc.wantWork {
			want = "subject"
		}
		if result == nil || result.ID != want {
			t.Errorf("%s: expected %q, got %+v", tc.name, want, result)
		}
		if got := maps[0].Remaining; got != tc.wantLeft {
			t.Errorf("%s: Remaining = %d, want %d", tc.name, got, tc.wantLeft)
		}
	}
}

// Maps of one block often share a pin and a size, so full ties are common. The
// territory's own map order settles them, then the id, whatever order the rows
// arrive in.
func TestFindBestMap_FullTieFollowsMapSequenceThenID(t *testing.T) {
	base := []MapWithDistance{
		{ID: "c", Sequence: 3, Coordinates: atUser, Aggregates: aggregatesJSON(40, 0)},
		{ID: "b", Sequence: 1, Coordinates: atUser, Aggregates: aggregatesJSON(40, 0)},
		{ID: "a", Sequence: 2, Coordinates: atUser, Aggregates: aggregatesJSON(40, 0)},
		{ID: "d", Sequence: 1, Coordinates: atUser, Aggregates: aggregatesJSON(40, 0)},
	}
	for start := range base {
		for _, reverse := range []bool{false, true} {
			maps := make([]MapWithDistance, 0, len(base))
			for i := range base {
				maps = append(maps, base[(start+i)%len(base)])
			}
			if reverse {
				slices.Reverse(maps)
			}
			result := findBestMap(maps, 1.3521, 103.8198, 90)
			if result == nil || result.ID != "b" {
				t.Errorf("rotation %d reverse=%v: expected 'b' (lowest sequence, then id), got %+v", start, reverse, result)
			}
		}
	}
}
