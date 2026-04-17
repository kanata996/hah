package reqx

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

type bindBodyNamedTags []string

type bindBodyReadErrorCloser struct{ err error }

func (r bindBodyReadErrorCloser) Read([]byte) (int, error) { return 0, r.err }
func (r bindBodyReadErrorCloser) Close() error             { return nil }

func TestBindBody_Contracts(t *testing.T) {
	t.Run("zero byte body is noop and does not require json content type", func(t *testing.T) {
		type request struct {
			Name string `json:"name"`
		}

		req := httptest.NewRequest(http.MethodPost, "/", http.NoBody)
		dst := request{Name: "existing"}

		if err := BindBody(req, &dst); err != nil {
			t.Fatalf("BindBody() error = %v", err)
		}
		if dst != (request{Name: "existing"}) {
			t.Fatalf("dst = %#v, want unchanged", dst)
		}
	})

	t.Run("non empty body only accepts application json", func(t *testing.T) {
		type request struct {
			Name string `json:"name"`
		}

		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"kanata"}`))
		req.Header.Set("Content-Type", "application/problem+json")

		err := BindBody(req, &request{})
		_ = assertHTTPStatusCode(t, err, http.StatusUnsupportedMediaType, CodeUnsupportedMediaType)
	})

	t.Run("supports nested struct family named slice family and time", func(t *testing.T) {
		type address struct {
			Street string `json:"street"`
		}
		type request struct {
			Name    string            `json:"name"`
			Address address           `json:"address"`
			Tags    bindBodyNamedTags `json:"tags"`
			When    time.Time         `json:"when"`
		}

		req := newJSONRequest(http.MethodPost, "/", `{"name":"kanata","address":{"street":"main"},"tags":["a","b"],"when":"2026-04-13T10:00:00Z"}`)

		var dst request
		if err := BindBody(req, &dst); err != nil {
			t.Fatalf("BindBody() error = %v", err)
		}
		if dst.Name != "kanata" || dst.Address.Street != "main" || !reflect.DeepEqual(dst.Tags, bindBodyNamedTags{"a", "b"}) {
			t.Fatalf("dst = %#v, want nested struct and named slice bound", dst)
		}
		if got := dst.When.UTC().Format(time.RFC3339); got != "2026-04-13T10:00:00Z" {
			t.Fatalf("when = %q, want 2026-04-13T10:00:00Z", got)
		}
	})

	t.Run("missing fields do not inherit previous target values", func(t *testing.T) {
		type request struct {
			Name string `json:"name"`
			Age  int    `json:"age"`
		}

		req := newJSONRequest(http.MethodPost, "/", `{"name":"kanata"}`)
		dst := request{Name: "existing", Age: 17}

		if err := BindBody(req, &dst); err != nil {
			t.Fatalf("BindBody() error = %v", err)
		}
		if dst != (request{Name: "kanata"}) {
			t.Fatalf("dst = %#v, want zero-based replacement semantics", dst)
		}
	})

	t.Run("recursive unknown field is invalid json and target unchanged", func(t *testing.T) {
		type address struct {
			Street string `json:"street"`
		}
		type request struct {
			Address address `json:"address"`
		}

		req := newJSONRequest(http.MethodPost, "/", `{"address":{"street":"main","extra":"x"}}`)
		dst := request{Address: address{Street: "existing"}}

		err := BindBody(req, &dst)
		_ = assertHTTPStatusCode(t, err, http.StatusBadRequest, CodeInvalidJSON)
		if dst != (request{Address: address{Street: "existing"}}) {
			t.Fatalf("dst = %#v, want unchanged", dst)
		}
	})

	t.Run("duplicate object key is invalid json", func(t *testing.T) {
		type request struct {
			Name string `json:"name"`
		}

		err := BindBody(newJSONRequest(http.MethodPost, "/", `{"name":"first","name":"second"}`), &request{})
		_ = assertHTTPStatusCode(t, err, http.StatusBadRequest, CodeInvalidJSON)
	})

	t.Run("top level non object is invalid json", func(t *testing.T) {
		type request struct {
			Name string `json:"name"`
		}

		err := BindBody(newJSONRequest(http.MethodPost, "/", `[]`), &request{})
		_ = assertHTTPStatusCode(t, err, http.StatusBadRequest, CodeInvalidJSON)
	})

	t.Run("unsupported field family is usage error", func(t *testing.T) {
		type request struct {
			Raw json.RawMessage `json:"raw"`
		}

		assertNotHTTPError(t, BindBody(newJSONRequest(http.MethodPost, "/", `{"raw":{}}`), &request{}))
	})

	t.Run("body read error returns ordinary error", func(t *testing.T) {
		type request struct {
			Name string `json:"name"`
		}

		wantErr := errors.New("read failed")
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Header.Set("Content-Type", "application/json")
		req.Body = bindBodyReadErrorCloser{err: wantErr}
		req.ContentLength = -1

		err := BindBody(req, &request{})
		if !errors.Is(err, wantErr) {
			t.Fatalf("BindBody() error = %v, want %v", err, wantErr)
		}
	})

	t.Run("whitespace body is invalid json not empty body", func(t *testing.T) {
		type request struct {
			Name string `json:"name"`
		}

		err := BindBody(newJSONRequest(http.MethodPost, "/", " \n\t "), &request{})
		_ = assertHTTPStatusCode(t, err, http.StatusBadRequest, CodeInvalidJSON)
	})

	t.Run("request body nil counts as zero byte body", func(t *testing.T) {
		type request struct {
			Name string `json:"name"`
		}

		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Body = nil
		dst := request{Name: "existing"}

		if err := BindBody(req, &dst); err != nil {
			t.Fatalf("BindBody() error = %v", err)
		}
		if dst != (request{Name: "existing"}) {
			t.Fatalf("dst = %#v, want unchanged", dst)
		}
	})

	t.Run("probe read eof keeps request body usable", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", io.NopCloser(strings.NewReader(`{"name":"kanata"}`)))
		req.Header.Set("Content-Type", "application/json")
		type request struct {
			Name string `json:"name"`
		}

		var dst request
		if err := BindBody(req, &dst); err != nil {
			t.Fatalf("BindBody() error = %v", err)
		}
		if dst.Name != "kanata" {
			t.Fatalf("dst = %#v, want bound body", dst)
		}
	})
}

func TestBindBody_UsageAndBoundaryContracts(t *testing.T) {
	t.Run("rejects invalid request and target shapes", func(t *testing.T) {
		type request struct {
			Name string `json:"name"`
		}

		assertNotHTTPError(t, BindBody(nil, &request{}))
		assertNotHTTPError(t, BindBody(newJSONRequest(http.MethodPost, "/", `{}`), nil))
		assertNotHTTPError(t, BindBody(newJSONRequest(http.MethodPost, "/", `{}`), request{}))

		var typedNil *request
		assertNotHTTPError(t, BindBody(newJSONRequest(http.MethodPost, "/", `{}`), typedNil))

		var unsupported map[string]string
		assertNotHTTPError(t, BindBody(newJSONRequest(http.MethodPost, "/", `{}`), &unsupported))
	})

	t.Run("accepts application json with charset parameter", func(t *testing.T) {
		type request struct {
			Name string `json:"name"`
		}

		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"kanata"}`))
		req.Header.Set("Content-Type", "application/json; charset=utf-8")

		var dst request
		if err := BindBody(req, &dst); err != nil {
			t.Fatalf("BindBody() error = %v", err)
		}
		if dst.Name != "kanata" {
			t.Fatalf("dst = %#v, want bound body", dst)
		}
	})

	t.Run("too large body returns request too large and preserves target", func(t *testing.T) {
		type request struct {
			Name string `json:"name"`
		}

		payload := `{"name":"` + strings.Repeat("a", int(defaultMaxBodyBytes)) + `"}`
		req := newJSONRequest(http.MethodPost, "/", payload)
		dst := request{Name: "existing"}

		err := BindBody(req, &dst)
		_ = assertHTTPStatusCode(t, err, http.StatusRequestEntityTooLarge, CodeRequestTooLarge)
		if dst != (request{Name: "existing"}) {
			t.Fatalf("dst = %#v, want unchanged", dst)
		}
	})

	t.Run("exactly one mib body still binds", func(t *testing.T) {
		type request struct {
			Name string `json:"name"`
		}

		const prefix = `{"name":"`
		const suffix = `"}`
		payload := prefix + strings.Repeat("a", int(defaultMaxBodyBytes)-len(prefix)-len(suffix)) + suffix

		var dst request
		if err := BindBody(newJSONRequest(http.MethodPost, "/", payload), &dst); err != nil {
			t.Fatalf("BindBody() error = %v", err)
		}
		if len(dst.Name) != int(defaultMaxBodyBytes)-len(prefix)-len(suffix) {
			t.Fatalf("name len = %d, want %d", len(dst.Name), int(defaultMaxBodyBytes)-len(prefix)-len(suffix))
		}
	})

	t.Run("malformed content type returns unsupported media type", func(t *testing.T) {
		type request struct {
			Name string `json:"name"`
		}

		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"kanata"}`))
		req.Header.Set("Content-Type", `application/json; charset="utf-8`)

		err := BindBody(req, &request{})
		_ = assertHTTPStatusCode(t, err, http.StatusUnsupportedMediaType, CodeUnsupportedMediaType)
	})

	t.Run("top level null and trailing data are invalid json", func(t *testing.T) {
		type request struct {
			Name string `json:"name"`
		}

		err := BindBody(newJSONRequest(http.MethodPost, "/", `null`), &request{})
		_ = assertHTTPStatusCode(t, err, http.StatusBadRequest, CodeInvalidJSON)

		err = BindBody(newJSONRequest(http.MethodPost, "/", `{"name":"kanata"} true`), &request{})
		_ = assertHTTPStatusCode(t, err, http.StatusBadRequest, CodeInvalidJSON)
	})

	t.Run("unexpected eof and bom are invalid json", func(t *testing.T) {
		type request struct {
			Name string `json:"name"`
		}

		err := BindBody(newJSONRequest(http.MethodPost, "/", `{"name":"kanata"`), &request{})
		_ = assertHTTPStatusCode(t, err, http.StatusBadRequest, CodeInvalidJSON)

		err = BindBody(newJSONRequest(http.MethodPost, "/", "\ufeff{}"), &request{})
		_ = assertHTTPStatusCode(t, err, http.StatusBadRequest, CodeInvalidJSON)
	})

	t.Run("ignored json fields do not trigger usage errors", func(t *testing.T) {
		type request struct {
			Name    string          `json:"name"`
			Ignored json.RawMessage `json:"-"`
			hidden  json.RawMessage
		}

		var dst request
		if err := BindBody(newJSONRequest(http.MethodPost, "/", `{"name":"kanata"}`), &dst); err != nil {
			t.Fatalf("BindBody() error = %v", err)
		}
		if dst.Name != "kanata" || dst.hidden != nil {
			t.Fatalf("dst = %#v, want ignored fields skipped", dst)
		}
	})
}
