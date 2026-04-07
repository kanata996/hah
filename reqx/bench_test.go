package reqx

// 测试清单：
// - 标记说明：[✓] 已核对且已有真实覆盖；[x] 尚未完成，不得作为验收依据。
// - [✓] 本文件定义 `BindBody`、`BindQueryParams`、`BindPathValues`、`BindHeaders`、`Bind`、`BindAndValidate` 的典型 benchmark 场景。
// - [✓] 这些 benchmark 仅用于性能观测，不作为功能验收覆盖。

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

type benchmarkQueryRequest struct {
	Name    string   `query:"name"`
	Page    int      `query:"page"`
	Enabled bool     `query:"enabled"`
	Tags    []string `query:"tag"`
}

type benchmarkPathRequest struct {
	ID   int    `param:"id"`
	Name string `param:"name"`
}

type benchmarkHeaderRequest struct {
	RequestID string `header:"x-request-id"`
	Retry     int    `header:"x-retry"`
	Enabled   bool   `header:"x-enabled"`
}

type benchmarkBindRequest struct {
	ID      string `param:"id" query:"id" json:"id"`
	Name    string `query:"name" json:"name" validate:"required,nospace"`
	Page    int    `query:"page"`
	Enabled bool   `query:"enabled"`
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

func BenchmarkBindQueryParams_Typical(b *testing.B) {
	b.ReportAllocs()

	req := httptest.NewRequest(http.MethodGet, "/?name=kanata&page=2&enabled=true&tag=a&tag=b", nil)

	for b.Loop() {
		var dst benchmarkQueryRequest
		if err := BindQueryParams(req, &dst); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBindPathValues_Typical(b *testing.B) {
	b.ReportAllocs()

	req := requestWithPathParams(map[string][]string{
		"id":   {"42"},
		"name": {"kanata"},
	})

	for b.Loop() {
		var dst benchmarkPathRequest
		if err := BindPathValues(req, &dst); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBindHeaders_Typical(b *testing.B) {
	b.ReportAllocs()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-Id", "req-123")
	req.Header.Set("X-Retry", "2")
	req.Header.Set("X-Enabled", "true")

	for b.Loop() {
		var dst benchmarkHeaderRequest
		if err := BindHeaders(req, &dst); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBind_RequestGET(b *testing.B) {
	b.ReportAllocs()

	req := requestWithPathParams(map[string][]string{
		"id": {"route-id"},
	})
	req.Method = http.MethodGet
	req.URL.RawQuery = "id=query-id&name=kanata&page=2&enabled=true"

	for b.Loop() {
		var dst benchmarkBindRequest
		if err := Bind(req, &dst); err != nil {
			b.Fatal(err)
		}
	}
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
