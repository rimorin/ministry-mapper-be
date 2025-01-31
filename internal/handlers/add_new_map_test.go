package handlers

import (
	"testing"
)

func TestIsValidSequence(t *testing.T) {
	tests := []struct {
		sequence string
		expected bool
	}{
		{"A1,B2,C3", true},
		{"A1, B2, C3", false},
		{"A-1, A-2, A-3", false},
		{"A-1,A-2,A-3", true},
		{"A1,B2,C3,", false},
		{"A1,B2,C3,D4", true},
		{"", false},
		{"A!,B2,C3", false},
		{"1,2,3", true},
		{"1,2,3,4A", true},
		{"1,2,3,4,5,!", false},
		{"1,2,3,4,5,6B", true},
		{",,A1", false},
		{"A1,,", false},
	}

	for _, test := range tests {
		result := isValidSequence(test.sequence)
		if result != test.expected {
			t.Errorf("isValidSequence(%q) = %v; expected %v", test.sequence, result, test.expected)
		}
	}
}
