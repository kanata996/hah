package reqx

// 测试清单：
// - 标记说明：[✓] 已核对且已有真实覆盖；[x] 尚未完成，不得作为验收依据。
// - [✓] `From`、`PathParam`、`QueryParam` 会公开暴露 path/query 原始读取与常见类型绑定的稳定成功语义。
// - [✓] `PathParam`、`QueryParam` 在缺值、nil request、非法输入、自定义解码失败和 pointer 目标下会返回稳定的零值或 `bad_request`。
// - [✓] `QueryParam` 对多值 query 会维持 slice、首值标量与缺失切片的公开契约。

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/uuid"
)

type customParamValue struct {
	value string
	err   error
}

func (v *customParamValue) UnmarshalParam(param string) error {
	if v.err != nil {
		return v.err
	}
	v.value = param
	return nil
}

type customParamsValue struct {
	values []string
	err    error
}

func (v *customParamsValue) UnmarshalParams(params []string) error {
	if v.err != nil {
		return v.err
	}
	v.values = append([]string(nil), params...)
	return nil
}

type customTextValue string

func (v *customTextValue) UnmarshalText(text []byte) error {
	*v = customTextValue(text)
	return nil
}

var (
	errFailingParamValue  = errors.New("custom param decode failed")
	errFailingParamsValue = errors.New("custom params decode failed")
	errFailingTextValue   = errors.New("custom text decode failed")
)

type failingParamValue struct{}

func (*failingParamValue) UnmarshalParam(string) error {
	return errFailingParamValue
}

type failingParamsValue struct{}

func (*failingParamsValue) UnmarshalParams([]string) error {
	return errFailingParamsValue
}

type failingTextValue string

func (*failingTextValue) UnmarshalText([]byte) error {
	return errFailingTextValue
}

func TestFrom_ProvidesRawParamReaders(t *testing.T) {
	req := requestWithPathParams(map[string][]string{
		"id": {""},
	})
	req.URL = mustParseURL("/accounts?cursor=next&cursor=later")

	reader := From(req)
	if got := reader.PathParam("id"); got != "" {
		t.Fatalf("PathParam(id) = %q, want empty string", got)
	}
	if got := reader.QueryParam("cursor"); got != "next" {
		t.Fatalf("QueryParam(cursor) = %q, want next", got)
	}
	if got := From(nil).PathParam("id"); got != "" {
		t.Fatalf("From(nil).PathParam() = %q, want empty string", got)
	}
	if got := From(nil).QueryParam("cursor"); got != "" {
		t.Fatalf("From(nil).QueryParam() = %q, want empty string", got)
	}
}

func TestPathParam_BindsSupportedTypes(t *testing.T) {
	t.Run("int", func(t *testing.T) {
		req := requestWithPathParams(map[string][]string{
			"id": {"42"},
		})

		got, err := PathParam[int](req, "id")
		if err != nil {
			t.Fatalf("PathParam[int]() error = %v", err)
		}
		if got != 42 {
			t.Fatalf("PathParam[int]() = %d, want 42", got)
		}
	})

	t.Run("string", func(t *testing.T) {
		req := requestWithPathParams(map[string][]string{
			"name": {"alice"},
		})

		got, err := PathParam[string](req, "name")
		if err != nil {
			t.Fatalf("PathParam[string]() error = %v", err)
		}
		if got != "alice" {
			t.Fatalf("PathParam[string]() = %q, want alice", got)
		}
	})

	t.Run("bind unmarshaler", func(t *testing.T) {
		req := requestWithPathParams(map[string][]string{
			"id": {"u_1"},
		})

		got, err := PathParam[customParamValue](req, "id")
		if err != nil {
			t.Fatalf("PathParam[customParamValue]() error = %v", err)
		}
		if got.value != "u_1" {
			t.Fatalf("PathParam[customParamValue]() = %#v, want value u_1", got)
		}
	})

	t.Run("text unmarshaler", func(t *testing.T) {
		req := requestWithPathParams(map[string][]string{
			"id": {"state"},
		})

		got, err := PathParam[customTextValue](req, "id")
		if err != nil {
			t.Fatalf("PathParam[customTextValue]() error = %v", err)
		}
		if got != "state" {
			t.Fatalf("PathParam[customTextValue]() = %q, want state", got)
		}
	})

	t.Run("uuid", func(t *testing.T) {
		want := uuid.New()
		req := requestWithPathParams(map[string][]string{
			"id": {want.String()},
		})

		got, err := PathParam[uuid.UUID](req, "id")
		if err != nil {
			t.Fatalf("PathParam[uuid.UUID]() error = %v", err)
		}
		if got != want {
			t.Fatalf("PathParam[uuid.UUID]() = %v, want %v", got, want)
		}
	})
}

func TestQueryParam_BindsSupportedTypes(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/accounts?tag=a&tag=b&state=open", nil)

	t.Run("slice", func(t *testing.T) {
		got, err := QueryParam[[]string](req, "tag")
		if err != nil {
			t.Fatalf("QueryParam[[]string]() error = %v", err)
		}
		if len(got) != 2 || got[0] != "a" || got[1] != "b" {
			t.Fatalf("QueryParam[[]string]() = %#v, want [a b]", got)
		}
	})

	t.Run("multiple unmarshaler", func(t *testing.T) {
		got, err := QueryParam[customParamsValue](req, "tag")
		if err != nil {
			t.Fatalf("QueryParam[customParamsValue]() error = %v", err)
		}
		if len(got.values) != 2 || got.values[0] != "a" || got.values[1] != "b" {
			t.Fatalf("QueryParam[customParamsValue]() = %#v, want values [a b]", got)
		}
	})

	t.Run("scalar uses first value", func(t *testing.T) {
		got, err := QueryParam[string](req, "tag")
		if err != nil {
			t.Fatalf("QueryParam[string]() error = %v", err)
		}
		if got != "a" {
			t.Fatalf("QueryParam[string]() = %q, want a", got)
		}
	})
}

func TestRequestParamHelpers_PointerTargets(t *testing.T) {
	t.Run("path param", func(t *testing.T) {
		req := requestWithPathParams(map[string][]string{
			"id": {"42"},
		})

		got, err := PathParam[*int](req, "id")
		if err != nil {
			t.Fatalf("PathParam[*int]() error = %v", err)
		}
		if got == nil || *got != 42 {
			t.Fatalf("PathParam[*int]() = %#v, want pointer to 42", got)
		}
	})

	t.Run("query param", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/accounts?page=7", nil)

		got, err := QueryParam[*int](req, "page")
		if err != nil {
			t.Fatalf("QueryParam[*int]() error = %v", err)
		}
		if got == nil || *got != 7 {
			t.Fatalf("QueryParam[*int]() = %#v, want pointer to 7", got)
		}
	})

	t.Run("missing query param returns nil", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/accounts", nil)

		got, err := QueryParam[*int](req, "page")
		if err != nil {
			t.Fatalf("QueryParam[*int](missing) error = %v", err)
		}
		if got != nil {
			t.Fatalf("QueryParam[*int](missing) = %#v, want nil", got)
		}
	})
}

func TestRequestParamHelpers_MissingValueReturnsZeroValue(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/accounts", nil)

	gotPath, err := PathParam[uuid.UUID](req, "id")
	if err != nil {
		t.Fatalf("PathParam(missing) error = %v", err)
	}
	if gotPath != uuid.Nil {
		t.Fatalf("PathParam(missing) = %v, want Nil", gotPath)
	}

	gotQuery, err := QueryParam[int](req, "page")
	if err != nil {
		t.Fatalf("QueryParam(missing) error = %v", err)
	}
	if gotQuery != 0 {
		t.Fatalf("QueryParam(missing) = %d, want 0", gotQuery)
	}
}

func TestRequestParamHelpers_InvalidInputIsBadRequest(t *testing.T) {
	t.Run("path int", func(t *testing.T) {
		req := requestWithPathParams(map[string][]string{
			"id": {"oops"},
		})

		if _, err := PathParam[int](req, "id"); err == nil {
			t.Fatal("PathParam[int](invalid) = nil, want error")
		} else {
			_ = assertHTTPError(t, err, http.StatusBadRequest, "bad_request", "Bad Request")
		}
	})

	t.Run("path uuid", func(t *testing.T) {
		req := requestWithPathParams(map[string][]string{
			"id": {"not-a-uuid"},
		})

		if _, err := PathParam[uuid.UUID](req, "id"); err == nil {
			t.Fatal("PathParam[uuid.UUID](invalid) = nil, want error")
		} else {
			_ = assertHTTPError(t, err, http.StatusBadRequest, "bad_request", "Bad Request")
		}
	})

	t.Run("query int", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/accounts?page=oops", nil)

		if _, err := QueryParam[int](req, "page"); err == nil {
			t.Fatal("QueryParam[int](invalid) = nil, want error")
		} else {
			_ = assertHTTPError(t, err, http.StatusBadRequest, "bad_request", "Bad Request")
		}
	})
}

func TestRequestParamHelpers_CustomTypeErrorsAreBadRequest(t *testing.T) {
	t.Run("path custom unmarshaler", func(t *testing.T) {
		req := requestWithPathParams(map[string][]string{
			"id": {"u_1"},
		})

		if _, err := PathParam[failingParamValue](req, "id"); err == nil {
			t.Fatal("PathParam[failingParamValue]() = nil, want error")
		} else {
			_ = assertHTTPError(t, err, http.StatusBadRequest, "bad_request", "Bad Request")
		}
	})

	t.Run("path text unmarshaler", func(t *testing.T) {
		req := requestWithPathParams(map[string][]string{
			"id": {"state"},
		})

		if _, err := PathParam[failingTextValue](req, "id"); err == nil {
			t.Fatal("PathParam[failingTextValue]() = nil, want error")
		} else {
			_ = assertHTTPError(t, err, http.StatusBadRequest, "bad_request", "Bad Request")
		}
	})

	t.Run("query multiple unmarshaler", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/accounts?tag=a&tag=b", nil)

		if _, err := QueryParam[failingParamsValue](req, "tag"); err == nil {
			t.Fatal("QueryParam[failingParamsValue]() = nil, want error")
		} else {
			_ = assertHTTPError(t, err, http.StatusBadRequest, "bad_request", "Bad Request")
		}
	})
}

func TestRequestParamHelpers_InvalidPointerTargetsAreBadRequest(t *testing.T) {
	pathReq := requestWithPathParams(map[string][]string{
		"id": {"oops"},
	})
	queryReq := httptest.NewRequest(http.MethodGet, "/accounts?page=oops", nil)

	if _, err := PathParam[*int](pathReq, "id"); err == nil {
		t.Fatal("PathParam[*int](invalid) = nil, want error")
	} else {
		_ = assertHTTPError(t, err, http.StatusBadRequest, "bad_request", "Bad Request")
	}

	if _, err := QueryParam[*int](queryReq, "page"); err == nil {
		t.Fatal("QueryParam[*int](invalid) = nil, want error")
	} else {
		_ = assertHTTPError(t, err, http.StatusBadRequest, "bad_request", "Bad Request")
	}
}

func TestRequestParamHelpers_NilRequest(t *testing.T) {
	if _, err := PathParam[string](nil, "id"); err == nil || err.Error() != "reqx: request must not be nil" {
		t.Fatalf("PathParam(nil) error = %v", err)
	}
	if _, err := QueryParam[string](nil, "id"); err == nil || err.Error() != "reqx: request must not be nil" {
		t.Fatalf("QueryParam(nil) error = %v", err)
	}
}

func TestQueryParam_BindsScalarTypes(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/items?page=3&active=true", nil)

	t.Run("int", func(t *testing.T) {
		got, err := QueryParam[int](req, "page")
		if err != nil {
			t.Fatalf("QueryParam[int]() error = %v", err)
		}
		if got != 3 {
			t.Fatalf("QueryParam[int]() = %d, want 3", got)
		}
	})

	t.Run("bool", func(t *testing.T) {
		got, err := QueryParam[bool](req, "active")
		if err != nil {
			t.Fatalf("QueryParam[bool]() error = %v", err)
		}
		if !got {
			t.Fatal("QueryParam[bool]() = false, want true")
		}
	})
}

func TestQueryParam_BindsIntSlice(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/items?id=1&id=2&id=3", nil)

	got, err := QueryParam[[]int](req, "id")
	if err != nil {
		t.Fatalf("QueryParam[[]int]() error = %v", err)
	}
	if len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Fatalf("QueryParam[[]int]() = %v, want [1 2 3]", got)
	}
}

func TestQueryParam_InvalidSliceElementIsBadRequest(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/items?id=1&id=oops", nil)

	if _, err := QueryParam[[]int](req, "id"); err == nil {
		t.Fatal("QueryParam[[]int](invalid element) = nil, want error")
	} else {
		_ = assertHTTPError(t, err, http.StatusBadRequest, "bad_request", "Bad Request")
	}
}

func TestQueryParam_MissingSliceReturnsNil(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/accounts", nil)

	got, err := QueryParam[[]string](req, "tags")
	if err != nil {
		t.Fatalf("QueryParam[[]string](missing) error = %v", err)
	}
	if got != nil {
		t.Fatalf("QueryParam[[]string](missing) = %v, want nil", got)
	}
}

func mustParseURL(raw string) *url.URL {
	parsed, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	return parsed
}
