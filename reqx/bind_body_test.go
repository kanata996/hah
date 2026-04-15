package reqx

import (
	"bytes"
	"errors"
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

	makeJSONBody := func(size int) []byte {
		t.Helper()

		const envelope = `{"name":""}`
		payloadLen := size - len(envelope)
		if payloadLen < 0 {
			t.Fatalf("size = %d, smaller than minimal JSON envelope", size)
		}
		return []byte(`{"name":"` + strings.Repeat("a", payloadLen) + `"}`)
	}

	findOversizedJSONBody := func() []byte {
		t.Helper()

		const maxProbeBodyBytes = 32 << 20
		for size := 1 << 10; size <= maxProbeBodyBytes; size <<= 1 {
			body := makeJSONBody(size)
			req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
			req.Header.Set("Content-Type", mimeApplicationJSON)
			req.ContentLength = int64(len(body))

			var dst request
			err := BindBody(req, &dst)
			if err == nil {
				continue
			}

			httpErr := assertHTTPErrorLike(t, err)
			if httpErr.Status() == http.StatusRequestEntityTooLarge &&
				httpErr.Code() == CodeRequestTooLarge &&
				httpErr.Detail() == "request body is too large" {
				return body
			}

			t.Fatalf("BindBody() probe error = %v, want nil or request_too_large", err)
		}

		t.Fatal("failed to find oversized body within probe range")
		return nil
	}

	oversizedBody := findOversizedJSONBody()

	t.Run("rejects representative oversized body with content length", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(oversizedBody))
		req.Header.Set("Content-Type", mimeApplicationJSON)
		req.ContentLength = int64(len(oversizedBody))

		dst := request{Name: "existing", Age: 17}
		_ = assertHTTPError(t, BindBody(req, &dst), http.StatusRequestEntityTooLarge, CodeRequestTooLarge, "request body is too large")
		if dst.Name != "existing" || dst.Age != 17 {
			t.Fatalf("dst = %#v, want existing values preserved on oversized body", dst)
		}
	})

	t.Run("rejects representative oversized body with unknown length", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(oversizedBody))
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

type probeErrorReadCloser struct {
	err error
}

func (r probeErrorReadCloser) Read(_ []byte) (int, error) {
	return 0, r.err
}

func (r probeErrorReadCloser) Close() error {
	return nil
}
