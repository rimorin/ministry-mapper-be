package handlers

import (
	"testing"
)

// Auth scenario matrix — covers all link-aware hooks (authorizeView, authorizeList,
// linkMapListAuth, authOrLink, AuthorizeMapAccess, authorizeMapSubscription).
//
// Decision flow:
//   1. HasSuperuserAuth? → ALLOW (unconditional, skips all further checks)
//   2. link-id header present? → validate link only (role ignored)
//        valid/non-expired → ALLOW
//        invalid/expired   → DENY
//   3. No link-id → role/auth check
//        passes → ALLOW
//        fails  → DENY
//
// User type    | link-id header  | Auth / Role          | Expected | Notes
// ─────────────┼─────────────────┼──────────────────────┼──────────┼────────────────────────────────────
// Superuser    | absent          | superuser            | allow    | bypasses all checks unconditionally
// Superuser    | valid           | superuser            | allow    | HasSuperuserAuth checked first
// Superuser    | invalid/expired | superuser            | allow    | link-id never reached for superuser
// Conductor /  | absent          | valid role           | allow    | normal authenticated path
// Admin        | valid           | valid role           | allow    | conductor opens publisher link; link wins
//              | invalid/expired | valid role           | deny     | link present → role ignored, link fails
//              | absent          | no role              | deny     | 403 Unauthorized
// Publisher    | valid           | no auth              | allow    | link-only access (no account needed)
//              | invalid/expired | no auth              | deny     | expired or wrong link

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
