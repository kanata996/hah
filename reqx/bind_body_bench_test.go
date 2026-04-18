package reqx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type benchmarkBodyRequest struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

func BenchmarkBindBody_SmallJSON(b *testing.B) {
	b.ReportAllocs()

	const payload = `{"name":"kanata","age":17}`

	for b.Loop() {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")

		var dst benchmarkBodyRequest
		if err := BindBody(req, &dst); err != nil {
			b.Fatal(err)
		}
	}
}
