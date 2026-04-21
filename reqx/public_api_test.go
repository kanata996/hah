package reqx_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/kanata996/hah/reqx"
)

type publicHTTPError interface {
	Status() int
	Code() string
	Detail() string
	Errors() []reqx.FieldError
}

func TestInvalidRequest_UsesReqxPublicFieldErrorSurface(t *testing.T) {
	err := reqx.InvalidRequest(reqx.FieldError{
		Field: "name",
		In:    reqx.InBody,
		Code:  reqx.CodeRequired,
	})
	if err == nil {
		t.Fatal("InvalidRequest() error = nil, want non-nil")
	}

	var httpErr publicHTTPError
	if !errors.As(err, &httpErr) || httpErr == nil {
		t.Fatalf("error type = %T, want public HTTP error surface", err)
	}

	if got := httpErr.Status(); got != http.StatusUnprocessableEntity {
		t.Fatalf("Status() = %d, want %d", got, http.StatusUnprocessableEntity)
	}
	if got := httpErr.Code(); got != "invalid_request" {
		t.Fatalf("Code() = %q, want %q", got, "invalid_request")
	}
	if got := httpErr.Detail(); got != "request contains invalid fields" {
		t.Fatalf("Detail() = %q, want %q", got, "request contains invalid fields")
	}

	fieldErrors := httpErr.Errors()
	want := []reqx.FieldError{{
		Field:  "name",
		In:     reqx.InBody,
		Code:   reqx.CodeRequired,
		Detail: "is required",
	}}
	if len(fieldErrors) != len(want) {
		t.Fatalf("field errors len = %d, want %d", len(fieldErrors), len(want))
	}
	for i := range want {
		if fieldErrors[i] != want[i] {
			t.Fatalf("fieldErrors[%d] = %#v, want %#v", i, fieldErrors[i], want[i])
		}
	}
}
