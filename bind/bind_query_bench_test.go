package bind

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

type benchmarkQueryRequest struct {
	Name    string   `query:"name"`
	Page    int      `query:"page"`
	Enabled bool     `query:"enabled"`
	Tags    []string `query:"tag"`
}

func BenchmarkBindQuery_Typical(b *testing.B) {
	b.ReportAllocs()

	req := httptest.NewRequest(http.MethodGet, "/?name=kanata&page=2&enabled=true&tag=a&tag=b", nil)

	for b.Loop() {
		var dst benchmarkQueryRequest
		if err := BindQuery(req, &dst); err != nil {
			b.Fatal(err)
		}
	}
}
