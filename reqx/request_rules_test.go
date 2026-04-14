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
	req := newJSONRequest(http.MethodPost, "/", `{"name":"kanata"}`)
	if err := RequireBody(req); err != nil {
		t.Fatalf("RequireBody(non-empty) error = %v", err)
	}

	emptyReq := newJSONRequest(http.MethodPost, "/", "")
	emptyReq.ContentLength = 0

	violation := assertSingleViolation(t, RequireBody(emptyReq))
	if violation.Field != "body" || violation.In != ViolationInBody || violation.Code != ViolationCodeRequired || violation.Detail != "is required" {
		t.Fatalf("violation = %#v", violation)
	}

	unknownLengthReq := newJSONRequest(http.MethodPost, "/", "")
	unknownLengthReq.ContentLength = -1

	violation = assertSingleViolation(t, RequireBody(unknownLengthReq))
	if violation.Field != "body" || violation.In != ViolationInBody || violation.Code != ViolationCodeRequired || violation.Detail != "is required" {
		t.Fatalf("violation = %#v", violation)
	}
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
}
