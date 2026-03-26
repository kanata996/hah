package reqx

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/url"
	"testing"
)

func FuzzDecodeJSON(f *testing.F) {
	f.Add([]byte(`{"name":"alice","age":20}`), "application/json", false, false, uint16(1024))
	f.Add([]byte(`{"name":`), "application/json", false, false, uint16(1024))
	f.Add([]byte(`{"name":"alice","extra":true}`), "application/json", false, false, uint16(1024))
	f.Add([]byte(``), "application/json", false, false, uint16(1024))
	f.Add([]byte(``), "application/json", false, true, uint16(1024))
	f.Add([]byte(`{"name":"alice"}`), "text/plain", false, false, uint16(1024))

	f.Fuzz(func(t *testing.T, body []byte, contentType string, allowUnknown, allowEmpty bool, maxBody uint16) {
		if len(body) > 4<<10 || len(contentType) > 256 {
			t.Skip()
		}

		req := &http.Request{
			Method: http.MethodPost,
			Header: make(http.Header),
			Body:   io.NopCloser(bytes.NewReader(body)),
		}
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}

		var dst struct {
			Name   string   `json:"name"`
			Age    int      `json:"age"`
			Tags   []string `json:"tags"`
			Active bool     `json:"active"`
		}

		opts := []DecodeOption{WithMaxBodyBytes(int64(maxBody) + 1)}
		if allowUnknown {
			opts = append(opts, AllowUnknownFields())
		}
		if allowEmpty {
			opts = append(opts, AllowEmptyBody())
		}

		err := DecodeJSON(req, &dst, opts...)
		if err == nil {
			return
		}

		var problem *Problem
		if !errors.As(err, &problem) || problem == nil {
			t.Fatalf("DecodeJSON() error = %T, want *Problem or nil", err)
		}
		if status := problem.Status(); status < 400 || status > 499 {
			t.Fatalf("problem status = %d, want 4xx", status)
		}
		if code := problem.Code(); code == "" {
			t.Fatal("problem code = empty, want stable machine code")
		}
		if message := problem.Message(); message == "" {
			t.Fatal("problem message = empty, want public message")
		}
	})
}

func FuzzDecodeQuery(f *testing.F) {
	f.Add("id=u_1&page=2&active=true", false)
	f.Add("id=u_1&extra=yes", false)
	f.Add("page=abc", false)
	f.Add("tag=a&tag=b", false)
	f.Add("%zz", false)

	f.Fuzz(func(t *testing.T, rawQuery string, allowUnknown bool) {
		if len(rawQuery) > 2<<10 {
			t.Skip()
		}

		req := &http.Request{
			Method: http.MethodGet,
			URL: &url.URL{
				Path:     "/users",
				RawQuery: rawQuery,
			},
		}

		var dst struct {
			ID     string   `query:"id"`
			Page   int      `query:"page"`
			Active bool     `query:"active"`
			Tags   []string `query:"tag"`
		}

		var err error
		if allowUnknown {
			err = DecodeQuery(req, &dst, AllowUnknownQueryFields())
		} else {
			err = DecodeQuery(req, &dst)
		}
		if err == nil {
			return
		}

		var problem *Problem
		if !errors.As(err, &problem) || problem == nil {
			t.Fatalf("DecodeQuery() error = %T, want *Problem or nil", err)
		}
		if status := problem.Status(); status < 400 || status > 499 {
			t.Fatalf("problem status = %d, want 4xx", status)
		}
		if code := problem.Code(); code == "" {
			t.Fatal("problem code = empty, want stable machine code")
		}
		if message := problem.Message(); message == "" {
			t.Fatal("problem message = empty, want public message")
		}
	})
}
