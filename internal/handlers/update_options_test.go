package handlers

import (
	"strings"
	"testing"
)

// TestIsValidCode tests the code pattern validation
func TestIsValidCode(t *testing.T) {
	testCases := []struct {
		name     string
		code     string
		expected bool
	}{
		{"alphanumeric", "ABC123", true},
		{"with underscore", "HOME_CALL", true},
		{"with hyphen", "HOME-CALL", true},
		{"mixed case", "HomeCall", true},
		{"numbers only", "12345", true},
		{"single char", "H", true},
		{"with spaces", "HOME CALL", false},
		{"with special chars", "HOME@CALL", false},
		{"with dot", "HOME.CALL", false},
		{"with slash", "HOME/CALL", false},
		{"with comma", "HOME,CALL", false},
		{"empty string", "", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isValidCode(tc.code)
			if result != tc.expected {
				t.Errorf("isValidCode(%q) = %v; want %v", tc.code, result, tc.expected)
			}
		})
	}
}

// TestValidateOptionFormat tests format validation for individual options
func TestValidateOptionFormat(t *testing.T) {
	testCases := []struct {
		name      string
		optionMap map[string]interface{}
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid option",
			optionMap: map[string]interface{}{
				"code":         "HC",
				"description":  "Home Call",
				"sequence":     1.0,
				"is_default":   true,
				"is_countable": true,
			},
			wantError: false,
		},
		{
			name: "valid with underscore",
			optionMap: map[string]interface{}{
				"code":        "HOME_CALL",
				"description": "Home Call",
				"sequence":    1.0,
				"is_default":  false,
			},
			wantError: false,
		},
		{
			name: "empty code",
			optionMap: map[string]interface{}{
				"code":       "",
				"sequence":   1.0,
				"is_default": true,
			},
			wantError: true,
			errorMsg:  "code cannot be empty",
		},
		{
			name: "whitespace only code",
			optionMap: map[string]interface{}{
				"code":       "   ",
				"sequence":   1.0,
				"is_default": true,
			},
			wantError: true,
			errorMsg:  "code cannot be empty",
		},
		{
			name: "code too long",
			optionMap: map[string]interface{}{
				"code":       strings.Repeat("A", 51),
				"sequence":   1.0,
				"is_default": true,
			},
			wantError: true,
			errorMsg:  "code cannot exceed 50 characters",
		},
		{
			name: "code with spaces",
			optionMap: map[string]interface{}{
				"code":       "HOME CALL",
				"sequence":   1.0,
				"is_default": true,
			},
			wantError: true,
			errorMsg:  "code can only contain letters, numbers, underscores, and hyphens",
		},
		{
			name: "code with special chars",
			optionMap: map[string]interface{}{
				"code":       "HOME@CALL",
				"sequence":   1.0,
				"is_default": true,
			},
			wantError: true,
			errorMsg:  "code can only contain letters, numbers, underscores, and hyphens",
		},
		{
			name: "description too long",
			optionMap: map[string]interface{}{
				"code":        "HC",
				"description": strings.Repeat("A", 201),
				"sequence":    1.0,
				"is_default":  true,
			},
			wantError: true,
			errorMsg:  "description cannot exceed 200 characters",
		},
		{
			name: "negative sequence",
			optionMap: map[string]interface{}{
				"code":       "HC",
				"sequence":   -1.0,
				"is_default": true,
			},
			wantError: true,
			errorMsg:  "sequence cannot be negative",
		},
		{
			name: "invalid sequence type",
			optionMap: map[string]interface{}{
				"code":       "HC",
				"sequence":   "not a number",
				"is_default": true,
			},
			wantError: true,
			errorMsg:  "invalid sequence format",
		},
		{
			name: "invalid is_default type",
			optionMap: map[string]interface{}{
				"code":       "HC",
				"sequence":   1.0,
				"is_default": "true",
			},
			wantError: true,
			errorMsg:  "invalid is_default format",
		},
		{
			name: "deleted option skips validation",
			optionMap: map[string]interface{}{
				"is_deleted": true,
				"code":       "",
			},
			wantError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateOptionFormat(tc.optionMap)
			if tc.wantError {
				if err == nil {
					t.Errorf("validateOptionFormat() expected error containing %q, got nil", tc.errorMsg)
				} else if !strings.Contains(err.Error(), tc.errorMsg) {
					t.Errorf("validateOptionFormat() error = %v, want error containing %q", err, tc.errorMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validateOptionFormat() unexpected error: %v", err)
				}
			}
		})
	}
}

// TestValidateOptionsPayload tests payload-level validation
func TestValidateOptionsPayload(t *testing.T) {
	testCases := []struct {
		name      string
		options   []interface{}
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid single option",
			options: []interface{}{
				map[string]interface{}{
					"code":       "HC",
					"sequence":   1.0,
					"is_default": true,
				},
			},
			wantError: false,
		},
		{
			name: "valid multiple options",
			options: []interface{}{
				map[string]interface{}{
					"code":       "HC",
					"sequence":   1.0,
					"is_default": true,
				},
				map[string]interface{}{
					"code":       "BC",
					"sequence":   2.0,
					"is_default": false,
				},
			},
			wantError: false,
		},
		{
			name: "duplicate code in payload",
			options: []interface{}{
				map[string]interface{}{
					"code":       "HC",
					"sequence":   1.0,
					"is_default": true,
				},
				map[string]interface{}{
					"code":       "HC",
					"sequence":   2.0,
					"is_default": false,
				},
			},
			wantError: true,
			errorMsg:  "duplicate code in payload",
		},
		{
			name: "duplicate sequence in payload",
			options: []interface{}{
				map[string]interface{}{
					"code":       "HC",
					"sequence":   1.0,
					"is_default": true,
				},
				map[string]interface{}{
					"code":       "BC",
					"sequence":   1.0,
					"is_default": false,
				},
			},
			wantError: true,
			errorMsg:  "duplicate sequence in payload",
		},
		{
			name: "no default option",
			options: []interface{}{
				map[string]interface{}{
					"code":       "HC",
					"sequence":   1.0,
					"is_default": false,
				},
			},
			wantError: true,
			errorMsg:  "exactly one option must be marked as default",
		},
		{
			name: "multiple defaults",
			options: []interface{}{
				map[string]interface{}{
					"code":       "HC",
					"sequence":   1.0,
					"is_default": true,
				},
				map[string]interface{}{
					"code":       "BC",
					"sequence":   2.0,
					"is_default": true,
				},
			},
			wantError: true,
			errorMsg:  "exactly one option must be marked as default",
		},
		{
			name: "deleted options ignored in default count",
			options: []interface{}{
				map[string]interface{}{
					"code":       "HC",
					"sequence":   1.0,
					"is_default": true,
					"is_deleted": true,
				},
				map[string]interface{}{
					"code":       "BC",
					"sequence":   2.0,
					"is_default": true,
				},
			},
			wantError: false,
		},
		{
			name: "invalid option format",
			options: []interface{}{
				"not a map",
			},
			wantError: true,
			errorMsg:  "invalid option format at index",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateOptionsPayload(tc.options)
			if tc.wantError {
				if err == nil {
					t.Errorf("validateOptionsPayload() expected error containing %q, got nil", tc.errorMsg)
				} else if !strings.Contains(err.Error(), tc.errorMsg) {
					t.Errorf("validateOptionsPayload() error = %v, want error containing %q", err, tc.errorMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validateOptionsPayload() unexpected error: %v", err)
				}
			}
		})
	}
}
