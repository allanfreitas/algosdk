package flyerrors

import (
	"errors"
	"testing"
)

func TestAppErrorConstructors(t *testing.T) {
	// Validation
	valErr := Validation("validation failed", Field("name", "required", "name is required"))
	if valErr.Kind() != KindValidation {
		t.Errorf("expected KindValidation, got %v", valErr.Kind())
	}
	if valErr.Error() != "validation failed" {
		t.Errorf("expected message 'validation failed', got %q", valErr.Error())
	}
	if len(valErr.Details()) != 1 {
		t.Errorf("expected 1 detail, got %d", len(valErr.Details()))
	}
	detail := valErr.Details()[0]
	if detail.Field != "name" || detail.Code != "required" || detail.Value != "name is required" {
		t.Errorf("unexpected detail: %+v", detail)
	}

	// NotFound
	nfErr := NotFound("user not found")
	if nfErr.Kind() != KindNotFound {
		t.Errorf("expected KindNotFound, got %v", nfErr.Kind())
	}

	// Conflict
	confErr := Conflict("already exists")
	if confErr.Kind() != KindConflict {
		t.Errorf("expected KindConflict, got %v", confErr.Kind())
	}

	// Forbidden
	forbErr := Forbidden("access denied")
	if forbErr.Kind() != KindForbidden {
		t.Errorf("expected KindForbidden, got %v", forbErr.Kind())
	}

	// Unauthorized
	unauthErr := Unauthorized("unauthorized")
	if unauthErr.Kind() != KindUnauthorized {
		t.Errorf("expected KindUnauthorized, got %v", unauthErr.Kind())
	}

	// BadRequest
	brErr := BadRequest("bad request")
	if brErr.Kind() != KindBadRequest {
		t.Errorf("expected KindBadRequest, got %v", brErr.Kind())
	}

	// Internal
	cause := errors.New("database connection lost")
	intErr := Internal("internal error occurred", cause)
	if intErr.Kind() != KindInternal {
		t.Errorf("expected KindInternal, got %v", intErr.Kind())
	}
	if intErr.Unwrap() != cause {
		t.Errorf("expected cause to be returned by Unwrap")
	}
}

func TestAsType(t *testing.T) {
	cause := errors.New("db error")
	appErr := Internal("something failed", cause)

	// Wrap
	wrapped := Wrap(appErr, "failed to process request")

	// Verify errors.Is with original cause
	if !errors.Is(wrapped, cause) {
		t.Errorf("expected wrapped error to be Is(cause)")
	}

	// Verify errors.AsType extraction (Go 1.26+)
	extracted, ok := errors.AsType[*AppError](wrapped)
	if !ok {
		t.Fatalf("expected errors.AsType to extract AppError")
	}
	if extracted.Kind() != KindInternal {
		t.Errorf("expected extracted kind to be KindInternal, got %v", extracted.Kind())
	}
	if extracted.Error() != "failed to process request" {
		t.Errorf("expected extracted message 'failed to process request', got %q", extracted.Error())
	}
}
