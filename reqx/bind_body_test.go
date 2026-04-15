package reqx

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type byteThenReadErrorCloser struct {
	done bool
	err  error
}

func (r *byteThenReadErrorCloser) Read(p []byte) (int, error) {
	if r.done {
		return 0, r.err
	}
	r.done = true
	p[0] = '{'
	return 1, nil
}

func (r *byteThenReadErrorCloser) Close() error {
	return nil
}

func TestBindBody_ContentTypeContract(t *testing.T) {
	type request struct {
		Name string `json:"name"`
	}

	t.Run("accepts application json", func(t *testing.T) {
		req := newJSONRequest(http.MethodPost, "/", `{"name":"kanata"}`)

		var dst request
		if err := BindBody(req, &dst); err != nil {
			t.Fatalf("BindBody() error = %v", err)
		}
		if dst.Name != "kanata" {
			t.Fatalf("name = %q, want kanata", dst.Name)
		}
	})

	t.Run("rejects missing content type for non empty body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"kanata"}`))

		dst := request{Name: "existing"}
		_ = assertHTTPError(
			t,
			BindBody(req, &dst),
			http.StatusUnsupportedMediaType,
			CodeUnsupportedMediaType,
			"Content-Type must be application/json",
		)
		if dst.Name != "existing" {
			t.Fatalf("name = %q, want existing value preserved before decode starts", dst.Name)
		}
	})

	t.Run("rejects application json suffix media type", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"kanata"}`))
		req.Header.Set("Content-Type", "application/problem+json")

		var dst request
		_ = assertHTTPError(
			t,
			BindBody(req, &dst),
			http.StatusUnsupportedMediaType,
			CodeUnsupportedMediaType,
			"Content-Type must be application/json",
		)
	})

	t.Run("rejects representative unsupported media types", func(t *testing.T) {
		cases := []struct {
			name        string
			contentType string
			body        string
		}{
			{name: "plain text", contentType: "text/plain", body: `{"name":"kanata"}`},
			{name: "multipart form", contentType: "multipart/form-data; boundary=boundary", body: `--boundary`},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))
				req.Header.Set("Content-Type", tc.contentType)

				var dst request
				_ = assertHTTPError(
					t,
					BindBody(req, &dst),
					http.StatusUnsupportedMediaType,
					CodeUnsupportedMediaType,
					"Content-Type must be application/json",
				)
			})
		}
	})

	t.Run("accepts mixed-case application json with params", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"kanata"}`))
		req.Header.Set("Content-Type", "Application/JSON; charset=utf-8")

		var dst request
		if err := BindBody(req, &dst); err != nil {
			t.Fatalf("BindBody() error = %v", err)
		}
		if dst.Name != "kanata" {
			t.Fatalf("name = %q, want kanata", dst.Name)
		}
	})

	t.Run("rejects malformed content type", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"kanata"}`))
		req.Header.Set("Content-Type", `application/json; charset="utf-8`)

		var dst request
		_ = assertHTTPError(
			t,
			BindBody(req, &dst),
			http.StatusUnsupportedMediaType,
			CodeUnsupportedMediaType,
			"Content-Type must be application/json",
		)
	})

	t.Run("surfaces body read errors for application json requests", func(t *testing.T) {
		wantErr := errors.New("read failed")
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.ContentLength = -1
		req.Header.Set("Content-Type", mimeApplicationJSON)
		req.Body = &byteThenReadErrorCloser{err: wantErr}

		dst := request{Name: "existing"}
		if err := BindBody(req, &dst); !errors.Is(err, wantErr) {
			t.Fatalf("BindBody() error = %v, want %v", err, wantErr)
		}
		if dst.Name != "existing" {
			t.Fatalf("name = %q, want existing value preserved when body read fails", dst.Name)
		}
	})
}

func TestBindBody_EmptyBodyContract(t *testing.T) {
	type request struct {
		Name string `json:"name"`
	}

	t.Run("content length zero is a noop even with invalid content type", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))
		req.Header.Set("Content-Type", "text/plain")
		req.ContentLength = 0

		dst := request{Name: "kanata"}
		if err := BindBody(req, &dst); err != nil {
			t.Fatalf("BindBody() error = %v", err)
		}
		if dst.Name != "kanata" {
			t.Fatalf("name = %q, want kanata", dst.Name)
		}
	})

	t.Run("unknown length empty body is also a noop", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))
		req.Header.Set("Content-Type", "text/plain")
		req.ContentLength = -1

		dst := request{Name: "kanata"}
		if err := BindBody(req, &dst); err != nil {
			t.Fatalf("BindBody() error = %v", err)
		}
		if dst.Name != "kanata" {
			t.Fatalf("name = %q, want kanata", dst.Name)
		}
	})

	t.Run("whitespace body is not treated as empty when content length is non zero", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(" \n\t "))
		req.Header.Set("Content-Type", mimeApplicationJSON)
		req.ContentLength = int64(len(" \n\t "))

		dst := request{Name: "kanata"}
		_ = assertHTTPError(t, BindBody(req, &dst), http.StatusBadRequest, CodeInvalidJSON, "request body must be valid JSON")
		if dst.Name != "kanata" {
			t.Fatalf("name = %q, want kanata", dst.Name)
		}
	})
}

func TestBindBody_EmptyBodyPreservesExistingValues(t *testing.T) {
	type request struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))
	req.Header.Set("Content-Type", mimeApplicationJSON)
	req.ContentLength = 0

	dst := request{Name: "kanata", Age: 17}
	if err := BindBody(req, &dst); err != nil {
		t.Fatalf("BindBody() error = %v", err)
	}
	if dst.Name != "kanata" || dst.Age != 17 {
		t.Fatalf("dst = %#v, want existing values preserved", dst)
	}
}

func TestBindBody_UnknownLengthNonEmptyBodyStillBinds(t *testing.T) {
	type request struct {
		Name string `json:"name"`
	}

	req := newJSONRequest(http.MethodPost, "/", `{"name":"kanata"}`)
	req.ContentLength = -1

	var dst request
	if err := BindBody(req, &dst); err != nil {
		t.Fatalf("BindBody() error = %v", err)
	}
	if dst.Name != "kanata" {
		t.Fatalf("name = %q, want kanata", dst.Name)
	}
}

func TestBindBody_PresenceProbeErrorsSurfaceBeforeRead(t *testing.T) {
	type request struct {
		Name string `json:"name"`
	}

	wantErr := errors.New("probe failed")
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.ContentLength = -1
	req.Header.Set("Content-Type", mimeApplicationJSON)
	req.Body = probeErrorReadCloser{err: wantErr}

	dst := request{Name: "existing"}
	if err := BindBody(req, &dst); !errors.Is(err, wantErr) {
		t.Fatalf("BindBody() error = %v, want %v", err, wantErr)
	}
	if dst.Name != "existing" {
		t.Fatalf("name = %q, want existing value preserved when presence probe fails", dst.Name)
	}
}

func TestBindBody_JSONContract(t *testing.T) {
	t.Run("array binds to slice target", func(t *testing.T) {
		type item struct {
			Name string `json:"name"`
		}

		req := newJSONRequest(http.MethodPost, "/", `[{"name":"a"},{"name":"b"}]`)
		var dst []item
		if err := BindBody(req, &dst); err != nil {
			t.Fatalf("BindBody() error = %v", err)
		}
		if len(dst) != 2 || dst[0].Name != "a" || dst[1].Name != "b" {
			t.Fatalf("dst = %#v", dst)
		}
	})

	t.Run("type mismatch returns bad request", func(t *testing.T) {
		type request struct {
			Name string `json:"name"`
			Age  int    `json:"age"`
		}

		req := newJSONRequest(http.MethodPost, "/", `{"name":"kanata","age":"oops"}`)
		dst := request{Name: "existing", Age: 17}
		_ = assertHTTPError(t, BindBody(req, &dst), http.StatusBadRequest, CodeInvalidJSON, "request body must be valid JSON")
		if dst.Name != "kanata" || dst.Age != 17 {
			t.Fatalf("dst = %#v, want earlier decoded fields preserved and failing field unchanged", dst)
		}
	})

	t.Run("omitted fields preserve existing values on success", func(t *testing.T) {
		type request struct {
			Name string `json:"name"`
			Age  int    `json:"age"`
		}

		req := newJSONRequest(http.MethodPost, "/", `{"name":"kanata"}`)
		dst := request{Name: "existing", Age: 17}

		if err := BindBody(req, &dst); err != nil {
			t.Fatalf("BindBody() error = %v", err)
		}
		if dst.Name != "kanata" || dst.Age != 17 {
			t.Fatalf("dst = %#v, want updated name with omitted age preserved", dst)
		}
	})

	t.Run("unknown fields are accepted by default", func(t *testing.T) {
		type request struct {
			Name string `json:"name"`
		}

		req := newJSONRequest(http.MethodPost, "/", `{"name":"kanata","extra":1}`)

		var dst request
		if err := BindBody(req, &dst); err != nil {
			t.Fatalf("BindBody() error = %v", err)
		}
		if dst.Name != "kanata" {
			t.Fatalf("name = %q, want kanata", dst.Name)
		}
	})

	t.Run("rejects trailing garbage after valid json", func(t *testing.T) {
		type request struct {
			Name string `json:"name"`
			Age  int    `json:"age"`
		}

		req := newJSONRequest(http.MethodPost, "/", `{"name":"kanata","age":17}xxx`)

		var dst request
		_ = assertHTTPError(t, BindBody(req, &dst), http.StatusBadRequest, CodeInvalidJSON, "request body must be valid JSON")
	})

	t.Run("rejects multiple top level json values", func(t *testing.T) {
		type request struct {
			Name string `json:"name"`
			Age  int    `json:"age"`
		}

		req := newJSONRequest(http.MethodPost, "/", `{"name":"kanata","age":17}{"name":"other"}`)

		var dst request
		_ = assertHTTPError(t, BindBody(req, &dst), http.StatusBadRequest, CodeInvalidJSON, "request body must be valid JSON")
	})

	t.Run("object binds to map target", func(t *testing.T) {
		req := newJSONRequest(http.MethodPost, "/", `{"name":"kanata","age":17}`)

		var dst map[string]any
		if err := BindBody(req, &dst); err != nil {
			t.Fatalf("BindBody() error = %v", err)
		}
		if got := dst["name"]; got != "kanata" {
			t.Fatalf("name = %#v, want kanata", got)
		}
		if got := dst["age"]; got != float64(17) {
			t.Fatalf("age = %#v, want 17", got)
		}
	})
}

func TestBindBody_RequestSizeLimitContract(t *testing.T) {
	type request struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	makeJSONBody := func(size int64) []byte {
		t.Helper()

		const envelope = `{"name":""}`
		payloadLen := int(size) - len(envelope)
		if payloadLen < 0 {
			t.Fatalf("size = %d, smaller than minimal JSON envelope", size)
		}
		return []byte(`{"name":"` + strings.Repeat("a", payloadLen) + `"}`)
	}

	t.Run("accepts body exactly at default limit", func(t *testing.T) {
		body := makeJSONBody(defaultMaxBodyBytes)
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		req.Header.Set("Content-Type", mimeApplicationJSON)
		req.ContentLength = int64(len(body))

		var dst request
		if err := BindBody(req, &dst); err != nil {
			t.Fatalf("BindBody() error = %v", err)
		}
		if got, want := len(dst.Name), len(body)-len(`{"name":""}`); got != want {
			t.Fatalf("len(name) = %d, want %d", got, want)
		}
	})

	t.Run("rejects body larger than default limit by one byte", func(t *testing.T) {
		body := makeJSONBody(defaultMaxBodyBytes + 1)
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		req.Header.Set("Content-Type", mimeApplicationJSON)
		req.ContentLength = int64(len(body))

		dst := request{Name: "existing", Age: 17}
		_ = assertHTTPError(t, BindBody(req, &dst), http.StatusRequestEntityTooLarge, CodeRequestTooLarge, "request body is too large")
		if dst.Name != "existing" || dst.Age != 17 {
			t.Fatalf("dst = %#v, want existing values preserved on oversized body", dst)
		}
	})

	t.Run("rejects unknown length body larger than default limit by one byte", func(t *testing.T) {
		body := makeJSONBody(defaultMaxBodyBytes + 1)
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		req.Header.Set("Content-Type", mimeApplicationJSON)
		req.ContentLength = -1

		dst := request{Name: "existing", Age: 17}
		_ = assertHTTPError(t, BindBody(req, &dst), http.StatusRequestEntityTooLarge, CodeRequestTooLarge, "request body is too large")
		if dst.Name != "existing" || dst.Age != 17 {
			t.Fatalf("dst = %#v, want existing values preserved on oversized unknown-length body", dst)
		}
	})
}

func TestBindBody_NilBodyIsNoop(t *testing.T) {
	type request struct {
		Name string `json:"name"`
	}

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Body = nil

	dst := request{Name: "existing"}
	if err := BindBody(req, &dst); err != nil {
		t.Fatalf("BindBody(nil body) error = %v", err)
	}
	if dst.Name != "existing" {
		t.Fatalf("name = %q, want existing value preserved", dst.Name)
	}
}

func TestBindBody_NilBodyAfterCachedPresenceProbeIsNoop(t *testing.T) {
	type request struct {
		Name string `json:"name"`
	}

	req := newJSONRequest(http.MethodPost, "/", `{"name":"kanata"}`)

	var first request
	if err := BindBody(req, &first); err != nil {
		t.Fatalf("first BindBody() error = %v", err)
	}
	if first.Name != "kanata" {
		t.Fatalf("first name = %q, want kanata", first.Name)
	}

	req.Body = nil

	dst := request{Name: "existing"}
	if err := BindBody(req, &dst); err != nil {
		t.Fatalf("second BindBody(nil body after cached probe) error = %v", err)
	}
	if dst.Name != "existing" {
		t.Fatalf("name = %q, want existing value preserved", dst.Name)
	}
}

type probeStaticReadCloser struct {
	data []byte
}

func (r *probeStaticReadCloser) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	if len(r.data) == 0 {
		return n, io.EOF
	}
	return n, nil
}

func (r *probeStaticReadCloser) Close() error {
	return nil
}

type probeByteThenErrorReadCloser struct {
	done bool
	err  error
}

func (r *probeByteThenErrorReadCloser) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if r.done {
		return 0, io.EOF
	}
	r.done = true
	p[0] = 'x'
	return 1, r.err
}

func (r *probeByteThenErrorReadCloser) Close() error {
	return nil
}

type probeErrorReadCloser struct {
	err error
}

func (r probeErrorReadCloser) Read(_ []byte) (int, error) {
	return 0, r.err
}

func (r probeErrorReadCloser) Close() error {
	return nil
}

func TestHasRequestBody(t *testing.T) {
	t.Run("nil request", func(t *testing.T) {
		if got, err := hasRequestBody(nil); err != nil || got {
			t.Fatalf("hasRequestBody(nil) = (%v, %v), want (false, nil)", got, err)
		}
	})

	t.Run("nil body is treated as empty", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Body = nil

		if got, err := hasRequestBody(req); err != nil || got {
			t.Fatalf("hasRequestBody(nil body) = (%v, %v), want (false, nil)", got, err)
		}
	})

	t.Run("non-empty body is detected and cached", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Body = &probeStaticReadCloser{data: []byte(`{"name":"kanata"}`)}

		if got, err := hasRequestBody(req); err != nil || !got {
			t.Fatalf("hasRequestBody(non-empty) = (%v, %v), want (true, nil)", got, err)
		}

		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("ReadAll(replayed body) error = %v", err)
		}
		if string(body) != `{"name":"kanata"}` {
			t.Fatalf("replayed body = %q, want original body", body)
		}

		if got, err := hasRequestBody(req); err != nil || !got {
			t.Fatalf("hasRequestBody(second call after read) = (%v, %v), want (true, nil)", got, err)
		}
	})

	t.Run("empty body is detected and cached", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Body = &probeStaticReadCloser{}

		if got, err := hasRequestBody(req); err != nil || got {
			t.Fatalf("hasRequestBody(empty) = (%v, %v), want (false, nil)", got, err)
		}

		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("ReadAll(empty replayed body) error = %v", err)
		}
		if len(body) != 0 {
			t.Fatalf("empty replayed body = %q, want empty", body)
		}

		if got, err := hasRequestBody(req); err != nil || got {
			t.Fatalf("hasRequestBody(second call after empty read) = (%v, %v), want (false, nil)", got, err)
		}
	})

	t.Run("read error after a byte preserves replay body", func(t *testing.T) {
		wantErr := errors.New("boom")
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Body = &probeByteThenErrorReadCloser{err: wantErr}

		if got, err := hasRequestBody(req); !errors.Is(err, wantErr) || got {
			t.Fatalf("hasRequestBody(byte+error) = (%v, %v), want (false, %v)", got, err, wantErr)
		}

		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("ReadAll(replayed error body) error = %v", err)
		}
		if string(body) != "x" {
			t.Fatalf("replayed error body = %q, want x", body)
		}
	})

	t.Run("read error without bytes is returned and not cached", func(t *testing.T) {
		wantErr := errors.New("read failed")
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Body = probeErrorReadCloser{err: wantErr}

		if got, err := hasRequestBody(req); !errors.Is(err, wantErr) || got {
			t.Fatalf("hasRequestBody(read error) = (%v, %v), want (false, %v)", got, err, wantErr)
		}

		if _, err := io.ReadAll(req.Body); !errors.Is(err, wantErr) {
			t.Fatalf("ReadAll(error body) error = %v, want %v", err, wantErr)
		}

		if got, err := hasRequestBody(req); !errors.Is(err, wantErr) || got {
			t.Fatalf("hasRequestBody(second read error) = (%v, %v), want (false, %v)", got, err, wantErr)
		}
	})
}
