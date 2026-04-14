package reqx

import (
	"net/http"
	"testing"
)

type benchmarkBindRequest struct {
	ID      string `param:"id" query:"id" json:"id"`
	Name    string `query:"name" json:"name" validate:"required,nospace"`
	Page    int    `query:"page"`
	Enabled bool   `query:"enabled"`
}

func BenchmarkBindAndValidate_RequestGET(b *testing.B) {
	b.ReportAllocs()

	req := requestWithPathParams(map[string][]string{
		"id": {"route-id"},
	})
	req.Method = http.MethodGet
	req.URL.RawQuery = "id=query-id&name=kanata&page=2&enabled=true"

	for b.Loop() {
		var dst benchmarkBindRequest
		if err := BindAndValidate(req, &dst); err != nil {
			b.Fatal(err)
		}
	}
}

type benchmarkBodyRequest struct {
	Name  string `json:"name" validate:"required,nospace"`
	Email string `json:"email" validate:"required"`
}

func BenchmarkBindBodyThenValidate_POST(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		req := newJSONRequest(http.MethodPost, "/", `{"name":"kanata","email":"k@example.com"}`)
		var dst benchmarkBodyRequest
		if err := bindAndValidateBody(req, &dst); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBindBodyThenValidate_ValidationFailure(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		req := newJSONRequest(http.MethodPost, "/", `{"name":"bad value","email":""}`)
		var dst benchmarkBodyRequest
		if err := bindAndValidateBody(req, &dst); err == nil {
			b.Fatal("expected validation error")
		}
	}
}
