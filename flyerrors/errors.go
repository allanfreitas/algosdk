package flyerrors

import "errors"

// ErrorDetail carries per-field or per-item error metadata.
// JSON shape is intentionally identical to flyhttp.ErrorDetail
// so handlers can forward it without any conversion.
type ErrorDetail struct {
	Field string `json:"field,omitempty"`
	Code  string `json:"code"`
	Value string `json:"value"`
}

// Kind classifies an AppError by its intended HTTP response category.
type Kind uint8

const (
	KindBadRequest         Kind = iota // 400
	KindUnauthorized                   // 401
	KindForbidden                      // 403
	KindNotFound                       // 404
	KindConflict                       // 409
	KindValidation                     // 422
	KindInternal                       // 500
	KindServiceUnavailable             // 503
)

// AppError is a structured error that carries an HTTP-category Kind,
// a human-readable message, optional field-level details, and an
// optional wrapped cause.
type AppError struct {
	kind    Kind
	message string
	details []ErrorDetail
	cause   error
}

func (e *AppError) Error() string         { return e.message }
func (e *AppError) Unwrap() error         { return e.cause }
func (e *AppError) Kind() Kind            { return e.kind }
func (e *AppError) Details() []ErrorDetail { return e.details }

// Validation returns a KindValidation AppError with optional field details.
func Validation(msg string, details ...ErrorDetail) *AppError {
	return &AppError{
		kind:    KindValidation,
		message: msg,
		details: details,
	}
}

// Field is a convenience constructor for a single ErrorDetail.
func Field(field, code, value string) ErrorDetail {
	return ErrorDetail{
		Field: field,
		Code:  code,
		Value: value,
	}
}

// NotFound returns a KindNotFound AppError.
func NotFound(msg string) *AppError {
	return &AppError{
		kind:    KindNotFound,
		message: msg,
	}
}

// Conflict returns a KindConflict AppError with optional details.
func Conflict(msg string, details ...ErrorDetail) *AppError {
	return &AppError{
		kind:    KindConflict,
		message: msg,
		details: details,
	}
}

// Forbidden returns a KindForbidden AppError.
func Forbidden(msg string) *AppError {
	return &AppError{
		kind:    KindForbidden,
		message: msg,
	}
}

// Unauthorized returns a KindUnauthorized AppError.
func Unauthorized(msg string) *AppError {
	return &AppError{
		kind:    KindUnauthorized,
		message: msg,
	}
}

// BadRequest returns a KindBadRequest AppError with optional details.
func BadRequest(msg string, details ...ErrorDetail) *AppError {
	return &AppError{
		kind:    KindBadRequest,
		message: msg,
		details: details,
	}
}

// Internal wraps a low-level cause as a KindInternal AppError.
func Internal(msg string, cause error) *AppError {
	return &AppError{
		kind:    KindInternal,
		message: msg,
		cause:   cause,
	}
}

// Wrap wraps any existing error, preserving its AsType/Is behavior.
// If the wrapped error is already an AppError, we preserve its Kind and Details.
func Wrap(err error, msg string) *AppError {
	if err == nil {
		return nil
	}
	kind := KindInternal
	var details []ErrorDetail
	if appErr, ok := errors.AsType[*AppError](err); ok {
		kind = appErr.kind
		details = appErr.details
	}
	return &AppError{
		kind:    kind,
		message: msg,
		cause:   err,
		details: details,
	}
}
