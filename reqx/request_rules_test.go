package reqx

import (
	"errors"
	"net/http"
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
	t.Run("non-empty body passes and can still be bound", func(t *testing.T) {
		type request struct {
			Name string `json:"name"`
			Age  int    `json:"age"`
		}

		req := newJSONRequest(http.MethodPost, "/", `{"name":"kanata"}`)
		if err := RequireBody(req); err != nil {
			t.Fatalf("RequireBody(non-empty) error = %v", err)
		}

		dst := request{Name: "existing", Age: 17}
		if err := BindBody(req, &dst); err != nil {
			t.Fatalf("BindBody() after RequireBody() error = %v", err)
		}
		if dst != (request{Name: "kanata"}) {
			t.Fatalf("dst = %#v, want zero-based bind result after RequireBody", dst)
		}
	})

	t.Run("whitespace body counts as present for RequireBody", func(t *testing.T) {
		req := newJSONRequest(http.MethodPost, "/", " \n\t ")

		if err := RequireBody(req); err != nil {
			t.Fatalf("RequireBody(whitespace) error = %v, want nil", err)
		}
	})

	t.Run("null body counts as present for RequireBody while BindBody rejects it", func(t *testing.T) {
		type request struct {
			Name string `json:"name"`
		}

		req := newJSONRequest(http.MethodPost, "/", `null`)

		if err := RequireBody(req); err != nil {
			t.Fatalf("RequireBody(null) error = %v, want nil", err)
		}

		err := BindBody(req, &request{})
		_ = assertHTTPStatusCode(t, err, http.StatusBadRequest, CodeInvalidJSON)
	})

	t.Run("content length zero body is required violation", func(t *testing.T) {
		req := newJSONRequest(http.MethodPost, "/", "")
		req.ContentLength = 0

		violation := assertSingleViolation(t, RequireBody(req))
		if violation.Field != "body" || violation.In != errx.InBody || violation.Code != errx.CodeRequired || violation.Detail != "is required" {
			t.Fatalf("violation = %#v", violation)
		}
	})

	t.Run("unknown length empty body is required violation", func(t *testing.T) {
		req := newJSONRequest(http.MethodPost, "/", "")
		req.ContentLength = -1

		violation := assertSingleViolation(t, RequireBody(req))
		if violation.Field != "body" || violation.In != errx.InBody || violation.Code != errx.CodeRequired || violation.Detail != "is required" {
			t.Fatalf("violation = %#v", violation)
		}
	})

	t.Run("nil body is required violation", func(t *testing.T) {
		req := newJSONRequest(http.MethodPost, "/", "")
		req.Body = nil
		req.ContentLength = 0

		violation := assertSingleViolation(t, RequireBody(req))
		if violation.Field != "body" || violation.In != errx.InBody || violation.Code != errx.CodeRequired || violation.Detail != "is required" {
			t.Fatalf("violation = %#v", violation)
		}
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

func TestRequireBody_EmptyBodyProbePreservesOriginalBody(t *testing.T) {
	testCases := []struct {
		name          string
		contentLength int64
	}{
		{name: "content length zero", contentLength: 0},
		{name: "unknown length", contentLength: -1},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := newJSONRequest(http.MethodPost, "/", "")
			req.ContentLength = tc.contentLength

			body := &eofReadCloser{}
			req.Body = body

			violation := assertSingleViolation(t, RequireBody(req))
			if violation.Field != "body" || violation.In != errx.InBody || violation.Code != errx.CodeRequired || violation.Detail != "is required" {
				t.Fatalf("violation = %#v", violation)
			}
			if req.Body != body {
				t.Fatalf("request body = %T, want original empty body preserved", req.Body)
			}
			if body.reads != 1 {
				t.Fatalf("body read count = %d, want 1 probe read", body.reads)
			}
		})
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
