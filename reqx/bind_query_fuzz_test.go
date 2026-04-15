package reqx

import (
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/kanata996/hah/errx"
)

func FuzzBindQueryPublicContracts(f *testing.F) {
	f.Add("/items?page=1")
	f.Add("/items?name=kanata&tag=a&tag=b")
	f.Add("/items?when=2026-04-09")
	f.Add("/items?page=oops")

	f.Fuzz(func(t *testing.T, target string) {
		type request struct {
			Page int      `query:"page"`
			Name string   `query:"name"`
			Tags []string `query:"tag"`
		}

		req, err := http.NewRequest(http.MethodGet, strings.TrimSpace(target), nil)
		if err != nil {
			req, err = http.NewRequest(http.MethodGet, "/", nil)
			if err != nil {
				t.Fatalf("http.NewRequest() fallback error = %v", err)
			}
		}

		var got request
		gotErr := BindQuery(req, &got)

		var want request
		wantErr := bindQuery(req, &want)

		if !sameHTTPError(gotErr, wantErr) {
			t.Fatalf("BindQuery() error = %v, want %v", gotErr, wantErr)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("BindQuery() result = %#v, want %#v", got, want)
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
