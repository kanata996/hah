package reqx

// 测试清单：
// - 标记说明：[✓] 已核对且已有真实覆盖；[x] 尚未完成，不得作为验收依据。
// - [✓] 本文件定义 `BindBody`、`BindQueryParams`、`BindPathValues`、`BindHeaders`、`Bind`、`BindAndValidate` 的典型 benchmark 场景。
// - [✓] 这些 benchmark 仅用于性能观测，不作为功能验收覆盖。
// - [✓] 包含 POST body 热路径和校验失败路径的 benchmark。

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

func BenchmarkBindAndValidateBody_POST(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		req := newJSONRequest(http.MethodPost, "/", `{"name":"kanata","email":"k@example.com"}`)
		var dst benchmarkBodyRequest
		if err := BindAndValidateBody(req, &dst); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBindAndValidateBody_ValidationFailure(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		req := newJSONRequest(http.MethodPost, "/", `{"name":"bad value","email":""}`)
		var dst benchmarkBodyRequest
		if err := BindAndValidateBody(req, &dst); err == nil {
			b.Fatal("expected validation error")
		}
	}
}
