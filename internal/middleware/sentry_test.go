package middleware

import (
	"errors"
	"testing"
)

// mockCauser implements the causer interface for testing.
type mockCauser struct {
	cause error
}

func (m *mockCauser) Error() string { return "wrapper: " + m.cause.Error() }
func (m *mockCauser) Cause() error  { return m.cause }

func TestCauserExtractionFromServerError(t *testing.T) {
	realCause := errors.New("sql: connection refused")
	wrapper := &mockCauser{cause: realCause}

	captureErr := error(wrapper)
	if c, ok := captureErr.(causer); ok {
		captureErr = c.Cause()
	}

	if captureErr != realCause {
		t.Errorf("expected real cause %q, got %q", realCause, captureErr)
	}
}

func TestCauserExtractionFromPlainError(t *testing.T) {
	plain := errors.New("plain error")

	captureErr := error(plain)
	if c, ok := captureErr.(causer); ok {
		captureErr = c.Cause()
	}

	if captureErr != plain {
		t.Errorf("plain error should pass through unchanged, got %q", captureErr)
	}
}

func TestCauserNotImplementedByPlainError(t *testing.T) {
	plain := errors.New("no cause here")
	_, ok := plain.(causer)
	if ok {
		t.Error("plain error should not implement causer")
	}
}

func TestCauserImplementedByMockCauser(t *testing.T) {
	mc := &mockCauser{cause: errors.New("real")}
	_, ok := error(mc).(causer)
	if !ok {
		t.Error("mockCauser should implement causer")
	}
}
