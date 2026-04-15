package reqx

import (
	"io"
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

	req := httptest.NewRequest(http.MethodPost, "/", http.NoBody)
	req.Header.Set("Content-Type", "application/json")
	const payload = `{"name":"kanata","age":17}`

	for b.Loop() {
		req.Body = io.NopCloser(strings.NewReader(payload))

		var dst benchmarkBodyRequest
		if err := BindBody(req, &dst); err != nil {
			b.Fatal(err)
		}
	}
}
