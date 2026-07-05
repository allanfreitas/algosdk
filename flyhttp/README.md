# flyhttp

`flyhttp` is a Go package that provides standardized HTTP response structures and constructor functions for success responses and error responses (following RFC 9457 / Problem Details).

## Import Path

```go
import "github.com/allanfreitas/algosdk/flyhttp"
```

## Features

- **Standardized Success Responses**: Generic struct with a message and payload data.
- **Problem Details for HTTP APIs (RFC 9457)**: A standardized JSON format for error responses.
- **Validation Errors Array**: Extends standard problem details with field-specific errors.
- **Framework Agnostic**: Returns raw structs, letting you serialize using Echo, Gin, Chi, or the standard `net/http` library.

---

## 1. Success Responses

### `SuccessResponse[T any]`

Defines the structure for successful HTTP responses.

```go
type SuccessResponse[T any] struct {
    Message string `json:"message,omitempty"`
    Data    T      `json:"data,omitempty"`
}
```

### `Success` Constructor

Creates a new `SuccessResponse` with the provided message and data payload.

```go
func Success[T any](message string, data T) SuccessResponse[T]
```

#### Example Usage

```go
type User struct {
    ID   string `json:"id"`
    Name string `json:"name"`
}

// In your HTTP handler:
user := User{ID: "123", Name: "Allan Freitas"}
response := flyhttp.Success("User created successfully", user)

// JSON Output:
// {
//   "message": "User created successfully",
//   "data": {
//     "id": "123",
//     "name": "Allan Freitas"
//   }
// }
```

---

## 2. Error Responses (RFC 9457 / Problem Details)

### `ProblemDetails` Struct

Follows the RFC 9457 specification with a custom `errors` list extension for field-level error details.

```go
type ProblemDetails struct {
    Type     string        `json:"type,omitempty"`
    Title    string        `json:"title"`
    Status   int           `json:"status"`
    Detail   string        `json:"detail,omitempty"`
    Instance string        `json:"instance,omitempty"`
    Errors   []ErrorDetail `json:"errors,omitempty"`
}
```

### `ErrorDetail` Struct

Defines a specific error occurrence (e.g., validation failure on a particular field).

```go
type ErrorDetail struct {
    Code  string `json:"code"`
    Value string `json:"value"`
    Field string `json:"field,omitempty"`
}
```

#### Helper Functions for `ErrorDetail`

*   **`Error(code, value string) ErrorDetail`**:
    Creates a general error without specifying a field.
*   **`FieldError(field, code, value string) ErrorDetail`**:
    Creates a field-specific validation error.

---

## Error Constructors

`flyhttp` provides standard constructors for common HTTP status codes, available in both simple versions (only detail string) and detailed versions (with sub-errors list):

### `BadRequest` & `BadRequestWithErrors`
```go
func BadRequest(detail string) ProblemDetails
func BadRequestWithErrors(detail string, errors ...ErrorDetail) ProblemDetails
```

### `Unauthorized` & `UnauthorizedWithErrors`
```go
func Unauthorized(detail string) ProblemDetails
func UnauthorizedWithErrors(detail string, errors ...ErrorDetail) ProblemDetails
```

### `NotFound` & `NotFoundWithErrors`
```go
func NotFound(detail string) ProblemDetails
func NotFoundWithErrors(detail string, errors ...ErrorDetail) ProblemDetails
```

### `Validation` & `ValidationWithErrors`
```go
func Validation(detail string) ProblemDetails
func ValidationWithErrors(detail string, errors ...ErrorDetail) ProblemDetails
```

### `Internal` & `InternalWithErrors`
```go
func Internal(detail string) ProblemDetails
func InternalWithErrors(detail string, errors ...ErrorDetail) ProblemDetails
```

### `ServiceUnavailable` & `ServiceUnavailableWithErrors`
```go
func ServiceUnavailable(detail string) ProblemDetails
func ServiceUnavailableWithErrors(detail string, errors ...ErrorDetail) ProblemDetails
```

### `Forbidden` & `ForbiddenWithErrors`
```go
func Forbidden(detail string) ProblemDetails
func ForbiddenWithErrors(detail string, errors ...ErrorDetail) ProblemDetails
```

### `Conflict` & `ConflictWithErrors`
```go
func Conflict(detail string) ProblemDetails
func ConflictWithErrors(detail string, errors ...ErrorDetail) ProblemDetails
```

---

## Code Examples

### A. Using with Echo (v5)

```go
package handlers

import (
    "net/http"
    "strings"
    "github.com/labstack/echo/v5"
    "github.com/allanfreitas/algosdk/flyhttp"
)

type RegisterReq struct {
    Name     string `json:"name"`
    Email    string `json:"email"`
    Password string `json:"password"`
}

func RegisterHandler(c *echo.Context) error {
    var req RegisterReq
    if err := c.Bind(&req); err != nil {
        return c.JSON(http.StatusBadRequest, flyhttp.BadRequest("Invalid request body"))
    }

    var valErrors []flyhttp.ErrorDetail

    // Validate fields and collect multiple validation errors
    if strings.TrimSpace(req.Name) == "" {
        valErrors = append(valErrors, flyhttp.FieldError("name", "required", "Name is a required field"))
    }
    if strings.TrimSpace(req.Email) == "" {
        valErrors = append(valErrors, flyhttp.FieldError("email", "required", "Email is a required field"))
    } else if !strings.Contains(req.Email, "@") {
        valErrors = append(valErrors, flyhttp.FieldError("email", "invalid", "Email must be a valid email address"))
    }
    if len(req.Password) < 8 {
        valErrors = append(valErrors, flyhttp.FieldError("password", "min_length", "Password must be at least 8 characters long"))
    }

    // If there are validation errors, return a 422 Unprocessable Entity
    if len(valErrors) > 0 {
        return c.JSON(http.StatusUnprocessableEntity, flyhttp.ValidationWithErrors("Validation failed", valErrors...))
    }

    // Success flow
    resp := flyhttp.Success("Registration initiated", map[string]string{"email": req.Email})
    return c.JSON(http.StatusOK, resp)
}
```

#### JSON Error Response Example
If the request is sent with an empty `name` and a `password` shorter than 8 characters, the output JSON format will be:

```json
{
  "type": "/errors/validation",
  "title": "Validation",
  "status": 422,
  "detail": "Validation failed",
  "errors": [
    {
      "code": "required",
      "value": "Name is a required field",
      "field": "name"
    },
    {
      "code": "min_length",
      "value": "Password must be at least 8 characters long",
      "field": "password"
    }
  ]
}
```

### B. Using with Standard `net/http`

```go
package main

import (
    "encoding/json"
    "net/http"
    "github.com/allanfreitas/algosdk/flyhttp"
)

func myHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        errResp := flyhttp.BadRequest("Only POST method is allowed")
        w.Header().Set("Content-Type", "application/problem+json")
        w.WriteHeader(errResp.Status)
        json.NewEncoder(w).Encode(errResp)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(flyhttp.Success("Success", "payload"))
}
```
