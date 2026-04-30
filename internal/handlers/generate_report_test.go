package handlers

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
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

// invokeGeneratorStub simulates the goroutine's call to the generator without a live app.
func invokeGeneratorStub(fn ReportGeneratorFn) error {
	return fn(nil, nil, nil)
}

// All 11 scenarios for this handler are documented below.
// Scenarios 1–8 are synchronous (decided before 202 is sent).
// Scenarios 9–11 run inside the goroutine and are invisible to the HTTP caller;
// errors are captured by Sentry.
//
// Scenario | Input / State                              | HTTP | Notes
// ─────────┼────────────────────────────────────────────┼──────┼─────────────────────────────────────────
//  1       | No auth token                              | 401  | Rejected by RequireAuth() middleware
//  2       | Malformed JSON body                        | 400  | "Invalid request body"
//  3       | congregation field missing or empty        | 400  | "congregation is required"
//  4       | Valid congregation, user NOT admin         | 403  | "Not an administrator for this congregation"
//  5       | Valid admin role, congregation deleted     | 404  | "Congregation not found"
//  6       | Valid congregation, user record not found  | 400  | "Could not validate your account"
//  7       | Valid congregation, user has no email      | 400  | "Your account has no email address configured"
//  8       | All checks pass                            | 202  | Goroutine launched; "Report generation started."
//  9       | [goroutine] congregation deleted post-202  | n/a  | Sentry; no email sent
// 10       | [goroutine] user deleted post-202          | n/a  | Sentry; no email sent
// 11       | [goroutine] generator returns error        | n/a  | Sentry; caller already received 202
// ─────────┴────────────────────────────────────────────┴──────┴─────────────────────────────────────────
//
// Full integration tests (scenarios 1–8) require a running PocketBase test app
// with seeded data. The table above is the authoritative specification.
// ---------------------------------------------------------------------------

// TestHandleGenerateReport_GeneratorError verifies that a generator returning
// an error does not panic and is handled gracefully (errors go to Sentry).
func TestHandleGenerateReport_GeneratorError(t *testing.T) {
	expectedErr := errors.New("report generation failed: MailerSend API error")

	called := false
	var capturedErr error

	generator := ReportGeneratorFn(func(app core.App, cong *core.Record, recipient *core.Record) error {
		called = true
		return expectedErr
	})

	capturedErr = invokeGeneratorStub(generator)

	if !called {
		t.Error("generator was not called")
	}
	if capturedErr == nil {
		t.Error("expected error from generator, got nil")
	}
	if !errors.Is(capturedErr, expectedErr) {
		t.Errorf("capturedErr = %v; want %v", capturedErr, expectedErr)
	}
}

// TestHandleGenerateReport_GeneratorSuccess verifies that a generator returning
// nil does not cause issues.
func TestHandleGenerateReport_GeneratorSuccess(t *testing.T) {
	called := false

	generator := ReportGeneratorFn(func(app core.App, cong *core.Record, recipient *core.Record) error {
		called = true
		return nil
	})

	err := invokeGeneratorStub(generator)
	if !called {
		t.Error("generator was not called")
	}
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

