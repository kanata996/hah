package reqx

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type bindBodyJSONValue struct {
	Value string
}

func (v *bindBodyJSONValue) UnmarshalJSON(data []byte) error {
	var decoded string
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	v.Value = "json:" + decoded
	return nil
}

type bindBodyRootJSONPointerTarget struct {
	Name string `json:"name"`
}

func (*bindBodyRootJSONPointerTarget) UnmarshalJSON([]byte) error { return nil }

type bindBodyRootJSONValueTarget struct {
	Name string `json:"name"`
}

func (bindBodyRootJSONValueTarget) UnmarshalJSON([]byte) error { return nil }

func TestBindBody_BasicContracts(t *testing.T) {
	t.Run("zero byte body is noop and does not require json content type", func(t *testing.T) {
		type request struct {
			Name string `json:"name"`
		}

		req := httptest.NewRequest(http.MethodPost, "/", http.NoBody)
		req.Header.Set("Content-Type", "text/plain")
		dst := request{Name: "existing"}

		if err := BindBody(req, &dst); err != nil {
			t.Fatalf("BindBody() error = %v", err)
		}
		if dst != (request{Name: "existing"}) {
			t.Fatalf("dst = %#v, want unchanged", dst)
		}
	})

	t.Run("valid json object decodes into zero value temp and commits atomically", func(t *testing.T) {
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
			t.Fatalf("dst = %#v, want zero-based replacement", dst)
		}
	})

	t.Run("application json with charset parameter is accepted", func(t *testing.T) {
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
}

func TestBindBody_FollowsEncodingJSONFieldSemantics(t *testing.T) {
	t.Run("raw message and custom decoders are allowed", func(t *testing.T) {
		type request struct {
			Raw   json.RawMessage   `json:"raw"`
			Value bindBodyJSONValue `json:"value"`
		}

		var dst request
		if err := BindBody(newJSONRequest(http.MethodPost, "/", `{"raw":{"x":1},"value":"ok"}`), &dst); err != nil {
			t.Fatalf("BindBody() error = %v", err)
		}
		if string(dst.Raw) != `{"x":1}` {
			t.Fatalf("raw = %q, want {\"x\":1}", string(dst.Raw))
		}
		if dst.Value.Value != "json:ok" {
			t.Fatalf("value = %#v, want custom decoder result", dst.Value)
		}
	})

	t.Run("anonymous unexported embedded fields participate like encoding json", func(t *testing.T) {
		type hidden struct {
			Raw json.RawMessage `json:"raw"`
		}
		type request struct {
			hidden
		}

		var dst request
		if err := BindBody(newJSONRequest(http.MethodPost, "/", `{"raw":{"x":1}}`), &dst); err != nil {
			t.Fatalf("BindBody() error = %v", err)
		}
		if string(dst.Raw) != `{"x":1}` {
			t.Fatalf("raw = %q, want {\"x\":1}", string(dst.Raw))
		}
	})

	t.Run("shadowed embedded fields follow dominant field semantics", func(t *testing.T) {
		type Embedded struct {
			Raw json.RawMessage `json:"raw"`
		}
		type request struct {
			Embedded
			Raw string `json:"raw"`
		}

		var dst request
		if err := BindBody(newJSONRequest(http.MethodPost, "/", `{"raw":"ok"}`), &dst); err != nil {
			t.Fatalf("BindBody() error = %v", err)
		}
		if dst.Raw != "ok" {
			t.Fatalf("dst = %#v, want dominant string field bound", dst)
		}
		if dst.Embedded.Raw != nil {
			t.Fatalf("embedded raw = %q, want nil", string(dst.Embedded.Raw))
		}
	})

	t.Run("duplicate keys follow standard library last wins behavior", func(t *testing.T) {
		type request struct {
			Name string `json:"name"`
		}

		var dst request
		if err := BindBody(newJSONRequest(http.MethodPost, "/", `{"name":"first","name":"second"}`), &dst); err != nil {
			t.Fatalf("BindBody() error = %v", err)
		}
		if dst.Name != "second" {
			t.Fatalf("dst = %#v, want last value to win", dst)
		}
	})
}

func TestBindBody_ClientErrorsPreserveTarget(t *testing.T) {
	type request struct {
		Name string `json:"name"`
		Age  int8   `json:"age"`
	}

	testCases := []struct {
		name        string
		body        string
		contentType string
		wantStatus  int
		wantCode    string
	}{
		{name: "unsupported media type", body: `{"name":"kanata"}`, contentType: "text/plain", wantStatus: http.StatusUnsupportedMediaType, wantCode: CodeUnsupportedMediaType},
		{name: "whitespace only body", body: " \n\t ", contentType: "application/json", wantStatus: http.StatusBadRequest, wantCode: CodeInvalidJSON},
		{name: "top level null", body: `null`, contentType: "application/json", wantStatus: http.StatusBadRequest, wantCode: CodeInvalidJSON},
		{name: "top level array", body: `[]`, contentType: "application/json", wantStatus: http.StatusBadRequest, wantCode: CodeInvalidJSON},
		{name: "top level string", body: `"x"`, contentType: "application/json", wantStatus: http.StatusBadRequest, wantCode: CodeInvalidJSON},
		{name: "top level number", body: `123`, contentType: "application/json", wantStatus: http.StatusBadRequest, wantCode: CodeInvalidJSON},
		{name: "top level boolean", body: `true`, contentType: "application/json", wantStatus: http.StatusBadRequest, wantCode: CodeInvalidJSON},
		{name: "truncated json", body: `{"name":"kanata"`, contentType: "application/json", wantStatus: http.StatusBadRequest, wantCode: CodeInvalidJSON},
		{name: "trailing data", body: `{"name":"kanata"} true`, contentType: "application/json", wantStatus: http.StatusBadRequest, wantCode: CodeInvalidJSON},
		{name: "unknown field", body: `{"extra":1}`, contentType: "application/json", wantStatus: http.StatusBadRequest, wantCode: CodeInvalidJSON},
		{name: "type mismatch", body: `{"age":"x"}`, contentType: "application/json", wantStatus: http.StatusBadRequest, wantCode: CodeInvalidJSON},
		{name: "overflow", body: `{"age":128}`, contentType: "application/json", wantStatus: http.StatusBadRequest, wantCode: CodeInvalidJSON},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dst := request{Name: "existing", Age: 7}
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))
			if tc.contentType != "" {
				req.Header.Set("Content-Type", tc.contentType)
			}

			err := BindBody(req, &dst)
			_ = assertHTTPStatusCode(t, err, tc.wantStatus, tc.wantCode)
			if dst != (request{Name: "existing", Age: 7}) {
				t.Fatalf("dst = %#v, want unchanged", dst)
			}
		})
	}
}

func TestBindBody_BoundariesAndUsageErrors(t *testing.T) {
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

	t.Run("rejects root dto implementing unmarshal json", func(t *testing.T) {
		assertNotHTTPError(t, BindBody(newJSONRequest(http.MethodPost, "/", `{"extra":1}`), &bindBodyRootJSONPointerTarget{}))
		assertNotHTTPError(t, BindBody(newJSONRequest(http.MethodPost, "/", `{"extra":1}`), &bindBodyRootJSONValueTarget{}))
	})

	t.Run("root dto unmarshal json usage error wins before body read", func(t *testing.T) {
		wantErr := errors.New("read failed")
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Header.Set("Content-Type", "application/json")
		req.Body = bindBodyReadErrorCloser{err: wantErr}
		req.ContentLength = -1

		err := BindBody(req, &bindBodyRootJSONPointerTarget{})
		assertNotHTTPError(t, err)
		if errors.Is(err, wantErr) {
			t.Fatalf("BindBody() error = %v, want usage error before body inspection", err)
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

	t.Run("oversized body wins before media type validation and preserves target", func(t *testing.T) {
		type request struct {
			Name string `json:"name"`
		}

		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat("a", int(defaultMaxBodyBytes)+1)))
		req.Header.Set("Content-Type", "text/plain")
		dst := request{Name: "existing"}

		err := BindBody(req, &dst)
		_ = assertHTTPStatusCode(t, err, http.StatusRequestEntityTooLarge, CodeRequestTooLarge)
		if dst != (request{Name: "existing"}) {
			t.Fatalf("dst = %#v, want unchanged", dst)
		}
	})

	t.Run("duplicate content type values are rejected and preserve target", func(t *testing.T) {
		type request struct {
			Name string `json:"name"`
		}

		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"kanata"}`))
		req.Header["Content-Type"] = []string{"application/json", "text/plain"}
		dst := request{Name: "existing"}

		err := BindBody(req, &dst)
		_ = assertHTTPStatusCode(t, err, http.StatusUnsupportedMediaType, CodeUnsupportedMediaType)
		if dst != (request{Name: "existing"}) {
			t.Fatalf("dst = %#v, want unchanged", dst)
		}
	})

	t.Run("body read failures are ordinary errors", func(t *testing.T) {
		type request struct {
			Name string `json:"name"`
		}

		wantErr := errors.New("read failed")
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Header.Set("Content-Type", "application/json")
		req.Body = bindBodyReadErrorCloser{err: wantErr}
		req.ContentLength = -1

		dst := request{Name: "existing"}
		err := BindBody(req, &dst)
		if !errors.Is(err, wantErr) {
			t.Fatalf("BindBody() error = %v, want %v", err, wantErr)
		}
		if dst != (request{Name: "existing"}) {
			t.Fatalf("dst = %#v, want unchanged", dst)
		}
	})

	t.Run("body readers that make no progress fail with ordinary error", func(t *testing.T) {
		type request struct {
			Name string `json:"name"`
		}
		const noProgressLimit = 100

		wantErr := errors.New("reader progressed too late")
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Header.Set("Content-Type", "application/json")
		req.Body = &bindBodyNoProgressThenErrorCloser{
			remaining: noProgressLimit,
			err:       wantErr,
		}
		req.ContentLength = -1

		dst := request{Name: "existing"}
		err := BindBody(req, &dst)
		if !errors.Is(err, io.ErrNoProgress) {
			t.Fatalf("BindBody() error = %v, want %v", err, io.ErrNoProgress)
		}
		if errors.Is(err, wantErr) {
			t.Fatalf("BindBody() error = %v, want no-progress error before wrapped reader error", err)
		}
		if dst != (request{Name: "existing"}) {
			t.Fatalf("dst = %#v, want unchanged", dst)
		}
	})
}

func TestBindBody_ExactOneMiBStillBinds(t *testing.T) {
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
}

func TestBodyMediaType_InternalBranches(t *testing.T) {
	t.Run("empty content type is allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", nil)

		mediaType, err := bodyMediaType(req)
		if err != nil || mediaType != "" {
			t.Fatalf("bodyMediaType() = (%q, %v), want (\"\", nil)", mediaType, err)
		}
	})

	t.Run("invalid content type returns parse error", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Header.Set("Content-Type", `application/json; charset="utf-8`)

		_, err := bodyMediaType(req)
		if err == nil {
			t.Fatal("bodyMediaType() error = nil, want parse error")
		}
	})

	t.Run("duplicate content type values return error", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Header["Content-Type"] = []string{"application/json", "text/plain"}

		_, err := bodyMediaType(req)
		if err == nil {
			t.Fatal("bodyMediaType() error = nil, want duplicate content type error")
		}
	})
}

func TestReadRequestBody_InternalBranches(t *testing.T) {
	t.Run("nil request body returns nil body without error", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Body = nil

		body, err := readRequestBody(req)
		if err != nil {
			t.Fatalf("readRequestBody() error = %v, want nil", err)
		}
		if body != nil {
			t.Fatalf("body = %v, want nil", body)
		}
	})
}
