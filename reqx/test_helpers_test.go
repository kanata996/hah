package reqx

// 测试清单：
// - 标记说明：[✓] 已核对且已有真实覆盖；[x] 尚未完成，不得作为验收依据。
// - [✓] 测试辅助文件：提供 JSON 请求构造与 HTTP/violation 断言辅助，不声明独立业务覆盖。

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/kanata996/hah/errx"
)

func newJSONRequest(method, target, body string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func assertHTTPError(t *testing.T, err error, wantStatus int, wantCode, wantDetail string) *errx.HTTPError {
	t.Helper()

	httpErr, ok := err.(*errx.HTTPError)
	if !ok {
		t.Fatalf("error type = %T, want *errx.HTTPError", err)
	}
	if got := httpErr.Status(); got != wantStatus {
		t.Fatalf("status = %d, want %d", got, wantStatus)
	}
	if got := httpErr.Code(); got != wantCode {
		t.Fatalf("code = %q, want %q", got, wantCode)
	}
	if got := httpErr.Detail(); got != wantDetail {
		t.Fatalf("detail = %q, want %q", got, wantDetail)
	}
	return httpErr
}

func assertViolations(t *testing.T, err error) []Violation {
	t.Helper()

	httpErr := assertHTTPError(
		t,
		err,
		http.StatusUnprocessableEntity,
		CodeInvalidRequest,
		"request contains invalid fields",
	)

	details := httpErr.Errors()
	violations := make([]Violation, 0, len(details))
	for i, detail := range details {
		violation, ok := detail.(Violation)
		if !ok {
			t.Fatalf("detail[%d] type = %T, want reqx.Violation", i, detail)
		}
		violations = append(violations, violation)
	}
	return violations
}

func assertSingleViolation(t *testing.T, err error) Violation {
	t.Helper()

	violations := assertViolations(t, err)
	if len(violations) != 1 {
		t.Fatalf("details len = %d, want 1", len(violations))
	}
	return violations[0]
}

func assertSameHTTPError(t *testing.T, gotErr, wantErr error) *errx.HTTPError {
	t.Helper()

	got, ok := gotErr.(*errx.HTTPError)
	if !ok {
		t.Fatalf("got error type = %T, want *errx.HTTPError", gotErr)
	}
	want, ok := wantErr.(*errx.HTTPError)
	if !ok {
		t.Fatalf("want error type = %T, want *errx.HTTPError", wantErr)
	}

	if got.Status() != want.Status() || got.Code() != want.Code() || got.Detail() != want.Detail() {
		t.Fatalf(
			"got error = (%d, %q, %q), want (%d, %q, %q)",
			got.Status(),
			got.Code(),
			got.Detail(),
			want.Status(),
			want.Code(),
			want.Detail(),
		)
	}
	if !reflect.DeepEqual(got.Errors(), want.Errors()) {
		t.Fatalf("got error details = %#v, want %#v", got.Errors(), want.Errors())
	}

	return got
}
