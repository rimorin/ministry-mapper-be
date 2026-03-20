package handlers

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGenerateReportRequest_JsonParsing(t *testing.T) {
	testCases := []struct {
		name             string
		body             string
		wantCongregation string
		wantEmpty        bool
	}{
		{
			name:             "valid congregation id",
			body:             `{"congregation": "abc123def456"}`,
			wantCongregation: "abc123def456",
		},
		{
			name:      "missing congregation field",
			body:      `{}`,
			wantEmpty: true,
		},
		{
			name:      "empty congregation string",
			body:      `{"congregation": ""}`,
			wantEmpty: true,
		},
		{
			name:             "extra fields are ignored",
			body:             `{"congregation": "abc123", "unknown": "value"}`,
			wantCongregation: "abc123",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var req GenerateReportRequest
			if err := json.NewDecoder(strings.NewReader(tc.body)).Decode(&req); err != nil {
				t.Fatalf("unexpected decode error: %v", err)
			}

			if tc.wantEmpty && req.Congregation != "" {
				t.Errorf("expected empty congregation, got %q", req.Congregation)
			}

			if !tc.wantEmpty && req.Congregation != tc.wantCongregation {
				t.Errorf("congregation = %q; want %q", req.Congregation, tc.wantCongregation)
			}
		})
	}
}

// TestHandleGenerateReport_Scenarios documents expected API behavior.
// Full integration tests require a running PocketBase test app with seeded data.
//
// Expected behavior:
//   - POST /report/generate with valid congregation ID + admin role → 202 Accepted
//   - POST /report/generate with valid congregation ID + non-admin role → 403 Forbidden
//   - POST /report/generate with missing congregation field → 400 Bad Request
//   - POST /report/generate with nonexistent congregation ID (but admin role) → 404 Not Found
func TestHandleGenerateReport_Scenarios(t *testing.T) {
	scenarios := []struct {
		name           string
		congregation   string
		userRole       string
		expectedStatus int
		expectedMsg    string
	}{
		{
			name:           "administrator triggers report",
			congregation:   "<valid congregation id>",
			userRole:       "administrator",
			expectedStatus: 202,
			expectedMsg:    "Report generation started",
		},
		{
			name:           "non-admin user is rejected",
			congregation:   "<valid congregation id>",
			userRole:       "conductor",
			expectedStatus: 403,
			expectedMsg:    "Not an administrator for this congregation",
		},
		{
			name:           "missing congregation field",
			congregation:   "",
			userRole:       "administrator",
			expectedStatus: 400,
			expectedMsg:    "congregation is required",
		},
		{
			name:           "nonexistent congregation id",
			congregation:   "doesnotexist000",
			userRole:       "administrator",
			expectedStatus: 404,
			expectedMsg:    "Congregation not found",
		},
	}

	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			t.Logf("congregation=%q role=%q → HTTP %d (%s)",
				s.congregation, s.userRole, s.expectedStatus, s.expectedMsg)
		})
	}
}
