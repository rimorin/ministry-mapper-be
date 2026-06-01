package handlers

import (
	"math"
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
	result := findBestMap(nil, 1.3521, 103.8198)
	if result != nil {
		t.Errorf("expected nil for empty maps list, got %+v", result)
	}
}

func TestFindBestMap_SingleMap(t *testing.T) {
	maps := []MapWithDistance{
		{ID: "map1", Progress: 50, AssignCount: 0, Coordinates: `{"lat":1.3521,"lng":103.8198}`},
	}
	result := findBestMap(maps, 1.3521, 103.8198)
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
	result := findBestMap(maps, 1.3521, 103.8198)
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
	result := findBestMap(maps, 1.3544, 103.8198)
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
	result := findBestMap(maps, 1.35215, 103.8198)
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
		result := findBestMap(maps, userLat, userLng)
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

func TestFindBestMap_InvalidCoordinatesSkipped(t *testing.T) {
	maps := []MapWithDistance{
		{ID: "bad", Progress: 10, AssignCount: 0, Coordinates: `not-json`},
		{ID: "good", Progress: 50, AssignCount: 0, Coordinates: `{"lat":1.3521,"lng":103.8198}`},
	}
	result := findBestMap(maps, 1.3521, 103.8198)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.ID != "good" {
		t.Errorf("expected 'good' (invalid coords skipped), got %s", result.ID)
	}
}

// TestFindBestMap_AllInvalidCoordinates checks the nil guard when every map
// has unparseable coordinates.
func TestFindBestMap_AllInvalidCoordinates(t *testing.T) {
	maps := []MapWithDistance{
		{ID: "bad1", Progress: 10, AssignCount: 0, Coordinates: `not-json`},
		{ID: "bad2", Progress: 20, AssignCount: 0, Coordinates: `{invalid}`},
	}
	result := findBestMap(maps, 1.3521, 103.8198)
	if result != nil {
		t.Errorf("expected nil when all coordinates are invalid, got %+v", result)
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
	result := findBestMap(maps, 1.3521, 103.8198)
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
	result := findBestMap(maps, 1.3521, 103.8198)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.ID != "closest" {
		t.Errorf("expected 'closest' (out-of-band map excluded), got %s", result.ID)
	}
}

// TestFindBestMap_InvalidCoordsInLowerCountCohort ensures a map with fewer
// assignments but unparseable coordinates does not prevent a valid higher-count
// map from being selected.
func TestFindBestMap_InvalidCoordsInLowerCountCohort(t *testing.T) {
	maps := []MapWithDistance{
		{ID: "cheap_bad", Progress: 10, AssignCount: 0, Coordinates: `not-json`},
		{ID: "valid",     Progress: 50, AssignCount: 1, Coordinates: `{"lat":1.3521,"lng":103.8198}`},
	}
	result := findBestMap(maps, 1.3521, 103.8198)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.ID != "valid" {
		t.Errorf("expected 'valid' (only parseable map), got %s", result.ID)
	}
}
