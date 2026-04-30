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
