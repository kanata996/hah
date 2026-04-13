package bind

// 测试清单：
// - 标记说明：[✓] 已核对且已有真实覆盖；[x] 尚未完成，不得作为验收依据。
// - [✓] BindPathValues、BindQueryParams、BindHeaders 的公开成功、失败和保留语义。
// - [✓] 单源公开 API 支持约定的 map 目标语义。
// - [✓] path/query/header 相关错误统一收敛为 400 bad_request。
// - [✓] 字符串源绑定的关键内部辅助契约，包括反射写入、自定义解码和 path helper。
// - [✓] BindHeaders 支持切片字段绑定多值 header。
// - [✓] BindQueryParams 对指针字段在缺失/空值时的保留与清零语义。
// - [✓] 单源公开 API 对非法目标值返回稳定错误，并定义字段级失败时的部分写入语义。
// - [✓] BindQueryParams 通过公开入口覆盖复杂类型的端到端绑定契约。

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kanata996/hah/errx"
	ireq "github.com/kanata996/hah/internal/req"
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
	if string(text) == "bad" {
		return errors.New("bad text")
	}
	*v = customTextValue(text)
	return nil
}

type queryState string

func (s *queryState) UnmarshalText(text []byte) error {
	switch string(text) {
	case "open", "closed":
		*s = queryState(text)
		return nil
	default:
		return fmt.Errorf("invalid state %q", string(text))
	}
}

func TestBindPathValues_BindsScalars(t *testing.T) {
	type request struct {
		ID   int    `param:"id"`
		Name string `param:"name"`
	}

	req := requestWithPathParams(map[string][]string{
		"id":   {"42"},
		"name": {"kanata"},
	})

	var dst request
	if err := BindPathValues(req, &dst); err != nil {
		t.Fatalf("BindPathValues() error = %v", err)
	}
	if dst.ID != 42 || dst.Name != "kanata" {
		t.Fatalf("dst = %#v, want bound path values", dst)
	}
}

func TestBindPathValues_MissingParamsPreserveExistingValues(t *testing.T) {
	type request struct {
		ID   int    `param:"id"`
		Name string `param:"name"`
	}

	req := requestWithPathParams(map[string][]string{
		"other": {"1"},
	})
	dst := request{ID: 7, Name: "existing"}

	if err := BindPathValues(req, &dst); err != nil {
		t.Fatalf("BindPathValues() error = %v", err)
	}
	if dst.ID != 7 || dst.Name != "existing" {
		t.Fatalf("dst = %#v, want existing values preserved", dst)
	}
}

func TestBindPathValues_EmptyValueBindsZeroValue(t *testing.T) {
	type request struct {
		ID int `param:"id"`
	}

	req := requestWithPathParams(map[string][]string{
		"id": {""},
	})

	dst := request{ID: 7}
	if err := BindPathValues(req, &dst); err != nil {
		t.Fatalf("BindPathValues() error = %v", err)
	}
	if dst.ID != 0 {
		t.Fatalf("id = %d, want 0 after empty path value overwrites existing value", dst.ID)
	}
}

func TestBindPathValues_BindingErrorsAreBadRequest(t *testing.T) {
	type request struct {
		ID int `param:"id"`
	}

	req := requestWithPathParams(map[string][]string{
		"id": {"oops"},
	})

	var dst request
	_ = assertHTTPError(t, BindPathValues(req, &dst), http.StatusBadRequest, "bad_request", "Bad Request")
}

func TestBindPathValues_NameMatchingIsCaseSensitive(t *testing.T) {
	type request struct {
		ID string `param:"id"`
	}

	req := requestWithPathParams(map[string][]string{
		"ID": {"route-id"},
	})

	dst := request{ID: "existing"}
	if err := BindPathValues(req, &dst); err != nil {
		t.Fatalf("BindPathValues() error = %v", err)
	}
	if dst.ID != "existing" {
		t.Fatalf("id = %q, want existing value preserved on mismatched case", dst.ID)
	}
}

func TestBindQueryParams_BindsSupportedTypes(t *testing.T) {
	type request struct {
		Page   int        `query:"page"`
		Search string     `query:"search"`
		State  queryState `query:"state"`
		IDs    []int      `query:"id"`
	}

	req := httptest.NewRequest(http.MethodGet, "/?page=2&search=kanata&state=open&id=1&id=2", nil)

	var dst request
	if err := BindQueryParams(req, &dst); err != nil {
		t.Fatalf("BindQueryParams() error = %v", err)
	}
	if dst.Page != 2 || dst.Search != "kanata" || dst.State != "open" {
		t.Fatalf("dst = %#v", dst)
	}
	if len(dst.IDs) != 2 || dst.IDs[0] != 1 || dst.IDs[1] != 2 {
		t.Fatalf("ids = %#v, want [1 2]", dst.IDs)
	}
}

func TestBindSingleSourceAPIs_BindMapTargets(t *testing.T) {
	t.Run("query binds supported map targets", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/?name=kanata&tag=a&tag=b", nil)

		stringMap := map[string]string(nil)
		if err := BindQueryParams(req, &stringMap); err != nil {
			t.Fatalf("BindQueryParams(map[string]string) error = %v", err)
		}
		if got := stringMap["name"]; got != "kanata" {
			t.Fatalf("stringMap[name] = %q, want kanata", got)
		}
		if got := stringMap["tag"]; got != "a" {
			t.Fatalf("stringMap[tag] = %q, want first value a", got)
		}

		sliceMap := map[string][]string(nil)
		if err := BindQueryParams(req, &sliceMap); err != nil {
			t.Fatalf("BindQueryParams(map[string][]string) error = %v", err)
		}
		if !reflect.DeepEqual(sliceMap["tag"], []string{"a", "b"}) {
			t.Fatalf("sliceMap[tag] = %#v, want [a b]", sliceMap["tag"])
		}

		anyMap := map[string]any(nil)
		if err := BindQueryParams(req, &anyMap); err != nil {
			t.Fatalf("BindQueryParams(map[string]any) error = %v", err)
		}
		if got := anyMap["name"]; got != "kanata" {
			t.Fatalf("anyMap[name] = %#v, want kanata", got)
		}
	})

	t.Run("path binds supported map targets", func(t *testing.T) {
		req := requestWithPathParams(map[string][]string{
			"id":   {"42"},
			"name": {"kanata"},
		})

		stringMap := map[string]string(nil)
		if err := BindPathValues(req, &stringMap); err != nil {
			t.Fatalf("BindPathValues(map[string]string) error = %v", err)
		}
		if got := stringMap["id"]; got != "42" {
			t.Fatalf("stringMap[id] = %q, want 42", got)
		}
		if got := stringMap["name"]; got != "kanata" {
			t.Fatalf("stringMap[name] = %q, want kanata", got)
		}
	})

	t.Run("header binds supported map targets", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Add("X-Trace-Id", "req-1")
		req.Header.Add("X-Trace-Id", "req-2")
		req.Header.Set("X-Name", "kanata")

		sliceMap := map[string][]string(nil)
		if err := BindHeaders(req, &sliceMap); err != nil {
			t.Fatalf("BindHeaders(map[string][]string) error = %v", err)
		}
		if !reflect.DeepEqual(sliceMap["X-Trace-Id"], []string{"req-1", "req-2"}) {
			t.Fatalf("sliceMap[X-Trace-Id] = %#v, want [req-1 req-2]", sliceMap["X-Trace-Id"])
		}

		anyMap := map[string]any(nil)
		if err := BindHeaders(req, &anyMap); err != nil {
			t.Fatalf("BindHeaders(map[string]any) error = %v", err)
		}
		if got := anyMap["X-Name"]; got != "kanata" {
			t.Fatalf("anyMap[X-Name] = %#v, want kanata", got)
		}
	})
}

func TestBindQueryParams_MissingParamsPreserveExistingValues(t *testing.T) {
	type request struct {
		Page   int    `query:"page"`
		Search string `query:"search"`
	}

	req := httptest.NewRequest(http.MethodGet, "/?other=1", nil)
	dst := request{Page: 3, Search: "existing"}

	if err := BindQueryParams(req, &dst); err != nil {
		t.Fatalf("BindQueryParams() error = %v", err)
	}
	if dst.Page != 3 || dst.Search != "existing" {
		t.Fatalf("dst = %#v, want existing values preserved", dst)
	}
}

func TestBindQueryParams_RepeatedScalarUsesFirstValue(t *testing.T) {
	type request struct {
		Page int `query:"page"`
	}

	req := httptest.NewRequest(http.MethodGet, "/?page=1&page=2", nil)

	var dst request
	if err := BindQueryParams(req, &dst); err != nil {
		t.Fatalf("BindQueryParams() error = %v", err)
	}
	if dst.Page != 1 {
		t.Fatalf("page = %d, want 1", dst.Page)
	}
}

func TestBindQueryParams_NameMatchingIsCaseSensitive(t *testing.T) {
	type request struct {
		Page int `query:"page"`
	}

	req := httptest.NewRequest(http.MethodGet, "/?PAGE=7", nil)

	dst := request{Page: 3}
	if err := BindQueryParams(req, &dst); err != nil {
		t.Fatalf("BindQueryParams() error = %v", err)
	}
	if dst.Page != 3 {
		t.Fatalf("page = %d, want existing value preserved on mismatched case", dst.Page)
	}
}

func TestBindQueryParams_BindingErrorsAreBadRequest(t *testing.T) {
	type request struct {
		Page int `query:"page"`
	}

	req := httptest.NewRequest(http.MethodGet, "/?page=oops", nil)

	var dst request
	_ = assertHTTPError(t, BindQueryParams(req, &dst), http.StatusBadRequest, "bad_request", "Bad Request")
}

func TestBindHeaders_BindsSupportedScalarTypes(t *testing.T) {
	type request struct {
		RequestID string `header:"x-request-id"`
		Retry     int    `header:"x-retry"`
		Enabled   bool   `header:"x-enabled"`
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-Id", "req-123")
	req.Header.Set("X-Retry", "2")
	req.Header.Set("X-Enabled", "true")

	var dst request
	if err := BindHeaders(req, &dst); err != nil {
		t.Fatalf("BindHeaders() error = %v", err)
	}
	if dst.RequestID != "req-123" || dst.Retry != 2 || !dst.Enabled {
		t.Fatalf("dst = %#v, want bound header values", dst)
	}
}

func TestBindHeaders_RepeatedScalarUsesFirstValue(t *testing.T) {
	type request struct {
		RequestID string `header:"x-request-id"`
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Add("X-Request-Id", "req-1")
	req.Header.Add("X-Request-Id", "req-2")

	var dst request
	if err := BindHeaders(req, &dst); err != nil {
		t.Fatalf("BindHeaders() error = %v", err)
	}
	if dst.RequestID != "req-1" {
		t.Fatalf("request_id = %q, want req-1", dst.RequestID)
	}
}

func TestBindHeaders_TrimmedNonCanonicalKeysStillBind(t *testing.T) {
	type request struct {
		Name string `header:"x-name"`
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header = http.Header{
		" ":      {"ignored"},
		"x-name": {"kanata"},
	}

	var dst request
	if err := BindHeaders(req, &dst); err != nil {
		t.Fatalf("BindHeaders() error = %v", err)
	}
	if dst.Name != "kanata" {
		t.Fatalf("name = %q, want kanata", dst.Name)
	}
}

func TestBindHeaders_BindingErrorsAreBadRequest(t *testing.T) {
	type request struct {
		Retry int `header:"x-retry"`
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Retry", "oops")

	var dst request
	_ = assertHTTPError(t, BindHeaders(req, &dst), http.StatusBadRequest, "bad_request", "Bad Request")
}

func TestBindHeaders_MissingInputsPreserveExistingValues(t *testing.T) {
	type request struct {
		TraceID string `header:"x-trace-id"`
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	dst := request{TraceID: "existing"}
	if err := BindHeaders(req, &dst); err != nil {
		t.Fatalf("BindHeaders() error = %v", err)
	}
	if dst.TraceID != "existing" {
		t.Fatalf("trace_id = %q, want existing", dst.TraceID)
	}
}

func TestBindHeaders_EmptyValueListsAreIgnored(t *testing.T) {
	type request struct {
		TraceID string `header:"x-trace-id"`
	}

	t.Run("nil slice", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header = http.Header{
			"X-Trace-Id": nil,
		}

		dst := request{TraceID: "existing"}
		if err := BindHeaders(req, &dst); err != nil {
			t.Fatalf("BindHeaders() error = %v", err)
		}
		if dst.TraceID != "existing" {
			t.Fatalf("trace_id = %q, want existing", dst.TraceID)
		}
	})

	t.Run("empty slice", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header = http.Header{
			"X-Trace-Id": {},
		}

		dst := request{TraceID: "existing"}
		if err := BindHeaders(req, &dst); err != nil {
			t.Fatalf("BindHeaders() error = %v", err)
		}
		if dst.TraceID != "existing" {
			t.Fatalf("trace_id = %q, want existing", dst.TraceID)
		}
	})
}

func TestBindValues_HelperBranches(t *testing.T) {
	if got := badRequestWrap(nil); got != nil {
		t.Fatalf("badRequestWrap(nil) = %v, want nil", got)
	}

	httpErr := errx.BadRequest("bad_request", "bad request")
	_ = assertHTTPError(t, badRequestWrap(httpErr), http.StatusBadRequest, "bad_request", "bad request")

	wrapped := badRequestWrap(errors.New("boom"))
	_ = assertHTTPError(t, wrapped, http.StatusBadRequest, "bad_request", "Bad Request")
}

func TestBindDataDefaultBranches(t *testing.T) {
	if err := bindDataDefault(nil, nil, "query"); err != nil {
		t.Fatalf("bindDataDefault(nil empty) error = %v", err)
	}
	if err := bindDataDefault(1, map[string][]string{"x": {"1"}}, "query"); err == nil || err.Error() != "binding element must be a pointer" {
		t.Fatalf("bindDataDefault(non-pointer) error = %v", err)
	}

	t.Run("map targets", func(t *testing.T) {
		stringMap := map[string]string(nil)
		if err := bindDataDefault(&stringMap, map[string][]string{"name": {"kanata"}, "skip": {}}, "query"); err != nil {
			t.Fatalf("bindDataDefault(map[string]string) error = %v", err)
		}
		if got := stringMap["name"]; got != "kanata" {
			t.Fatalf("stringMap[name] = %q, want kanata", got)
		}
		if _, ok := stringMap["skip"]; ok {
			t.Fatalf("stringMap[skip] unexpectedly set")
		}

		sliceMap := map[string][]string(nil)
		if err := bindDataDefault(&sliceMap, map[string][]string{"tag": {"a", "b"}}, "query"); err != nil {
			t.Fatalf("bindDataDefault(map[string][]string) error = %v", err)
		}
		if got := strings.Join(sliceMap["tag"], ","); got != "a,b" {
			t.Fatalf("sliceMap[tag] = %q, want a,b", got)
		}

		anyMap := map[string]any(nil)
		if err := bindDataDefault(&anyMap, map[string][]string{"name": {"kanata"}}, "query"); err != nil {
			t.Fatalf("bindDataDefault(map[string]any) error = %v", err)
		}
		if got := anyMap["name"]; got != "kanata" {
			t.Fatalf("anyMap[name] = %#v, want kanata", got)
		}

		intMap := map[string]int(nil)
		if err := bindDataDefault(&intMap, map[string][]string{"n": {"1"}}, "query"); err != nil {
			t.Fatalf("bindDataDefault(map[string]int) error = %v", err)
		}
		if intMap != nil {
			t.Fatalf("intMap = %#v, want nil no-op", intMap)
		}

		type stringKey string
		keyMap := map[stringKey]string(nil)
		if err := bindDataDefault(&keyMap, map[string][]string{"name": {"kanata"}}, "query"); err != nil {
			t.Fatalf("bindDataDefault(map[custom]string) error = %v", err)
		}
		if keyMap != nil {
			t.Fatalf("keyMap = %#v, want nil no-op", keyMap)
		}

		stringerMap := map[string]fmt.Stringer(nil)
		if err := bindDataDefault(&stringerMap, map[string][]string{"name": {"kanata"}}, "query"); err != nil {
			t.Fatalf("bindDataDefault(map[string]fmt.Stringer) error = %v", err)
		}
		if stringerMap != nil {
			t.Fatalf("stringerMap = %#v, want nil no-op", stringerMap)
		}
	})

	t.Run("scalar destination rules", func(t *testing.T) {
		value := 1
		if err := bindDataDefault(&value, map[string][]string{"n": {"1"}}, "json"); err == nil || err.Error() != "binding element must be a struct" {
			t.Fatalf("bindDataDefault(json scalar) error = %v", err)
		}
		if err := bindDataDefault(&value, map[string][]string{"n": {"1"}}, "query"); err != nil {
			t.Fatalf("bindDataDefault(query scalar) error = %v", err)
		}
	})

	t.Run("empty value lists are ignored for struct fields", func(t *testing.T) {
		type request struct {
			Page int `query:"page"`
		}

		dst := request{Page: 3}
		if err := bindDataDefault(&dst, map[string][]string{"page": {}}, "query"); err != nil {
			t.Fatalf("bindDataDefault(empty values) error = %v", err)
		}
		if dst.Page != 3 {
			t.Fatalf("page = %d, want existing value preserved", dst.Page)
		}
	})

	t.Run("struct binding", func(t *testing.T) {
		type nested struct {
			Name string `query:"name"`
		}
		type request struct {
			Nested nested
			Age    *int              `query:"age"`
			IDs    []int             `query:"id"`
			When   time.Time         `query:"when" format:"2006-01-02"`
			Whens  []time.Time       `query:"whens" format:"15:04:05"`
			Custom customParamValue  `query:"custom"`
			Multi  customParamsValue `query:"multi"`
			State  customTextValue   `query:"state"`
			Trace  string            `header:"x-trace-id"`
		}

		var dst request
		err := bindDataDefault(&dst, map[string][]string{
			"name":       {"kanata"},
			"age":        {"17"},
			"id":         {"1", "2"},
			"when":       {"2026-04-09"},
			"whens":      {"10:11:12", "13:14:15"},
			"custom":     {"x"},
			"multi":      {"a", "b"},
			"state":      {"open"},
			"X-Trace-Id": {"req-1"},
		}, "query")
		if err != nil {
			t.Fatalf("bindDataDefault(struct) error = %v", err)
		}
		if dst.Nested.Name != "kanata" {
			t.Fatalf("Nested.Name = %q, want kanata", dst.Nested.Name)
		}
		if dst.Age == nil || *dst.Age != 17 {
			t.Fatalf("Age = %#v, want 17", dst.Age)
		}
		if !reflect.DeepEqual(dst.IDs, []int{1, 2}) {
			t.Fatalf("IDs = %#v, want [1 2]", dst.IDs)
		}
		if got := dst.When.Format("2006-01-02"); got != "2026-04-09" {
			t.Fatalf("When = %q, want 2026-04-09", got)
		}
		if len(dst.Whens) != 2 || dst.Whens[0].Format("15:04:05") != "10:11:12" || dst.Whens[1].Format("15:04:05") != "13:14:15" {
			t.Fatalf("Whens = %v, want 10:11:12 and 13:14:15", dst.Whens)
		}
		if dst.Custom.value != "x" {
			t.Fatalf("Custom = %#v, want x", dst.Custom)
		}
		if !reflect.DeepEqual(dst.Multi.values, []string{"a", "b"}) {
			t.Fatalf("Multi = %#v, want [a b]", dst.Multi)
		}
		if dst.State != "open" {
			t.Fatalf("State = %q, want open", dst.State)
		}

		headerDst := struct {
			Trace string `header:"x-trace-id"`
		}{}
		if err := bindDataDefault(&headerDst, map[string][]string{"X-Trace-Id": {"req-1"}}, "header"); err != nil {
			t.Fatalf("bindDataDefault(case-insensitive header) error = %v", err)
		}
		if headerDst.Trace != "req-1" {
			t.Fatalf("Trace = %q, want req-1", headerDst.Trace)
		}
	})

	t.Run("anonymous tagged struct is rejected", func(t *testing.T) {
		type Embedded struct {
			Name string
		}
		type request struct {
			Embedded `query:"name"`
		}

		var dst request
		err := bindDataDefault(&dst, map[string][]string{"name": {"kanata"}}, "query")
		if err == nil || err.Error() != "query/param/header tags are not allowed with anonymous struct field" {
			t.Fatalf("bindDataDefault(anonymous tagged) error = %v", err)
		}
	})

	t.Run("anonymous pointer nil and unexported field are skipped", func(t *testing.T) {
		type embedded struct {
			Name string `query:"name"`
		}
		type request struct {
			*embedded
			name string `query:"name"`
		}

		dst := request{}
		if err := bindDataDefault(&dst, map[string][]string{"name": {"kanata"}}, "query"); err != nil {
			t.Fatalf("bindDataDefault(skip nil embedded/unexported) error = %v", err)
		}
		if dst.embedded != nil {
			t.Fatalf("embedded = %#v, want nil", dst.embedded)
		}
		if dst.name != "" {
			t.Fatalf("name = %q, want empty", dst.name)
		}
	})

	t.Run("anonymous pointer non nil is traversed", func(t *testing.T) {
		type Embedded struct {
			Name string `query:"name"`
		}
		type request struct {
			*Embedded
		}

		dst := request{Embedded: &Embedded{}}
		if err := bindDataDefault(&dst, map[string][]string{"name": {"kanata"}}, "query"); err != nil {
			t.Fatalf("bindDataDefault(non-nil embedded pointer) error = %v", err)
		}
		if dst.Embedded == nil || dst.Name != "kanata" {
			t.Fatalf("embedded = %#v, want name=kanata", dst.Embedded)
		}
	})

	t.Run("recursive and decoder errors propagate", func(t *testing.T) {
		type nested struct {
			Age int `query:"age"`
		}
		type request struct {
			Nested nested
		}

		var recursive request
		err := bindDataDefault(&recursive, map[string][]string{"age": {"oops"}}, "query")
		if err == nil {
			t.Fatal("bindDataDefault(recursive error) = nil")
		}

		type withMulti struct {
			Multi customParamsValue `query:"multi"`
		}
		var multi withMulti
		multi.Multi.err = errors.New("multi failed")
		err = bindDataDefault(&multi, map[string][]string{"multi": {"x"}}, "query")
		if err == nil || err.Error() != "multi failed" {
			t.Fatalf("bindDataDefault(multi error) = %v", err)
		}

		type withCustom struct {
			Custom customParamValue `query:"custom"`
		}
		var custom withCustom
		custom.Custom.err = errors.New("custom failed")
		err = bindDataDefault(&custom, map[string][]string{"custom": {"x"}}, "query")
		if err == nil || err.Error() != "custom failed" {
			t.Fatalf("bindDataDefault(custom error) = %v", err)
		}

		type withTime struct {
			When time.Time `query:"when" format:"2006-01-02"`
		}
		var timed withTime
		err = bindDataDefault(&timed, map[string][]string{"when": {"bad"}}, "query")
		if err == nil {
			t.Fatal("bindDataDefault(time parse error) = nil")
		}

		type withTimes struct {
			Whens []time.Time `query:"whens" format:"15:04:05"`
		}
		var timeds withTimes
		err = bindDataDefault(&timeds, map[string][]string{"whens": {"10:11:12", "bad"}}, "query")
		if err == nil {
			t.Fatal("bindDataDefault(times slice parse error) = nil")
		}

		type withIDs struct {
			IDs []int `query:"id"`
		}
		var ids withIDs
		err = bindDataDefault(&ids, map[string][]string{"id": {"1", "oops"}}, "query")
		if err == nil {
			t.Fatal("bindDataDefault(slice parse error) = nil")
		}
	})
}

func TestUnmarshalHelpersAndSetters(t *testing.T) {
	var multi customParamsValue
	ok, err := unmarshalInputsToFieldDefault(reflect.Slice, []string{"a", "b"}, reflect.ValueOf(&multi).Elem())
	if !ok || err != nil || !reflect.DeepEqual(multi.values, []string{"a", "b"}) {
		t.Fatalf("unmarshalInputsToFieldDefault(slice) = (%v, %v), values=%#v", ok, err, multi.values)
	}

	var multiPtr *customParamsValue
	ok, err = unmarshalInputsToFieldDefault(reflect.Pointer, []string{"x"}, reflect.ValueOf(&multiPtr).Elem())
	if !ok || err != nil || multiPtr == nil || !reflect.DeepEqual(multiPtr.values, []string{"x"}) {
		t.Fatalf("unmarshalInputsToFieldDefault(pointer) = (%v, %v), value=%#v", ok, err, multiPtr)
	}

	var plain string
	ok, err = unmarshalInputsToFieldDefault(reflect.String, []string{"x"}, reflect.ValueOf(&plain).Elem())
	if ok || err != nil {
		t.Fatalf("unmarshalInputsToFieldDefault(string) = (%v, %v), want false nil", ok, err)
	}

	var when time.Time
	ok, err = unmarshalInputToFieldDefault(reflect.Struct, "2026-04-09", reflect.ValueOf(&when).Elem(), "2006-01-02")
	if !ok || err != nil || when.Format("2006-01-02") != "2026-04-09" {
		t.Fatalf("unmarshalInputToFieldDefault(time) = (%v, %v), when=%v", ok, err, when)
	}
	ok, err = unmarshalInputToFieldDefault(reflect.Struct, "bad", reflect.ValueOf(&when).Elem(), "2006-01-02")
	if !ok || err == nil {
		t.Fatalf("unmarshalInputToFieldDefault(invalid time) = (%v, %v), want true error", ok, err)
	}

	var custom *customParamValue
	ok, err = unmarshalInputToFieldDefault(reflect.Pointer, "value", reflect.ValueOf(&custom).Elem(), "")
	if !ok || err != nil || custom == nil || custom.value != "value" {
		t.Fatalf("unmarshalInputToFieldDefault(BindUnmarshaler) = (%v, %v), value=%#v", ok, err, custom)
	}

	var text customTextValue
	ok, err = unmarshalInputToFieldDefault(reflect.String, "open", reflect.ValueOf(&text).Elem(), "")
	if !ok || err != nil || text != "open" {
		t.Fatalf("unmarshalInputToFieldDefault(TextUnmarshaler) = (%v, %v), value=%q", ok, err, text)
	}

	ok, err = unmarshalInputToFieldDefault(reflect.Int, "1", reflect.ValueOf(new(int)).Elem(), "")
	if ok || err != nil {
		t.Fatalf("unmarshalInputToFieldDefault(int) = (%v, %v), want false nil", ok, err)
	}

	t.Run("scalar kinds", func(t *testing.T) {
		var i int
		if err := setWithProperTypeDefault(reflect.Int, "1", reflect.ValueOf(&i).Elem(), ""); err != nil || i != 1 {
			t.Fatalf("setWithProperTypeDefault(int) error = %v, value=%d", err, i)
		}
		var i8 int8
		if err := setWithProperTypeDefault(reflect.Int8, "1", reflect.ValueOf(&i8).Elem(), ""); err != nil || i8 != 1 {
			t.Fatalf("setWithProperTypeDefault(int8) error = %v, value=%d", err, i8)
		}
		var i16 int16
		if err := setWithProperTypeDefault(reflect.Int16, "1", reflect.ValueOf(&i16).Elem(), ""); err != nil || i16 != 1 {
			t.Fatalf("setWithProperTypeDefault(int16) error = %v, value=%d", err, i16)
		}
		var i32 int32
		if err := setWithProperTypeDefault(reflect.Int32, "1", reflect.ValueOf(&i32).Elem(), ""); err != nil || i32 != 1 {
			t.Fatalf("setWithProperTypeDefault(int32) error = %v, value=%d", err, i32)
		}
		var i64 int64
		if err := setWithProperTypeDefault(reflect.Int64, "1", reflect.ValueOf(&i64).Elem(), ""); err != nil || i64 != 1 {
			t.Fatalf("setWithProperTypeDefault(int64) error = %v, value=%d", err, i64)
		}
		var u uint
		if err := setWithProperTypeDefault(reflect.Uint, "", reflect.ValueOf(&u).Elem(), ""); err != nil || u != 0 {
			t.Fatalf("setWithProperTypeDefault(uint) error = %v, value=%d", err, u)
		}
		var u8 uint8
		if err := setWithProperTypeDefault(reflect.Uint8, "1", reflect.ValueOf(&u8).Elem(), ""); err != nil || u8 != 1 {
			t.Fatalf("setWithProperTypeDefault(uint8) error = %v, value=%d", err, u8)
		}
		var u16 uint16
		if err := setWithProperTypeDefault(reflect.Uint16, "1", reflect.ValueOf(&u16).Elem(), ""); err != nil || u16 != 1 {
			t.Fatalf("setWithProperTypeDefault(uint16) error = %v, value=%d", err, u16)
		}
		var u32 uint32
		if err := setWithProperTypeDefault(reflect.Uint32, "1", reflect.ValueOf(&u32).Elem(), ""); err != nil || u32 != 1 {
			t.Fatalf("setWithProperTypeDefault(uint32) error = %v, value=%d", err, u32)
		}
		var u64 uint64
		if err := setWithProperTypeDefault(reflect.Uint64, "1", reflect.ValueOf(&u64).Elem(), ""); err != nil || u64 != 1 {
			t.Fatalf("setWithProperTypeDefault(uint64) error = %v, value=%d", err, u64)
		}
		var b bool
		if err := setWithProperTypeDefault(reflect.Bool, "", reflect.ValueOf(&b).Elem(), ""); err != nil || b {
			t.Fatalf("setWithProperTypeDefault(bool empty) error = %v, value=%v", err, b)
		}
		var f32 float32
		if err := setWithProperTypeDefault(reflect.Float32, "", reflect.ValueOf(&f32).Elem(), ""); err != nil || f32 != 0 {
			t.Fatalf("setWithProperTypeDefault(float32 empty) error = %v, value=%v", err, f32)
		}
		var f64 float64
		if err := setWithProperTypeDefault(reflect.Float64, "1.5", reflect.ValueOf(&f64).Elem(), ""); err != nil || f64 != 1.5 {
			t.Fatalf("setWithProperTypeDefault(float64) error = %v, value=%v", err, f64)
		}
		var s string
		if err := setWithProperTypeDefault(reflect.String, "x", reflect.ValueOf(&s).Elem(), ""); err != nil || s != "x" {
			t.Fatalf("setWithProperTypeDefault(string) error = %v, value=%q", err, s)
		}
		var ptr *int
		if err := setWithProperTypeDefault(reflect.Pointer, "2", reflect.ValueOf(&ptr).Elem(), ""); err != nil || ptr == nil || *ptr != 2 {
			t.Fatalf("setWithProperTypeDefault(pointer) error = %v, value=%#v", err, ptr)
		}
	})

	var unsupported struct{}
	if err := setWithProperTypeDefault(reflect.Struct, "x", reflect.ValueOf(&unsupported).Elem(), ""); err == nil || err.Error() != "unknown type" {
		t.Fatalf("setWithProperTypeDefault(struct) error = %v", err)
	}

	var customValue customParamValue
	if err := setWithProperTypeDefault(reflect.Struct, "value", reflect.ValueOf(&customValue).Elem(), ""); err != nil || customValue.value != "value" {
		t.Fatalf("setWithProperTypeDefault(BindUnmarshaler) error = %v, value=%#v", err, customValue)
	}
}

func TestPathHelpers(t *testing.T) {
	if got := ireq.PathWildcardNames("   "); got != nil {
		t.Fatalf("pathWildcardNames(blank) = %#v, want nil", got)
	}
	if got := ireq.PathWildcardNames("GET /users/{user_id}/files/{path...}/{$}/{id:rest}/{ }"); !reflect.DeepEqual(got, []string{"user_id", "path", "id"}) {
		t.Fatalf("pathWildcardNames() = %#v", got)
	}
	if got := ireq.PathWildcardNames("/users/{user_id"); len(got) != 0 {
		t.Fatalf("pathWildcardNames(invalid pattern) = %#v, want empty", got)
	}
}

func TestBindHeaders_BindsSliceFields(t *testing.T) {
	type request struct {
		Tags []string `header:"x-tags"`
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Add("X-Tags", "a")
	req.Header.Add("X-Tags", "b")

	var dst request
	if err := BindHeaders(req, &dst); err != nil {
		t.Fatalf("BindHeaders() error = %v", err)
	}
	if !reflect.DeepEqual(dst.Tags, []string{"a", "b"}) {
		t.Fatalf("tags = %#v, want [a b]", dst.Tags)
	}
}

func TestBindQueryParams_PointerFieldPreservation(t *testing.T) {
	type request struct {
		Page *int `query:"page"`
	}

	t.Run("nil pointer preserved when query param is missing", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/?other=1", nil)

		var dst request
		if err := BindQueryParams(req, &dst); err != nil {
			t.Fatalf("BindQueryParams() error = %v", err)
		}
		if dst.Page != nil {
			t.Fatalf("page = %#v, want nil", dst.Page)
		}
	})

	t.Run("empty value allocates pointer and sets zero", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/?page=", nil)

		var dst request
		if err := BindQueryParams(req, &dst); err != nil {
			t.Fatalf("BindQueryParams() error = %v", err)
		}
		if dst.Page == nil || *dst.Page != 0 {
			t.Fatalf("page = %#v, want &0", dst.Page)
		}
	})
}

func TestBindSingleSourcePublicAPIs_RejectInvalidDestinations(t *testing.T) {
	queryReq := httptest.NewRequest(http.MethodGet, "/?page=1", nil)
	pathReq := requestWithPathParams(map[string][]string{
		"id": {"1"},
	})
	headerReq := httptest.NewRequest(http.MethodGet, "/", nil)
	headerReq.Header.Set("X-Trace-Id", "req-1")

	tests := []struct {
		name string
		call func(any) error
	}{
		{
			name: "query rejects non-pointer destination",
			call: func(target any) error {
				return BindQueryParams(queryReq, target)
			},
		},
		{
			name: "path rejects non-pointer destination",
			call: func(target any) error {
				return BindPathValues(pathReq, target)
			},
		},
		{
			name: "header rejects non-pointer destination",
			call: func(target any) error {
				return BindHeaders(headerReq, target)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(struct{}{}); err == nil || err.Error() != "bind: destination must not be nil" {
				t.Fatalf("error = %v, want bind: destination must not be nil", err)
			}

			var typedNil *struct{}
			if err := tt.call(typedNil); err == nil || err.Error() != "bind: destination must not be nil" {
				t.Fatalf("typed nil error = %v, want bind: destination must not be nil", err)
			}
		})
	}
}

func TestBindQueryParams_BindsComplexTypesEndToEnd(t *testing.T) {
	type nested struct {
		Name string `query:"name"`
	}
	type request struct {
		Nested nested
		Age    *int              `query:"age"`
		IDs    []int             `query:"id"`
		When   time.Time         `query:"when" format:"2006-01-02"`
		Custom customParamValue  `query:"custom"`
		Multi  customParamsValue `query:"multi"`
		State  customTextValue   `query:"state"`
	}

	req := httptest.NewRequest(
		http.MethodGet,
		"/?name=kanata&age=17&id=1&id=2&when=2026-04-09&custom=x&multi=a&multi=b&state=open",
		nil,
	)

	var dst request
	if err := BindQueryParams(req, &dst); err != nil {
		t.Fatalf("BindQueryParams() error = %v", err)
	}
	if dst.Nested.Name != "kanata" {
		t.Fatalf("Nested.Name = %q, want kanata", dst.Nested.Name)
	}
	if dst.Age == nil || *dst.Age != 17 {
		t.Fatalf("Age = %#v, want 17", dst.Age)
	}
	if !reflect.DeepEqual(dst.IDs, []int{1, 2}) {
		t.Fatalf("IDs = %#v, want [1 2]", dst.IDs)
	}
	if got := dst.When.Format("2006-01-02"); got != "2026-04-09" {
		t.Fatalf("When = %q, want 2026-04-09", got)
	}
	if dst.Custom.value != "x" {
		t.Fatalf("Custom = %#v, want x", dst.Custom)
	}
	if !reflect.DeepEqual(dst.Multi.values, []string{"a", "b"}) {
		t.Fatalf("Multi = %#v, want [a b]", dst.Multi)
	}
	if dst.State != "open" {
		t.Fatalf("State = %q, want open", dst.State)
	}
}

func TestSingleSourcePublicAPIs_PartialUpdatesPersistOnFieldFailure(t *testing.T) {
	t.Run("path preserves earlier writes and leaves later fields untouched", func(t *testing.T) {
		type request struct {
			ID   string `param:"id"`
			Age  int    `param:"age"`
			Name string `param:"name"`
		}

		req := requestWithPathParams(map[string][]string{
			"id":   {"route-id"},
			"age":  {"oops"},
			"name": {"after-error"},
		})

		dst := request{ID: "existing-id", Age: 3, Name: "existing-name"}
		err := BindPathValues(req, &dst)
		_ = assertHTTPError(t, err, http.StatusBadRequest, "bad_request", "Bad Request")
		if dst.ID != "route-id" || dst.Age != 3 || dst.Name != "existing-name" {
			t.Fatalf("dst = %#v, want earlier path writes preserved and later field untouched", dst)
		}
	})

	t.Run("query preserves earlier writes and leaves later fields untouched", func(t *testing.T) {
		type request struct {
			Name string `query:"name"`
			Page int    `query:"page"`
			Note string `query:"note"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?name=kanata&page=oops&note=after-error", nil)

		dst := request{Name: "existing-name", Page: 3, Note: "existing-note"}
		err := BindQueryParams(req, &dst)
		_ = assertHTTPError(t, err, http.StatusBadRequest, "bad_request", "Bad Request")
		if dst.Name != "kanata" || dst.Page != 3 || dst.Note != "existing-note" {
			t.Fatalf("dst = %#v, want earlier query writes preserved and later field untouched", dst)
		}
	})

	t.Run("header preserves earlier writes and leaves later fields untouched", func(t *testing.T) {
		type request struct {
			TraceID string `header:"x-trace-id"`
			Retry   int    `header:"x-retry"`
			Region  string `header:"x-region"`
		}

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Trace-Id", "req-1")
		req.Header.Set("X-Retry", "oops")
		req.Header.Set("X-Region", "after-error")

		dst := request{TraceID: "existing-trace", Retry: 3, Region: "existing-region"}
		err := BindHeaders(req, &dst)
		_ = assertHTTPError(t, err, http.StatusBadRequest, "bad_request", "Bad Request")
		if dst.TraceID != "req-1" || dst.Retry != 3 || dst.Region != "existing-region" {
			t.Fatalf("dst = %#v, want earlier header writes preserved and later field untouched", dst)
		}
	})
}
