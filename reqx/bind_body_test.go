package reqx

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

type bindBodyReadErrorCloser struct{ err error }

func (r bindBodyReadErrorCloser) Read([]byte) (int, error) { return 0, r.err }
func (r bindBodyReadErrorCloser) Close() error             { return nil }

type bindBodyPrefixThenErrorCloser struct {
	prefix   byte
	firstErr error
	nextErr  error
	used     bool
}

func (r *bindBodyPrefixThenErrorCloser) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if !r.used {
		r.used = true
		p[0] = r.prefix
		return 1, r.firstErr
	}
	if r.nextErr != nil {
		return 0, r.nextErr
	}
	return 0, io.EOF
}

func (r *bindBodyPrefixThenErrorCloser) Close() error { return nil }

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

func TestBindBody_ComposesWithRequireBodyOnSameRequest(t *testing.T) {
	type request struct {
		Name string `json:"name"`
	}

	t.Run("require then bind", func(t *testing.T) {
		req := newJSONRequest(http.MethodPost, "/", `{"name":"kanata"}`)

		if err := RequireBody(req); err != nil {
			t.Fatalf("RequireBody() error = %v", err)
		}

		var dst request
		if err := BindBody(req, &dst); err != nil {
			t.Fatalf("BindBody() error = %v", err)
		}
		if dst != (request{Name: "kanata"}) {
			t.Fatalf("dst = %#v, want bound body", dst)
		}
	})

	t.Run("bind then require", func(t *testing.T) {
		req := newJSONRequest(http.MethodPost, "/", `{"name":"kanata"}`)

		var dst request
		if err := BindBody(req, &dst); err != nil {
			t.Fatalf("BindBody() error = %v", err)
		}
		if err := RequireBody(req); err != nil {
			t.Fatalf("RequireBody() error = %v", err)
		}
		if dst != (request{Name: "kanata"}) {
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

	t.Run("too large while checking body presence returns request too large", func(t *testing.T) {
		type request struct {
			Name string `json:"name"`
		}

		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Header.Set("Content-Type", "application/json")
		req.Body = bindBodyReadErrorCloser{err: errRequestTooLarge}
		req.ContentLength = -1

		dst := request{Name: "existing"}
		err := BindBody(req, &dst)
		_ = assertHTTPStatusCode(t, err, http.StatusRequestEntityTooLarge, CodeRequestTooLarge)
		if dst != (request{Name: "existing"}) {
			t.Fatalf("dst = %#v, want unchanged", dst)
		}
	})

	t.Run("unsupported media type wins before size limit and preserves target", func(t *testing.T) {
		type request struct {
			Name string `json:"name"`
		}

		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat("a", int(defaultMaxBodyBytes)+1)))
		req.Header.Set("Content-Type", "text/plain")
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

	t.Run("body read failure after body presence check is an ordinary error", func(t *testing.T) {
		type request struct {
			Name string `json:"name"`
		}

		wantErr := errors.New("read failed after peek")
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Header.Set("Content-Type", "application/json")
		req.Body = &bindBodyPrefixThenErrorCloser{prefix: '{', nextErr: wantErr}
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

func TestBindBody_CachesBodyBytesForSameRequest(t *testing.T) {
	type request struct {
		Name string `json:"name"`
	}

	req := newJSONRequest(http.MethodPost, "/", `{"name":"kanata"}`)
	var first request
	if err := BindBody(req, &first); err != nil {
		t.Fatalf("first BindBody() error = %v", err)
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("ReadAll(req.Body) error = %v", err)
	}
	if string(body) != `{"name":"kanata"}` {
		t.Fatalf("body = %q, want original request body replayed", string(body))
	}

	var second request
	if err := BindBody(req, &second); err != nil {
		t.Fatalf("second BindBody() error = %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("second bind = %#v, want %#v", second, first)
	}
}

func TestRequestHasBody_InternalBranches(t *testing.T) {
	t.Run("nil request or body is treated as no body", func(t *testing.T) {
		hasBody, err := requestHasBody(nil)
		if err != nil || hasBody {
			t.Fatalf("requestHasBody(nil) = (%v, %v), want (false, nil)", hasBody, err)
		}

		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Body = nil

		hasBody, err = requestHasBody(req)
		if err != nil || hasBody {
			t.Fatalf("requestHasBody(req with nil body) = (%v, %v), want (false, nil)", hasBody, err)
		}
	})

	t.Run("presence cache short circuits body reads", func(t *testing.T) {
		wantErr := errors.New("cached failure")
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Body = bindBodyReadErrorCloser{err: errors.New("body should not be read")}
		req = req.WithContext(context.WithValue(req.Context(), requestBodyPresenceKey{}, requestBodyPresenceState{
			has: true,
			err: wantErr,
		}))

		hasBody, err := requestHasBody(req)
		if !hasBody || !errors.Is(err, wantErr) {
			t.Fatalf("requestHasBody(cached) = (%v, %v), want (true, %v)", hasBody, err, wantErr)
		}
	})

	t.Run("partial read failure replays consumed prefix", func(t *testing.T) {
		wantErr := errors.New("peek failed")
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Body = &bindBodyPrefixThenErrorCloser{prefix: '{', firstErr: wantErr}
		req.ContentLength = -1

		hasBody, err := requestHasBody(req)
		if hasBody || !errors.Is(err, wantErr) {
			t.Fatalf("requestHasBody() = (%v, %v), want (false, %v)", hasBody, err, wantErr)
		}

		body, readErr := io.ReadAll(req.Body)
		if readErr != nil {
			t.Fatalf("ReadAll(req.Body) error = %v", readErr)
		}
		if string(body) != "{" {
			t.Fatalf("body = %q, want replayed prefix", string(body))
		}
	})
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
}
