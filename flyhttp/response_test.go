package flyhttp

import (
	"errors"
	"net/http"
	"testing"

	"github.com/allanfreitas/algosdk/flyerrors"
)

func TestToHTTP(t *testing.T) {
	// 1. Nil error or non-AppError
	status, prob := ToHTTP(nil)
	if status != http.StatusInternalServerError {
		t.Errorf("expected 500 status, got %d", status)
	}
	if prob.Title != "Internal Server Error" {
		t.Errorf("expected 'Internal Server Error' title, got %q", prob.Title)
	}

	status, prob = ToHTTP(errors.New("some raw error"))
	if status != http.StatusInternalServerError {
		t.Errorf("expected 500 status, got %d", status)
	}

	// 2. Validation AppError
	valErr := flyerrors.Validation(
		"invalid fields",
		flyerrors.Field("email", "invalid", "email is invalid"),
		flyerrors.Field("password", "short", "password too short"),
	)
	status, prob = ToHTTP(valErr)
	if status != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", status)
	}
	if prob.Title != "Validation" {
		t.Errorf("expected 'Validation' title, got %q", prob.Title)
	}
	if len(prob.Errors) != 2 {
		t.Errorf("expected 2 details, got %d", len(prob.Errors))
	}
	if prob.Errors[0].Field != "email" || prob.Errors[0].Code != "invalid" || prob.Errors[0].Value != "email is invalid" {
		t.Errorf("unexpected email error: %+v", prob.Errors[0])
	}
	if prob.Errors[1].Field != "password" || prob.Errors[1].Code != "short" || prob.Errors[1].Value != "password too short" {
		t.Errorf("unexpected password error: %+v", prob.Errors[1])
	}

	// 3. NotFound AppError
	nfErr := flyerrors.NotFound("item not found")
	status, prob = ToHTTP(nfErr)
	if status != http.StatusNotFound {
		t.Errorf("expected 404, got %d", status)
	}
	if prob.Title != "Resource Not Found" {
		t.Errorf("expected 'Resource Not Found' title, got %q", prob.Title)
	}
	if prob.Detail != "item not found" {
		t.Errorf("expected detail 'item not found', got %q", prob.Detail)
	}

	// 4. Wrapped AppError
	wrappedErr := flyerrors.Wrap(nfErr, "wrapper message")
	status, prob = ToHTTP(wrappedErr)
	if status != http.StatusNotFound {
		t.Errorf("expected wrapped 404, got %d", status)
	}
	if prob.Detail != "wrapper message" {
		t.Errorf("expected wrapper detail message, got %q", prob.Detail)
	}
}

func TestSuccess(t *testing.T) {
	data := "test-payload"
	res := Success(data)
	if res != data {
		t.Errorf("expected %v, got %v", data, res)
	}
}

func TestSuccessMessage(t *testing.T) {
	msg := "Operation completed successfully"
	res := SuccessMessage(msg)
	if res.Message != msg {
		t.Errorf("expected message %q, got %q", msg, res.Message)
	}
}
