package handlers

import (
	"testing"
)

// TestCodeFormatRegex tests the regex pattern directly
func TestCodeFormatRegex(t *testing.T) {
	testCases := []struct {
		code     string
		expected bool
	}{
		// Valid codes
		{"A1", true},
		{"ABC123", true},
		{"123", true},
		{"A-1", true},
		{"Unit-123", true},
		{"A-1-B-2", true},
		{"abc", true},
		{"ABC", true},
		{"AbC123", true},
		{"1-2-3", true},
		
		// Invalid codes
		{"", false},
		{"A 1", false},
		{"A_1", false},
		{"A*1", false},
		{"A.1", false},
		{"A/1", false},
		{"A@1", false},
		{"A#1", false},
		{"A$1", false},
		{"A%1", false},
		{"A&1", false},
		{"A+1", false},
		{"A=1", false},
		{"A!1", false},
		{"A?1", false},
		{"A,1", false},
		{"A;1", false},
		{"A:1", false},
		{"A'1", false},
		{"A\"1", false},
		{"A|1", false},
		{"A\\1", false},
		{"A<1", false},
		{"A>1", false},
		{"A(1", false},
		{"A)1", false},
		{"A[1", false},
		{"A]1", false},
		{"A{1", false},
		{"A}1", false},
	}

	for _, tc := range testCases {
		t.Run(tc.code, func(t *testing.T) {
			result := codeFormatRegex.MatchString(tc.code)
			if result != tc.expected {
				t.Errorf("codeFormatRegex.MatchString(%q) = %v; want %v", tc.code, result, tc.expected)
			}
		})
	}
}

