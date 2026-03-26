package hah_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kanata996/hah"
)

func BenchmarkWriteErrorImmediate(b *testing.B) {
	req := httptest.NewRequest(http.MethodGet, "/reports/heavy", nil)
	err := hah.NewHTTPError(http.StatusTooManyRequests, "rate_limited", "rate limit exceeded")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		hah.WriteError(rr, req, err)
	}
}

func BenchmarkDecodeAndValidateJSON(b *testing.B) {
	type createUserRequest struct {
		Name string `json:"name"`
		Role string `json:"role"`
	}

	body := `{"name":"alice","role":"admin"}`
	validate := func(value *createUserRequest) []hah.Violation {
		if strings.TrimSpace(value.Name) == "" {
			return []hah.Violation{{
				Field:   "name",
				Code:    "required",
				Message: "is required",
			}}
		}
		return nil
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(body)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		var input createUserRequest
		if err := hah.DecodeAndValidateJSON(req, &input, validate); err != nil {
			b.Fatalf("DecodeAndValidateJSON() error = %v", err)
		}
	}
}
