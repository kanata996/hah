package resp

// 测试清单：
// - 标记说明：[✓] 已核对且已有真实覆盖；[x] 尚未完成，不得作为验收依据。
// - [✓] `JSON` 在典型小型响应体上的基准性能。
// - [✓] `JSONBlob` 直写原始 JSON 字节的基准性能。
// - [✓] `WriteError` 在 4xx 校验错误场景下的基准性能。
// - [✓] `WriteError` 在 5xx 服务端错误场景下的基准性能。

import (
	"errors"
	"net/http"
	"testing"

	"github.com/kanata996/hah/errx"
)

type benchmarkResponseWriter struct {
	header http.Header
}

type benchmarkSuccessProfile struct {
	Plan   string   `json:"plan"`
	Region string   `json:"region"`
	Tags   []string `json:"tags"`
}

type benchmarkSuccessPayload struct {
	ID      string                  `json:"id"`
	Name    string                  `json:"name"`
	Email   string                  `json:"email"`
	Active  bool                    `json:"active"`
	Roles   []string                `json:"roles"`
	Profile benchmarkSuccessProfile `json:"profile"`
}

type benchmarkValidationError struct {
	Field  string `json:"field"`
	Reason string `json:"reason"`
}

var (
	benchmarkJSONPayload = benchmarkSuccessPayload{
		ID:     "acct_123456",
		Name:   "kanata",
		Email:  "kanata@example.com",
		Active: true,
		Roles:  []string{"owner", "billing"},
		Profile: benchmarkSuccessProfile{
			Plan:   "pro",
			Region: "ap-southeast-1",
			Tags:   []string{"prod", "priority"},
		},
	}
	benchmarkJSONBlobPayload = []byte(`{"id":"acct_123456","name":"kanata","email":"kanata@example.com","active":true,"roles":["owner","billing"],"profile":{"plan":"pro","region":"ap-southeast-1","tags":["prod","priority"]}}`)
	benchmarkClientHTTPError = errx.UnprocessableEntity(
		"validation_failed",
		"request validation failed",
		benchmarkValidationError{Field: "email", Reason: "must be a valid email"},
		benchmarkValidationError{Field: "name", Reason: "must not be blank"},
	)
	errBenchmarkServer = errors.New("dial tcp 10.0.0.7:5432: connect: connection reset by peer")
)

func (w *benchmarkResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header, 1)
	}
	return w.header
}

func (w *benchmarkResponseWriter) WriteHeader(_ int) {}

func (w *benchmarkResponseWriter) Write(p []byte) (int, error) {
	return len(p), nil
}

func BenchmarkJSON_Typical(b *testing.B) {
	b.ReportAllocs()

	w := &benchmarkResponseWriter{header: make(http.Header, 1)}

	for b.Loop() {
		if err := JSON(w, http.StatusOK, benchmarkJSONPayload); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkJSONBlob_Typical(b *testing.B) {
	b.ReportAllocs()

	w := &benchmarkResponseWriter{header: make(http.Header, 1)}

	for b.Loop() {
		if err := JSONBlob(w, http.StatusOK, benchmarkJSONBlobPayload); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWriteError_ClientError422(b *testing.B) {
	b.ReportAllocs()

	w := &benchmarkResponseWriter{header: make(http.Header, 1)}

	for b.Loop() {
		if err := WriteError(w, benchmarkClientHTTPError); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWriteError_ServerError500(b *testing.B) {
	b.ReportAllocs()

	w := &benchmarkResponseWriter{header: make(http.Header, 1)}

	for b.Loop() {
		if err := WriteError(w, errBenchmarkServer); err != nil {
			b.Fatal(err)
		}
	}
}
