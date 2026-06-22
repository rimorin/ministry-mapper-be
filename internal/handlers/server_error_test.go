package handlers

import (
	"errors"
	"fmt"
	"testing"

	"github.com/pocketbase/pocketbase/tools/router"
)

func TestServerErrorError(t *testing.T) {
	cause := errors.New("db: UNIQUE constraint failed")
	se := newServerError(cause)
	if se.Error() != cause.Error() {
		t.Errorf("Error() = %q; want %q", se.Error(), cause.Error())
	}
}

func TestServerErrorCause(t *testing.T) {
	cause := errors.New("connection reset by peer")
	se := newServerError(cause).(*serverError)
	if se.Cause() != cause {
		t.Errorf("Cause() returned wrong error")
	}
}

func TestServerErrorUnwrapsTo500ApiError(t *testing.T) {
	se := newServerError(errors.New("some db error"))
	var apiErr *router.ApiError
	if !errors.As(se, &apiErr) {
		t.Fatal("errors.As did not find *router.ApiError in serverError chain")
	}
	if apiErr.Status != 500 {
		t.Errorf("ApiError.Code = %d; want 500", apiErr.Status)
	}
}

func TestServerErrorMessageNotExposedToClient(t *testing.T) {
	cause := errors.New("internal db error: table 'addresses' is locked")
	se := newServerError(cause)
	var apiErr *router.ApiError
	errors.As(se, &apiErr)
	if apiErr.Message == cause.Error() {
		t.Error("real db error message is exposed in ApiError.Message — should be generic")
	}
}

func TestWrapTransactionErrorPassesThroughApiError(t *testing.T) {
	original := router.NewNotFoundError("Map not found", nil)
	result := wrapTransactionError(original)
	if result != original {
		t.Errorf("wrapTransactionError should return ApiError unchanged, got %T", result)
	}
}

func TestWrapTransactionErrorPassesThroughWrappedApiError(t *testing.T) {
	apiErr := router.NewBadRequestError("code already exists", nil)
	wrapped := fmt.Errorf("tx: %w", apiErr)
	result := wrapTransactionError(wrapped)
	if result != wrapped {
		t.Errorf("wrapTransactionError should return wrapped ApiError unchanged")
	}
}

func TestWrapTransactionErrorWrapsRawError(t *testing.T) {
	raw := errors.New("sql: database is locked")
	result := wrapTransactionError(raw)
	se, ok := result.(*serverError)
	if !ok {
		t.Fatalf("wrapTransactionError should return *serverError for raw error, got %T", result)
	}
	if se.Cause() != raw {
		t.Error("serverError.Cause() should be the original raw error")
	}
}

func TestWrapTransactionErrorRawErrorProduces500(t *testing.T) {
	result := wrapTransactionError(errors.New("disk full"))
	var apiErr *router.ApiError
	if !errors.As(result, &apiErr) {
		t.Fatal("errors.As did not find *router.ApiError after wrapTransactionError")
	}
	if apiErr.Status != 500 {
		t.Errorf("ApiError.Code = %d; want 500", apiErr.Status)
	}
}
