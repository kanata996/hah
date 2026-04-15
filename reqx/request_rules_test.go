package reqx

import (
	"errors"
	"net/http"
	"testing"
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
		if dst.Name != "kanata" || dst.Age != 17 {
			t.Fatalf("dst = %#v, want bound body fields with omitted fields preserved", dst)
		}
	})

	t.Run("content length zero body is required violation", func(t *testing.T) {
		req := newJSONRequest(http.MethodPost, "/", "")
		req.ContentLength = 0

		violation := assertSingleViolation(t, RequireBody(req))
		if violation.Field != "body" || violation.In != ViolationInBody || violation.Code != ViolationCodeRequired || violation.Detail != "is required" {
			t.Fatalf("violation = %#v", violation)
		}
	})

	t.Run("unknown length empty body is required violation", func(t *testing.T) {
		req := newJSONRequest(http.MethodPost, "/", "")
		req.ContentLength = -1

		violation := assertSingleViolation(t, RequireBody(req))
		if violation.Field != "body" || violation.In != ViolationInBody || violation.Code != ViolationCodeRequired || violation.Detail != "is required" {
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

func TestInvalidRequest_UsesViolationEnvelope(t *testing.T) {
	testCases := []struct {
		name string
		in   Violation
		want Violation
	}{
		{
			name: "default invalid",
			in:   Violation{Field: "name"},
			want: Violation{Field: "name", Code: ViolationCodeInvalid, Detail: "is invalid"},
		},
		{
			name: "required",
			in:   Violation{Field: "body", In: ViolationInBody, Code: ViolationCodeRequired},
			want: Violation{Field: "body", In: ViolationInBody, Code: ViolationCodeRequired, Detail: "is required"},
		},
		{
			name: "unknown",
			in:   Violation{Field: "extra", In: ViolationInQuery, Code: ViolationCodeUnknown},
			want: Violation{Field: "extra", In: ViolationInQuery, Code: ViolationCodeUnknown, Detail: "unknown field"},
		},
		{
			name: "type",
			in:   Violation{Field: "limit", In: ViolationInBody, Code: ViolationCodeType},
			want: Violation{Field: "limit", In: ViolationInBody, Code: ViolationCodeType, Detail: "has invalid type"},
		},
		{
			name: "multiple",
			in:   Violation{Field: "X-Trace-Id", In: ViolationInHeader, Code: ViolationCodeMultiple},
			want: Violation{Field: "X-Trace-Id", In: ViolationInHeader, Code: ViolationCodeMultiple, Detail: "must not be repeated"},
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
			Violation{Field: "page", In: ViolationInQuery},
			Violation{Field: "body", In: ViolationInBody, Code: ViolationCodeRequired},
		))

		want := []Violation{
			{Field: "page", In: ViolationInQuery, Code: ViolationCodeInvalid, Detail: "is invalid"},
			{Field: "body", In: ViolationInBody, Code: ViolationCodeRequired, Detail: "is required"},
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
