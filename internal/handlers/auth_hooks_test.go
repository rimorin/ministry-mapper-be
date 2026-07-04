package handlers

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/search"
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
			name:   "single quotes",
			filter: `map = 'map03'`,
			want:   []string{"map03"},
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
			got := extractIdsFromFilter(mapIdPattern, tc.filter)
			if len(got) != len(tc.want) {
				t.Errorf("extractIdsFromFilter(mapIdPattern, %q) = %v; want %v", tc.filter, got, tc.want)
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

func TestExtractAllCongIdsFromFilter(t *testing.T) {
	tests := []struct {
		name   string
		filter string
		want   []string
	}{
		{
			name:   "single congregation id",
			filter: `congregation = "cong01"`,
			want:   []string{"cong01"},
		},
		{
			name:   "congregation with extra spacing",
			filter: `congregation  =  "cong02"`,
			want:   []string{"cong02"},
		},
		{
			name:   "compound filter with and",
			filter: `congregation = "cong03" && is_default = true`,
			want:   []string{"cong03"},
		},
		{
			name:   "two congregations (injection pattern)",
			filter: `congregation = "cong01" || congregation = "cong02"`,
			want:   []string{"cong01", "cong02"},
		},
		{
			name:   "single quotes",
			filter: `congregation = 'cong04'`,
			want:   []string{"cong04"},
		},
		{
			name:   "no congregation field",
			filter: `map = "map01"`,
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
			got := extractIdsFromFilter(congIdPattern, tc.filter)
			if len(got) != len(tc.want) {
				t.Errorf("extractIdsFromFilter(congIdPattern, %q) = %v; want %v", tc.filter, got, tc.want)
				return
			}
			wantSet := make(map[string]bool, len(tc.want))
			for _, id := range tc.want {
				wantSet[id] = true
			}
			for _, id := range got {
				if !wantSet[id] {
					t.Errorf("unexpected id %q in result for filter %q", id, tc.filter)
				}
			}
		})
	}
}

func TestExtractAllTerritoryIdsFromFilter(t *testing.T) {
	tests := []struct {
		name   string
		filter string
		want   []string
	}{
		{
			name:   "single territory id",
			filter: `territory = "terr01"`,
			want:   []string{"terr01"},
		},
		{
			name:   "compound filter with and",
			filter: `territory = "terr02" && type = "multi"`,
			want:   []string{"terr02"},
		},
		{
			name:   "two territories (injection pattern)",
			filter: `territory = "terr01" || territory = "terr02"`,
			want:   []string{"terr01", "terr02"},
		},
		{
			name:   "single quotes",
			filter: `territory = 'terr03'`,
			want:   []string{"terr03"},
		},
		{
			name:   "no territory field",
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
			got := extractIdsFromFilter(territoryIdPattern, tc.filter)
			if len(got) != len(tc.want) {
				t.Errorf("extractIdsFromFilter(territoryIdPattern, %q) = %v; want %v", tc.filter, got, tc.want)
				return
			}
			wantSet := make(map[string]bool, len(tc.want))
			for _, id := range tc.want {
				wantSet[id] = true
			}
			for _, id := range got {
				if !wantSet[id] {
					t.Errorf("unexpected id %q in result for filter %q", id, tc.filter)
				}
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
			name:   "single quotes",
			filter: `user = 'user03'`,
			want:   "user03",
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

func TestFilterListResults(t *testing.T) {
	collection := core.NewBaseCollection("test")
	newRecord := func(id, mapId string) *core.Record {
		r := core.NewRecord(collection)
		r.Id = id
		r.Set("map", mapId)
		return r
	}

	records := []*core.Record{
		newRecord("rec1", "map01"),
		newRecord("rec2", "map02"),
		newRecord("rec3", "map01"),
	}
	result := &search.Result{Page: 1, PerPage: 30, TotalItems: 3, TotalPages: 1, Items: &records}
	event := &core.RecordsListRequestEvent{Records: records, Result: result}

	filterListResults(event, func(r *core.Record) bool { return r.GetString("map") == "map01" })

	if len(event.Records) != 2 {
		t.Fatalf("event.Records has %d items; want 2", len(event.Records))
	}
	for _, r := range event.Records {
		if r.GetString("map") != "map01" {
			t.Errorf("unexpected record %q survived filtering", r.Id)
		}
	}

	items, ok := event.Result.Items.(*[]*core.Record)
	if !ok {
		t.Fatalf("Result.Items is %T; want *[]*core.Record", event.Result.Items)
	}
	if len(*items) != 2 {
		t.Errorf("Result.Items has %d items; want 2 (the response payload must match the filtered records)", len(*items))
	}
	if event.Result.TotalItems != 2 {
		t.Errorf("Result.TotalItems = %d; want 2", event.Result.TotalItems)
	}
	if event.Result.TotalPages != 1 {
		t.Errorf("Result.TotalPages = %d; want 1", event.Result.TotalPages)
	}
}

func TestFilterListResults_NoneRemoved(t *testing.T) {
	collection := core.NewBaseCollection("test")
	r1 := core.NewRecord(collection)
	r1.Id = "rec1"
	records := []*core.Record{r1}
	result := &search.Result{Page: 1, PerPage: 30, TotalItems: 1, TotalPages: 1, Items: &records}
	event := &core.RecordsListRequestEvent{Records: records, Result: result}

	filterListResults(event, func(*core.Record) bool { return true })

	if len(event.Records) != 1 || event.Result.TotalItems != 1 {
		t.Errorf("filtering with an always-true predicate must not alter results")
	}
}

func TestOrClause(t *testing.T) {
	tests := []struct {
		name   string
		field  string
		values []string
		want   string
	}{
		{
			name:   "no values",
			field:  "map",
			values: nil,
			want:   "",
		},
		{
			name:   "single value",
			field:  "map",
			values: []string{"map01"},
			want:   `map = "map01"`,
		},
		{
			name:   "multiple values are ORed",
			field:  "congregation",
			values: []string{"cong01", "cong02"},
			want:   `congregation = "cong01" || congregation = "cong02"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := orClause(tc.field, tc.values)
			if got != tc.want {
				t.Errorf("orClause(%q, %v) = %q; want %q", tc.field, tc.values, got, tc.want)
			}
		})
	}
}
