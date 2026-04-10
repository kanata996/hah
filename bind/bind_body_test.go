package bind

// 测试清单：
// - 标记说明：[✓] 已核对且已有真实覆盖；[x] 尚未完成，不得作为验收依据。
// - [✓] BindBody 的 Content-Type、空 body、未知字段与非法 JSON 契约。
// - [✓] BindBody 支持标准 decoder 目标，包括 struct、slice、map。
// - [✓] BindBody 在 body 过大时返回稳定的 413 契约，并保留已有值。
// - [✓] body 相关内部辅助维持稳定契约，包括 media type、读 body 和错误映射。

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

type failingReadCloser struct {
	err error
}

func (r failingReadCloser) Read(_ []byte) (int, error) {
	return 0, r.err
}

func (r failingReadCloser) Close() error {
	return nil
}

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

		var dst request
		_ = assertHTTPError(
			t,
			BindBody(req, &dst),
			http.StatusUnsupportedMediaType,
			CodeUnsupportedMediaType,
			"Content-Type must be application/json",
		)
	})

	t.Run("rejects unsupported media type", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"kanata"}`))
		req.Header.Set("Content-Type", "text/plain")

		var dst request
		_ = assertHTTPError(
			t,
			BindBody(req, &dst),
			http.StatusUnsupportedMediaType,
			CodeUnsupportedMediaType,
			"Content-Type must be application/json",
		)
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

	t.Run("rejects xml content type", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`<request><name>kanata</name></request>`))
		req.Header.Set("Content-Type", "application/xml")

		var dst request
		_ = assertHTTPError(
			t,
			BindBody(req, &dst),
			http.StatusUnsupportedMediaType,
			CodeUnsupportedMediaType,
			"Content-Type must be application/json",
		)
	})

	t.Run("rejects form content type", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`name=kanata`))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		var dst request
		_ = assertHTTPError(
			t,
			BindBody(req, &dst),
			http.StatusUnsupportedMediaType,
			CodeUnsupportedMediaType,
			"Content-Type must be application/json",
		)
	})

	t.Run("rejects multipart content type", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`--boundary`))
		req.Header.Set("Content-Type", "multipart/form-data; boundary=boundary")

		var dst request
		_ = assertHTTPError(
			t,
			BindBody(req, &dst),
			http.StatusUnsupportedMediaType,
			CodeUnsupportedMediaType,
			"Content-Type must be application/json",
		)
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

	t.Run("rejects unsupported content type before reading the full body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.ContentLength = -1
		req.Header.Set("Content-Type", "text/plain")
		req.Body = &byteThenReadErrorCloser{err: errors.New("read failed")}

		var dst request
		_ = assertHTTPError(
			t,
			BindBody(req, &dst),
			http.StatusUnsupportedMediaType,
			CodeUnsupportedMediaType,
			"Content-Type must be application/json",
		)
	})

	t.Run("rejects malformed content type before surfacing later body read errors", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.ContentLength = -1
		req.Header.Set("Content-Type", `application/json; charset="utf-8`)
		req.Body = &byteThenReadErrorCloser{err: errors.New("read failed")}

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

func TestBindBody_JSONContract(t *testing.T) {
	t.Run("top level null follows default decoder semantics", func(t *testing.T) {
		type request struct {
			Name string `json:"name"`
			Age  int    `json:"age"`
		}

		req := newJSONRequest(http.MethodPost, "/", `null`)
		dst := request{Name: "existing", Age: 17}
		if err := BindBody(req, &dst); err != nil {
			t.Fatalf("BindBody() error = %v", err)
		}
		if dst.Name != "existing" || dst.Age != 17 {
			t.Fatalf("dst = %#v, want existing values preserved for top-level null", dst)
		}
	})

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

	t.Run("type mismatch returns bad request and preserves prior decoded fields", func(t *testing.T) {
		type request struct {
			Name string `json:"name"`
			Age  int    `json:"age"`
		}

		req := newJSONRequest(http.MethodPost, "/", `{"name":"kanata","age":"oops"}`)
		dst := request{Name: "existing", Age: 7}
		_ = assertHTTPError(t, BindBody(req, &dst), http.StatusBadRequest, CodeInvalidJSON, "request body must be valid JSON")
		if dst.Name != "kanata" || dst.Age != 7 {
			t.Fatalf("dst = %#v, want decoded fields preserved and invalid field unchanged", dst)
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

	t.Run("rejects trailing garbage after valid json while preserving first value", func(t *testing.T) {
		type request struct {
			Name string `json:"name"`
			Age  int    `json:"age"`
		}

		req := newJSONRequest(http.MethodPost, "/", `{"name":"kanata","age":17}xxx`)

		dst := request{Name: "existing", Age: 7}
		_ = assertHTTPError(t, BindBody(req, &dst), http.StatusBadRequest, CodeInvalidJSON, "request body must be valid JSON")
		if dst.Name != "kanata" || dst.Age != 17 {
			t.Fatalf("dst = %#v, want first JSON value preserved before trailing-garbage error", dst)
		}
	})

	t.Run("rejects multiple top level json values while preserving first value", func(t *testing.T) {
		type request struct {
			Name string `json:"name"`
			Age  int    `json:"age"`
		}

		req := newJSONRequest(http.MethodPost, "/", `{"name":"kanata","age":17}{"name":"other"}`)

		dst := request{Name: "existing", Age: 7}
		_ = assertHTTPError(t, BindBody(req, &dst), http.StatusBadRequest, CodeInvalidJSON, "request body must be valid JSON")
		if dst.Name != "kanata" || dst.Age != 17 {
			t.Fatalf("dst = %#v, want first JSON value preserved before multi-value error", dst)
		}
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

func TestBindBody_RequestTooLarge(t *testing.T) {
	type request struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	oversizedName := strings.Repeat("a", int(defaultMaxBodyBytes))
	body := []byte(`{"name":"` + oversizedName + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("Content-Type", mimeApplicationJSON)
	req.ContentLength = int64(len(body))

	dst := request{Name: "existing", Age: 17}
	_ = assertHTTPError(t, BindBody(req, &dst), http.StatusRequestEntityTooLarge, CodeRequestTooLarge, "request body is too large")
	if dst.Name != "existing" || dst.Age != 17 {
		t.Fatalf("dst = %#v, want existing values preserved on oversized body", dst)
	}
}

func TestBindBody_HelperBranches(t *testing.T) {
	if got, err := bodyMediaType(nil); err != nil || got != "" {
		t.Fatalf("bodyMediaType(nil) = (%q, %v), want (empty, nil)", got, err)
	}

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"kanata"}`))
	req.Header.Set("Content-Type", " application/json ; charset=utf-8 ")
	if got, err := bodyMediaType(req); err != nil || got != mimeApplicationJSON {
		t.Fatalf("bodyMediaType() = (%q, %v), want (%q, nil)", got, err, mimeApplicationJSON)
	}

	badReq := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"kanata"}`))
	badReq.Header.Set("Content-Type", `application/json; charset="utf-8`)
	if _, err := bodyMediaType(badReq); err == nil {
		t.Fatal("bodyMediaType(malformed) error = nil, want parse error")
	}

	type payload struct {
		Name string `json:"name"`
	}

	if err := decodeJSONBody([]byte(`{"name":"kanata"}`), &payload{}, false); err != nil {
		t.Fatalf("decodeJSONBody() error = %v", err)
	}

	err := decodeJSONBody([]byte(`{"extra":1}`), &payload{}, false)
	_ = assertHTTPError(t, err, http.StatusBadRequest, CodeInvalidJSON, "request body must be valid JSON")

	invalidUnmarshalErr := &json.InvalidUnmarshalError{Type: reflect.TypeOf(payload{})}
	if got := mapJSONBodyDecodeError(invalidUnmarshalErr); got != invalidUnmarshalErr {
		t.Fatalf("mapJSONBodyDecodeError() = %v, want same error", got)
	}

	data, err := readBody(io.NopCloser(strings.NewReader("ok")), 0)
	if err != nil || string(data) != "ok" {
		t.Fatalf("readBody(default max) = (%q, %v), want (ok, nil)", data, err)
	}
	if data, err := readBody(nil, 10); err != nil || data != nil {
		t.Fatalf("readBody(nil) = (%v, %v), want (nil, nil)", data, err)
	}

	wantErr := errors.New("read failed")
	if _, err := readBody(failingReadCloser{err: wantErr}, 10); !errors.Is(err, wantErr) {
		t.Fatalf("readBody(failing) error = %v, want %v", err, wantErr)
	}

	if err := bindBodyDefault(nil, &payload{}, defaultBindConfig().body); err == nil || err.Error() != "bind: request must not be nil" {
		t.Fatalf("bindBodyDefault(nil) error = %v", err)
	}
	if err := bindBodyDefault(req, payload{}, defaultBindConfig().body); err == nil || err.Error() != "bind: destination must not be nil" {
		t.Fatalf("bindBodyDefault(non-pointer) error = %v", err)
	}

	readErrReq := httptest.NewRequest(http.MethodPost, "/", nil)
	readErrReq.ContentLength = 1
	readErrReq.Header.Set("Content-Type", mimeApplicationJSON)
	readErrReq.Body = failingReadCloser{err: wantErr}
	if err := bindBodyDefault(readErrReq, &payload{}, defaultBindConfig().body); !errors.Is(err, wantErr) {
		t.Fatalf("bindBodyDefault(read error) = %v, want %v", err, wantErr)
	}

	readErrAfterProbeReq := httptest.NewRequest(http.MethodPost, "/", nil)
	readErrAfterProbeReq.ContentLength = -1
	readErrAfterProbeReq.Header.Set("Content-Type", mimeApplicationJSON)
	readErrAfterProbeReq.Body = &byteThenReadErrorCloser{err: wantErr}
	if err := bindBodyDefault(readErrAfterProbeReq, &payload{}, defaultBindConfig().body); !errors.Is(err, wantErr) {
		t.Fatalf("bindBodyDefault(read error after probe) = %v, want %v", err, wantErr)
	}
}
