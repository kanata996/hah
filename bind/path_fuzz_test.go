package bind

// 测试清单：
// - 标记说明：[✓] 已核对且已有真实覆盖；[x] 尚未完成，不得作为验收依据。
// - [✓] Fuzz 校验 Bind 的公开结果与单源 API 按默认顺序组合后的结果一致。
// - [✓] Fuzz 校验 Bind 的错误结果维持稳定的 HTTPError 契约。

import (
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/kanata996/hah/errx"
)

func FuzzBindPublicContracts(f *testing.F) {
	f.Add(http.MethodGet, "/items?page=1", "", "application/json")
	f.Add(http.MethodPost, "/items", `{"name":"kanata"}`, mimeApplicationJSON)
	f.Add(http.MethodPost, "/items", "", "text/plain")
	f.Add(http.MethodPost, "/items", " \n\t ", mimeApplicationJSON)

	f.Fuzz(func(t *testing.T, method, target, body, contentType string) {
		type request struct {
			ID   string `param:"id" query:"id" json:"id"`
			Page int    `query:"page"`
			Name string `json:"name" header:"x-name"`
		}

		safeMethod := strings.TrimSpace(method)
		if !isValidHTTPMethod(safeMethod) {
			safeMethod = http.MethodGet
		}

		newRequest := func() *http.Request {
			req, err := http.NewRequest(safeMethod, target, strings.NewReader(body))
			if err != nil {
				req, err = http.NewRequest(safeMethod, "/", strings.NewReader(body))
				if err != nil {
					t.Fatalf("http.NewRequest() fallback error = %v", err)
				}
			}
			if contentType != "" {
				req.Header.Set("Content-Type", contentType)
			}
			req.Header.Set("X-Name", "header-name")
			req.SetPathValue("id", "route-id")
			req.Pattern = "/items/{id}"
			return req
		}

		var got request
		gotErr := Bind(newRequest(), &got)

		var want request
		wantErr := BindPathValues(newRequest(), &want)
		if wantErr == nil {
			normalizedMethod := strings.ToUpper(strings.TrimSpace(safeMethod))
			if normalizedMethod == http.MethodGet || normalizedMethod == http.MethodDelete || normalizedMethod == http.MethodHead {
				wantErr = BindQueryParams(newRequest(), &want)
			}
		}
		if wantErr == nil {
			wantErr = BindBody(newRequest(), &want)
		}

		if !sameHTTPError(gotErr, wantErr) {
			t.Fatalf("Bind() error = %v, want %v", gotErr, wantErr)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("Bind() result = %#v, want %#v", got, want)
		}
	})
}

func sameHTTPError(got, want error) bool {
	if got == nil || want == nil {
		return got == want
	}

	var gotHTTP, wantHTTP *errx.HTTPError
	if errors.As(got, &gotHTTP) && errors.As(want, &wantHTTP) && gotHTTP != nil && wantHTTP != nil {
		return gotHTTP.Status() == wantHTTP.Status() &&
			gotHTTP.Code() == wantHTTP.Code() &&
			gotHTTP.Detail() == wantHTTP.Detail()
	}

	return got.Error() == want.Error()
}

func isValidHTTPMethod(method string) bool {
	method = strings.TrimSpace(method)
	if method == "" {
		return false
	}

	for _, r := range method {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case strings.ContainsRune("!#$%&'*+-.^_`|~", r):
		default:
			return false
		}
	}

	return true
}
