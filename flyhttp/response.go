package flyhttp

import (
	"errors"
	"net/http"

	"github.com/allanfreitas/algosdk/flyerrors"
)

// SuccessMessageResponse define o padrão para respostas com apenas uma mensagem.
type SuccessMessageResponse struct {
	Message string `json:"message"`
}

// Success retorna os dados diretamente para serem serializados no corpo da resposta.
func Success[T any](data T) T {
	return data
}

// SuccessMessage cria uma resposta contendo apenas uma mensagem.
func SuccessMessage(message string) SuccessMessageResponse {
	return SuccessMessageResponse{
		Message: message,
	}
}

// -----problem_details.go------
// ErrorDetail define a estrutura para cada erro específico no array de erros.
type ErrorDetail struct {
	Code  string `json:"code"`
	Value string `json:"value"`
	Field string `json:"field,omitempty"`
}

// ProblemDetails segue o RFC 9457 com a extensão do array "errors".
type ProblemDetails struct {
	Type     string        `json:"type,omitempty"`
	Title    string        `json:"title"`
	Status   int           `json:"status"`
	Detail   string        `json:"detail,omitempty"`
	Instance string        `json:"instance,omitempty"`
	Errors   []ErrorDetail `json:"errors,omitempty"`
}

func Error(code, value string) ErrorDetail {
	return ErrorDetail{
		Code:  code,
		Value: value,
	}
}

func FieldError(field, code, value string) ErrorDetail {
	return ErrorDetail{
		Field: field,
		Code:  code,
		Value: value,
	}
}

// BadRequest cria um erro 400 padronizado apenas com detalhe.
func BadRequest(detail string) ProblemDetails {
	return ProblemDetails{
		Type:   "/errors/bad-request",
		Title:  "Bad Request",
		Status: http.StatusBadRequest,
		Detail: detail,
	}
}

// BadRequestWithErrors cria um erro 400 padronizado com detalhes de erros.
func BadRequestWithErrors(detail string, errors ...ErrorDetail) ProblemDetails {
	return ProblemDetails{
		Type:   "/errors/bad-request",
		Title:  "Bad Request",
		Status: http.StatusBadRequest,
		Detail: detail,
		Errors: errors,
	}
}

// Unauthorized cria um erro 401 padronizado apenas com detalhe.
func Unauthorized(detail string) ProblemDetails {
	return ProblemDetails{
		Type:   "/errors/unauthorized",
		Title:  "Unauthorized",
		Status: http.StatusUnauthorized,
		Detail: detail,
	}
}

// UnauthorizedWithErrors cria um erro 401 padronizado com detalhes de erros.
func UnauthorizedWithErrors(detail string, errors ...ErrorDetail) ProblemDetails {
	return ProblemDetails{
		Type:   "/errors/unauthorized",
		Title:  "Unauthorized",
		Status: http.StatusUnauthorized,
		Detail: detail,
		Errors: errors,
	}
}

// NotFound cria um erro 404 padronizado apenas com detalhe.
func NotFound(detail string) ProblemDetails {
	return ProblemDetails{
		Type:   "/errors/not-found",
		Title:  "Resource Not Found",
		Status: http.StatusNotFound,
		Detail: detail,
	}
}

// NotFoundWithErrors cria um erro 404 padronizado com detalhes de erros.
func NotFoundWithErrors(detail string, errors ...ErrorDetail) ProblemDetails {
	return ProblemDetails{
		Type:   "/errors/not-found",
		Title:  "Resource Not Found",
		Status: http.StatusNotFound,
		Detail: detail,
		Errors: errors,
	}
}

// Validation cria um erro 422 padronizado apenas com detalhe.
func Validation(detail string) ProblemDetails {
	return ProblemDetails{
		Type:   "/errors/validation",
		Title:  "Validation",
		Status: http.StatusUnprocessableEntity,
		Detail: detail,
	}
}

// ValidationWithErrors cria um erro 422 padronizado com detalhes de erros.
func ValidationWithErrors(detail string, errors ...ErrorDetail) ProblemDetails {
	return ProblemDetails{
		Type:   "/errors/validation",
		Title:  "Validation",
		Status: http.StatusUnprocessableEntity,
		Detail: detail,
		Errors: errors,
	}
}

// Internal cria um erro 500 padronizado apenas com detalhe.
func Internal(detail string) ProblemDetails {
	return ProblemDetails{
		Type:   "/errors/internal-server-error",
		Title:  "Internal Server Error",
		Status: http.StatusInternalServerError,
		Detail: detail,
	}
}

// InternalWithErrors cria um erro 500 padronizado com detalhes de erros.
func InternalWithErrors(detail string, errors ...ErrorDetail) ProblemDetails {
	return ProblemDetails{
		Type:   "/errors/internal-server-error",
		Title:  "Internal Server Error",
		Status: http.StatusInternalServerError,
		Detail: detail,
		Errors: errors,
	}
}

// ServiceUnavailable cria um erro 503 padronizado apenas com detalhe.
func ServiceUnavailable(detail string) ProblemDetails {
	return ProblemDetails{
		Type:   "/errors/service-unavailable",
		Title:  "Service Unavailable",
		Status: http.StatusServiceUnavailable,
		Detail: detail,
	}
}

// ServiceUnavailableWithErrors cria um erro 503 padronizado com detalhes de erros.
func ServiceUnavailableWithErrors(detail string, errors ...ErrorDetail) ProblemDetails {
	return ProblemDetails{
		Type:   "/errors/service-unavailable",
		Title:  "Service Unavailable",
		Status: http.StatusServiceUnavailable,
		Detail: detail,
		Errors: errors,
	}
}

// Forbidden cria um erro 403 padronizado apenas com detalhe.
func Forbidden(detail string) ProblemDetails {
	return ProblemDetails{
		Type:   "/errors/forbidden",
		Title:  "Forbidden",
		Status: http.StatusForbidden,
		Detail: detail,
	}
}

// ForbiddenWithErrors cria um erro 403 padronizado com detalhes de erros.
func ForbiddenWithErrors(detail string, errors ...ErrorDetail) ProblemDetails {
	return ProblemDetails{
		Type:   "/errors/forbidden",
		Title:  "Forbidden",
		Status: http.StatusForbidden,
		Detail: detail,
		Errors: errors,
	}
}

// Conflict cria um erro 409 padronizado apenas com detalhe.
func Conflict(detail string) ProblemDetails {
	return ProblemDetails{
		Type:   "/errors/conflict",
		Title:  "Conflict",
		Status: http.StatusConflict,
		Detail: detail,
	}
}

// ConflictWithErrors cria um erro 409 padronizado com detalhes de erros.
func ConflictWithErrors(detail string, errors ...ErrorDetail) ProblemDetails {
	return ProblemDetails{
		Type:   "/errors/conflict",
		Title:  "Conflict",
		Status: http.StatusConflict,
		Detail: detail,
		Errors: errors,
	}
}

// ToHTTP converts a flyerrors.AppError (or an error wrapping one) into an HTTP
// status code and a ProblemDetails body.
// If err is nil or does not contain a flyerrors.AppError, it returns
// 500 Internal Server Error with a generic message.
func ToHTTP(err error) (int, ProblemDetails) {
	appErr, ok := errors.AsType[*flyerrors.AppError](err)
	if !ok {
		return http.StatusInternalServerError, Internal("internal server error")
	}

	details := make([]ErrorDetail, len(appErr.Details()))
	for i, d := range appErr.Details() {
		details[i] = ErrorDetail{
			Field: d.Field,
			Code:  d.Code,
			Value: d.Value,
		}
	}

	switch appErr.Kind() {
	case flyerrors.KindBadRequest:
		return http.StatusBadRequest, BadRequestWithErrors(appErr.Error(), details...)
	case flyerrors.KindUnauthorized:
		return http.StatusUnauthorized, UnauthorizedWithErrors(appErr.Error(), details...)
	case flyerrors.KindForbidden:
		return http.StatusForbidden, ForbiddenWithErrors(appErr.Error(), details...)
	case flyerrors.KindNotFound:
		return http.StatusNotFound, NotFoundWithErrors(appErr.Error(), details...)
	case flyerrors.KindConflict:
		return http.StatusConflict, ConflictWithErrors(appErr.Error(), details...)
	case flyerrors.KindValidation:
		return http.StatusUnprocessableEntity, ValidationWithErrors(appErr.Error(), details...)
	case flyerrors.KindServiceUnavailable:
		return http.StatusServiceUnavailable, ServiceUnavailableWithErrors(appErr.Error(), details...)
	default:
		return http.StatusInternalServerError, Internal(appErr.Error())
	}
}

