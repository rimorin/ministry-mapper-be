package handlers

import (
	"strings"
	"testing"
)

func TestValidateSequencePayload(t *testing.T) {
	cases := []struct {
		name    string
		codes   []CodeSequenceUpdate
		wantErr string
	}{
		{
			name: "a complete reorder is accepted",
			codes: []CodeSequenceUpdate{
				{Code: "4299", Sequence: 0},
				{Code: "4301", Sequence: 1},
				{Code: "4303", Sequence: 2},
			},
		},
		{
			// The defect this guard exists for: 4301 and 4303 both on sequence 1
			// put cells under the wrong column header and drop one column from
			// the Excel export.
			name: "two codes claiming one sequence is rejected",
			codes: []CodeSequenceUpdate{
				{Code: "4299", Sequence: 0},
				{Code: "4301", Sequence: 1},
				{Code: "4303", Sequence: 1},
			},
			wantErr: "both claim sequence 1",
		},
		{
			name: "a repeated code is rejected",
			codes: []CodeSequenceUpdate{
				{Code: "4301", Sequence: 0},
				{Code: "4301", Sequence: 1},
			},
			wantErr: "Duplicate code",
		},
		{
			name:    "an empty code is rejected",
			codes:   []CodeSequenceUpdate{{Code: "", Sequence: 0}},
			wantErr: "cannot be empty",
		},
		{
			name: "gaps are allowed, only collisions are not",
			codes: []CodeSequenceUpdate{
				{Code: "4299", Sequence: 0},
				{Code: "4301", Sequence: 7},
				{Code: "4303", Sequence: 92},
			},
		},
		{
			name: "letter suffixed codes are distinct",
			codes: []CodeSequenceUpdate{
				{Code: "10", Sequence: 0},
				{Code: "10A", Sequence: 1},
				{Code: "10B", Sequence: 2},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			seen, err := validateSequencePayload(c.codes)

			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(seen) != len(c.codes) {
					t.Errorf("returned %d codes, want %d", len(seen), len(c.codes))
				}
				return
			}

			if err == nil {
				t.Fatalf("expected an error containing %q, got nil", c.wantErr)
			}
			if !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), c.wantErr)
			}
		})
	}
}

func TestValidateSequencePayloadEmptyRequest(t *testing.T) {
	// HandleMapUpdateSequence rejects an empty codes array before reaching the
	// payload check, so here an empty slice is simply vacuously valid.
	seen, err := validateSequencePayload(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(seen) != 0 {
		t.Errorf("returned %d codes, want 0", len(seen))
	}
}
