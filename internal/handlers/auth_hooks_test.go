package handlers

import (
	"testing"
)

// Auth scenario matrix — covers all link-aware hooks (authorizeView, authorizeList,
// linkMapListAuth, authOrLink, AuthorizeMapAccess, authorizeMapSubscription).
//
// Scenario | link-id header | Auth / Role          | Expected | Notes
// ─────────┼────────────────┼──────────────────────┼──────────┼──────────────────────────────────────
//  1       | absent         | superuser            | allow    | superuser always bypasses all checks
//  2       | absent         | valid role           | allow    | normal authenticated user path
//  3       | absent         | no role / no auth    | deny     | 403 – Auth required / Unauthorized
//  4       | valid          | no auth              | allow    | publisher-only link access
//  5       | valid          | valid role           | allow    | conductor opens own link; link wins
//  6       | invalid/expired| valid role           | deny     | link present → role ignored, link fails
//  7       | invalid/expired| superuser            | deny     | superuser does NOT bypass link check
//                                                              | (only superuser bypass is before link check)
//
// NOTE: Scenario 7 applies only to hooks that check link-id AFTER the superuser guard.
// All six link-aware hooks guard superuser first, then branch on link-id presence.
// So a superuser without a link-id header still passes (scenario 1 / no link-id path),
// but a superuser who sends an invalid link-id is denied (scenario 6 applies to everyone).

// --- Filter extraction tests (pure logic, no DB required) ---

func TestExtractMapIdFromFilter(t *testing.T) {
	tests := []struct {
		name   string
		filter string
		want   string
	}{
		{
			name:   "standard quoted map id",
			filter: `map = "abc123"`,
			want:   "abc123",
		},
		{
			name:   "map id with extra spacing",
			filter: `map  =  "xyz789"`,
			want:   "xyz789",
		},
		{
			name:   "compound filter with map",
			filter: `map = "mapId01" && status = "not_done"`,
			want:   "mapId01",
		},
		{
			name:   "no map field",
			filter: `congregation = "congId01"`,
			want:   "",
		},
		{
			name:   "empty filter",
			filter: "",
			want:   "",
		},
		{
			name:   "map field without quotes",
			filter: `map = abc123`,
			want:   "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractMapIdFromFilter(tc.filter)
			if got != tc.want {
				t.Errorf("extractMapIdFromFilter(%q) = %q; want %q", tc.filter, got, tc.want)
			}
		})
	}
}

func TestExtractAllMapIdsFromFilter(t *testing.T) {
	tests := []struct {
		name   string
		filter string
		want   []string
	}{
		{
			name:   "single map id",
			filter: `map = "map01"`,
			want:   []string{"map01"},
		},
		{
			name:   "two map ids",
			filter: `map = "map01" || map = "map02"`,
			want:   []string{"map01", "map02"},
		},
		{
			name:   "no map ids",
			filter: `congregation = "cong01"`,
			want:   []string{},
		},
		{
			name:   "empty filter",
			filter: "",
			want:   []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractAllMapIdsFromFilter(tc.filter)
			if len(got) != len(tc.want) {
				t.Errorf("extractAllMapIdsFromFilter(%q) = %v; want %v", tc.filter, got, tc.want)
				return
			}
			wantSet := make(map[string]bool, len(tc.want))
			for _, id := range tc.want {
				wantSet[id] = true
			}
			for _, id := range got {
				if !wantSet[id] {
					t.Errorf("unexpected id %q in result", id)
				}
			}
		})
	}
}

func TestExtractCongIdFromFilter(t *testing.T) {
	tests := []struct {
		name   string
		filter string
		want   string
	}{
		{
			name:   "standard congregation filter",
			filter: `congregation = "cong01"`,
			want:   "cong01",
		},
		{
			name:   "congregation with extra spacing",
			filter: `congregation  =  "cong02"`,
			want:   "cong02",
		},
		{
			name:   "compound filter",
			filter: `congregation = "cong03" && is_default = true`,
			want:   "cong03",
		},
		{
			name:   "no congregation field",
			filter: `map = "map01"`,
			want:   "",
		},
		{
			name:   "empty filter",
			filter: "",
			want:   "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractCongIdFromFilter(tc.filter)
			if got != tc.want {
				t.Errorf("extractCongIdFromFilter(%q) = %q; want %q", tc.filter, got, tc.want)
			}
		})
	}
}

func TestExtractTerritoryIdFromFilter(t *testing.T) {
	tests := []struct {
		name   string
		filter string
		want   string
	}{
		{
			name:   "standard territory filter",
			filter: `territory = "terr01"`,
			want:   "terr01",
		},
		{
			name:   "compound filter",
			filter: `territory = "terr02" && type = "multi"`,
			want:   "terr02",
		},
		{
			name:   "no territory field",
			filter: `congregation = "cong01"`,
			want:   "",
		},
		{
			name:   "empty filter",
			filter: "",
			want:   "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractTerritoryIdFromFilter(tc.filter)
			if got != tc.want {
				t.Errorf("extractTerritoryIdFromFilter(%q) = %q; want %q", tc.filter, got, tc.want)
			}
		})
	}
}

func TestExtractUserIdFromFilter(t *testing.T) {
	tests := []struct {
		name   string
		filter string
		want   string
	}{
		{
			name:   "standard user filter",
			filter: `user = "user01"`,
			want:   "user01",
		},
		{
			name:   "compound filter",
			filter: `user = "user02" && congregation = "cong01"`,
			want:   "user02",
		},
		{
			name:   "no user field",
			filter: `congregation = "cong01"`,
			want:   "",
		},
		{
			name:   "empty filter",
			filter: "",
			want:   "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractUserIdFromFilter(tc.filter)
			if got != tc.want {
				t.Errorf("extractUserIdFromFilter(%q) = %q; want %q", tc.filter, got, tc.want)
			}
		})
	}
}

// TestAuthorizeMapSubscription_NoFilter ensures empty/missing map filter
// is rejected before any DB check.
func TestAuthorizeMapSubscription_NoFilter(t *testing.T) {
	tests := []struct {
		name   string
		filter string
		want   bool
	}{
		{name: "empty filter", filter: "", want: false},
		{name: "no map field", filter: `congregation = "cong01"`, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Pass nil app — must not reach DB when mapId is empty.
			got := authorizeMapSubscription(nil, nil, "", tc.filter)
			if got != tc.want {
				t.Errorf("authorizeMapSubscription(nil, nil, %q, %q) = %v; want %v",
					"", tc.filter, got, tc.want)
			}
		})
	}
}
