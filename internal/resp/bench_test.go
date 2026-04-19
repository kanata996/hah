package resp

import (
	"errors"
	"net/http"
	"testing"

	"github.com/kanata996/hah/internal/errx"
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
	benchmarkClientHTTPError = errx.UnprocessableEntity(
		"validation_failed",
		"request validation failed",
	).WithViolations([]errx.Violation{
		{Field: "email", In: errx.InBody, Code: errx.CodeInvalid, Detail: "must be a valid email"},
		{Field: "name", In: errx.InBody, Code: errx.CodeRequired, Detail: "must not be blank"},
	})
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
