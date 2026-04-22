package resp

import (
	"context"
	"net/http"
	"testing"

	"github.com/kanata996/hah/internal/errx"
)

func TestSuccessResponseExportsDefaultEnvelope(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		response, err := SuccessResponse(http.StatusOK, map[string]any{"id": "u_1"})
		if err != nil {
			t.Fatalf("SuccessResponse() error = %v", err)
		}
		if response.Status != http.StatusOK {
			t.Fatalf("Status = %d, want %d", response.Status, http.StatusOK)
		}
		if response.Code != successTopCode {
			t.Fatalf("Code = %d, want %d", response.Code, successTopCode)
		}
		if response.Message != successMessage {
			t.Fatalf("Message = %q, want %q", response.Message, successMessage)
		}
		if response.Error != nil {
			t.Fatalf("Error = %#v, want nil", response.Error)
		}

		data, ok := response.Data.(map[string]any)
		if !ok {
			t.Fatalf("Data = %#v, want object", response.Data)
		}
		if got := data["id"]; got != "u_1" {
			t.Fatalf("Data[id] = %#v, want u_1", got)
		}
	})

	t.Run("typed nil payload is preserved", func(t *testing.T) {
		type user struct{ ID string }
		var nilUser *user

		response, err := SuccessResponse(http.StatusCreated, nilUser)
		if err != nil {
			t.Fatalf("SuccessResponse() error = %v", err)
		}
		if response.Status != http.StatusCreated {
			t.Fatalf("Status = %d, want %d", response.Status, http.StatusCreated)
		}
		if response.Data != nilUser {
			t.Fatalf("Data = %#v, want %#v", response.Data, nilUser)
		}
	})
}

func TestSuccessResponseRejectsUnsupportedStatus(t *testing.T) {
	response, err := SuccessResponse(http.StatusNoContent, map[string]any{"id": "u_1"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if response != nil {
		t.Fatalf("Response = %#v, want nil", response)
	}
}

func TestErrorResponseExportsDefaultEnvelope(t *testing.T) {
	httpErr := errx.NewHTTPError(http.StatusUnprocessableEntity, "", "").WithFieldErrors([]errx.FieldError{
		{Field: "name", In: errx.InBody, Code: errx.CodeRequired, Detail: "is required"},
	})

	response, err := ErrorResponse(httpErr, 42201)
	if err != nil {
		t.Fatalf("ErrorResponse() error = %v", err)
	}
	if response.Status != http.StatusUnprocessableEntity {
		t.Fatalf("Status = %d, want %d", response.Status, http.StatusUnprocessableEntity)
	}
	if response.Code != 42201 {
		t.Fatalf("Code = %d, want 42201", response.Code)
	}
	if response.Message != "unprocessable entity" {
		t.Fatalf("Message = %q, want unprocessable entity", response.Message)
	}
	if response.Data != nil {
		t.Fatalf("Data = %#v, want nil", response.Data)
	}
	if response.Error == nil {
		t.Fatal("Error = nil, want value")
	}
	if response.Error.Reason != "unprocessable_entity" {
		t.Fatalf("Error.Reason = %q, want unprocessable_entity", response.Error.Reason)
	}
	if len(response.Error.Details) != 1 {
		t.Fatalf("len(Error.Details) = %d, want 1", len(response.Error.Details))
	}
	if got := response.Error.Details[0]; got.Field != "name" || got.In != errx.InBody || got.Code != errx.CodeRequired || got.Detail != "is required" {
		t.Fatalf("Error.Details[0] = %#v", got)
	}
}

func TestErrorResponseDefaultsAndNilError(t *testing.T) {
	t.Run("context deadline exceeded uses normalized default response", func(t *testing.T) {
		response, err := ErrorResponse(context.DeadlineExceeded)
		if err != nil {
			t.Fatalf("ErrorResponse() error = %v", err)
		}
		if response.Status != http.StatusGatewayTimeout {
			t.Fatalf("Status = %d, want %d", response.Status, http.StatusGatewayTimeout)
		}
		if response.Code != 50400 {
			t.Fatalf("Code = %d, want 50400", response.Code)
		}
		if response.Message != "timeout" {
			t.Fatalf("Message = %q, want timeout", response.Message)
		}
		if response.Error == nil || response.Error.Reason != "timeout" {
			t.Fatalf("Error = %#v, want timeout reason", response.Error)
		}
	})

	t.Run("nil error is noop", func(t *testing.T) {
		response, err := ErrorResponse(nil, 40001)
		if err != nil {
			t.Fatalf("ErrorResponse() error = %v, want nil", err)
		}
		if response != nil {
			t.Fatalf("Response = %#v, want nil", response)
		}
	})
}
