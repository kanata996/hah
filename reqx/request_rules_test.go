package reqx

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/kanata996/hah/errx"
)

type errorReadCloser struct {
	err error
}

func (r errorReadCloser) Read(_ []byte) (int, error) {
	return 0, r.err
}

func (r errorReadCloser) Close() error {
	return nil
}

func TestRequireBody(t *testing.T) {
	t.Run("non empty body passes and can still be bound", func(t *testing.T) {
		type request struct {
			Name string `json:"name"`
		}

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

	t.Run("bound body still counts as present for RequireBody", func(t *testing.T) {
		type request struct {
			Name string `json:"name"`
		}

		req := newJSONRequest(http.MethodPost, "/", `{"name":"kanata"}`)

		var dst request
		if err := BindBody(req, &dst); err != nil {
			t.Fatalf("BindBody() error = %v", err)
		}
		if err := RequireBody(req); err != nil {
			t.Fatalf("RequireBody() error = %v", err)
		}
	})

	t.Run("whitespace and null bodies count as present", func(t *testing.T) {
		for _, body := range []string{" \n\t ", `null`} {
			req := newJSONRequest(http.MethodPost, "/", body)
			if err := RequireBody(req); err != nil {
				t.Fatalf("RequireBody(%q) error = %v, want nil", body, err)
			}
		}
	})

	t.Run("zero byte bodies are required violations", func(t *testing.T) {
		testCases := []*http.Request{
			func() *http.Request {
				req := newJSONRequest(http.MethodPost, "/", "")
				req.ContentLength = 0
				return req
			}(),
			func() *http.Request {
				req := newJSONRequest(http.MethodPost, "/", "")
				req.ContentLength = -1
				return req
			}(),
			func() *http.Request {
				req := newJSONRequest(http.MethodPost, "/", "")
				req.Body = nil
				req.ContentLength = 0
				return req
			}(),
		}

		for _, req := range testCases {
			violation := assertSingleViolation(t, RequireBody(req))
			if violation.Field != "body" || violation.In != errx.InBody || violation.Code != errx.CodeRequired || violation.Detail != "is required" {
				t.Fatalf("violation = %#v", violation)
			}
		}
	})

	t.Run("oversized body returns request too large", func(t *testing.T) {
		payload := `{"name":"` + strings.Repeat("a", int(defaultMaxBodyBytes)) + `"}`
		err := RequireBody(newJSONRequest(http.MethodPost, "/", payload))
		_ = assertHTTPStatusCode(t, err, http.StatusRequestEntityTooLarge, CodeRequestTooLarge)
	})
}

func TestRequireBodyNilRequest(t *testing.T) {
	if err := RequireBody(nil); err == nil || err.Error() != "reqx: request must not be nil" {
		t.Fatalf("RequireBody(nil) error = %v", err)
	}
}

func TestRequireBodyReadError(t *testing.T) {
	wantErr := errors.New("read failed")
	req := newJSONRequest(http.MethodPost, "/", "")
	req.Body = errorReadCloser{err: wantErr}
	req.ContentLength = -1

	if err := RequireBody(req); !errors.Is(err, wantErr) {
		t.Fatalf("RequireBody(read error) = %v, want %v", err, wantErr)
	}
}

func TestInvalidRequest_UsesViolationEnvelope(t *testing.T) {
	testCases := []struct {
		name string
		in   errx.Violation
		want errx.Violation
	}{
		{
			name: "default invalid",
			in:   errx.Violation{Field: "name"},
			want: errx.Violation{Field: "name", Code: errx.CodeInvalid, Detail: "is invalid"},
		},
		{
			name: "required",
			in:   errx.Violation{Field: "body", In: errx.InBody, Code: errx.CodeRequired},
			want: errx.Violation{Field: "body", In: errx.InBody, Code: errx.CodeRequired, Detail: "is required"},
		},
		{
			name: "unknown",
			in:   errx.Violation{Field: "extra", In: errx.InQuery, Code: errx.CodeUnknown},
			want: errx.Violation{Field: "extra", In: errx.InQuery, Code: errx.CodeUnknown, Detail: "unknown field"},
		},
		{
			name: "type",
			in:   errx.Violation{Field: "limit", In: errx.InBody, Code: errx.CodeType},
			want: errx.Violation{Field: "limit", In: errx.InBody, Code: errx.CodeType, Detail: "has invalid type"},
		},
		{
			name: "multiple",
			in:   errx.Violation{Field: "X-Trace-Id", In: errx.InHeader, Code: errx.CodeMultiple},
			want: errx.Violation{Field: "X-Trace-Id", In: errx.InHeader, Code: errx.CodeMultiple, Detail: "must appear only once"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			violation := assertSingleViolation(t, InvalidRequest(tc.in))
			if violation != tc.want {
				t.Fatalf("violation = %#v, want %#v", violation, tc.want)
			}
		})
	}

	t.Run("multiple violations are preserved in order", func(t *testing.T) {
		got := assertViolations(t, InvalidRequest(
			errx.Violation{Field: "page", In: errx.InQuery},
			errx.Violation{Field: "body", In: errx.InBody, Code: errx.CodeRequired},
		))

		want := []errx.Violation{
			{Field: "page", In: errx.InQuery, Code: errx.CodeInvalid, Detail: "is invalid"},
			{Field: "body", In: errx.InBody, Code: errx.CodeRequired, Detail: "is required"},
		}
		if len(got) != len(want) {
			t.Fatalf("violations len = %d, want %d", len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("violations[%d] = %#v, want %#v", i, got[i], want[i])
			}
		}
	})
}
