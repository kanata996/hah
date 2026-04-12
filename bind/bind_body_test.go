package bind

// 测试清单：
// - 标记说明：[✓] 已核对且已有真实覆盖；[x] 尚未完成，不得作为验收依据。
// - [✓] BindBody 的 Content-Type、空 body、未知字段与非法 JSON 契约。
// - [✓] BindBody 支持标准 decoder 目标，包括 struct、slice、map。
// - [✓] BindBody 公开入口拒绝无效 destination，包括非指针和 typed nil pointer。
// - [✓] BindBody 在 body 大小达到上限、超出上限和 unknown-length 超限时维持稳定契约。
// - [✓] body 相关内部辅助有最小补充覆盖，包括 media type、读 body 和错误映射。
// - [✓] BindBody 在 req.Body 为 nil 时保持 no-op。

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

		var dst request
		if err := BindBody(req, &dst); !errors.Is(err, wantErr) {
			t.Fatalf("BindBody() error = %v, want %v", err, wantErr)
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

func TestBindBody_PublicEntryPointRejectsInvalidDestinations(t *testing.T) {
	type request struct {
		Name string `json:"name"`
	}

	req := newJSONRequest(http.MethodPost, "/", `{"name":"kanata"}`)

	t.Run("rejects non pointer destination", func(t *testing.T) {
		if err := BindBody(req, request{}); err == nil || err.Error() != "bind: destination must not be nil" {
			t.Fatalf("BindBody(non-pointer) error = %v", err)
		}
	})

	t.Run("rejects typed nil pointer destination", func(t *testing.T) {
		var dst *request
		if err := BindBody(req, dst); err == nil || err.Error() != "bind: destination must not be nil" {
			t.Fatalf("BindBody(typed nil pointer) error = %v", err)
		}
	})
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
		var dst request
		_ = assertHTTPError(t, BindBody(req, &dst), http.StatusBadRequest, CodeInvalidJSON, "request body must be valid JSON")
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
