package bind

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
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

type alwaysFailingParamValue struct{}

func (*alwaysFailingParamValue) UnmarshalParam(string) error {
	return errors.New("custom failed")
}

type alwaysFailingParamsValue struct{}

func (*alwaysFailingParamsValue) UnmarshalParams([]string) error {
	return errors.New("multi failed")
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
	_ = assertBadRequest(t, BindPathValues(req, &dst))
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

func TestBindPathValues_SupportsNetHTTPPatternVariants(t *testing.T) {
	t.Run("method-prefixed typed wildcard binds by declared name", func(t *testing.T) {
		type request struct {
			ID int `param:"id"`
		}

		req := httptest.NewRequest(http.MethodGet, "/accounts/42", nil)
		req.Pattern = "GET /accounts/{id:[0-9]+}"
		req.SetPathValue("id", "42")

		var dst request
		if err := BindPathValues(req, &dst); err != nil {
			t.Fatalf("BindPathValues() error = %v", err)
		}
		if dst.ID != 42 {
			t.Fatalf("id = %d, want 42", dst.ID)
		}
	})

	t.Run("catch-all wildcard binds full path value", func(t *testing.T) {
		type request struct {
			Path string `param:"path"`
		}

		req := httptest.NewRequest(http.MethodGet, "/files/a/b/report.txt", nil)
		req.Pattern = "/files/{path...}"
		req.SetPathValue("path", "a/b/report.txt")

		var dst request
		if err := BindPathValues(req, &dst); err != nil {
			t.Fatalf("BindPathValues() error = %v", err)
		}
		if dst.Path != "a/b/report.txt" {
			t.Fatalf("path = %q, want a/b/report.txt", dst.Path)
		}
	})
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

func TestBindQueryParams_BindsUnsignedAndFloatTypes(t *testing.T) {
	type request struct {
		Attempt uint    `query:"attempt"`
		Limit   uint64  `query:"limit"`
		Ratio   float32 `query:"ratio"`
		Score   float64 `query:"score"`
	}

	req := httptest.NewRequest(http.MethodGet, "/?attempt=2&limit=9&ratio=1.5&score=2.25", nil)

	var dst request
	if err := BindQueryParams(req, &dst); err != nil {
		t.Fatalf("BindQueryParams() error = %v", err)
	}
	if dst.Attempt != 2 || dst.Limit != 9 || dst.Ratio != 1.5 || dst.Score != 2.25 {
		t.Fatalf("dst = %#v, want bound unsigned and float values", dst)
	}
}

func TestBindQueryParams_EmptyUnsignedAndFloatValuesBindZero(t *testing.T) {
	type request struct {
		Attempt uint    `query:"attempt"`
		Limit   uint64  `query:"limit"`
		Ratio   float32 `query:"ratio"`
		Score   float64 `query:"score"`
	}

	req := httptest.NewRequest(http.MethodGet, "/?attempt=&limit=&ratio=&score=", nil)
	dst := request{
		Attempt: 7,
		Limit:   9,
		Ratio:   1.5,
		Score:   2.25,
	}

	if err := BindQueryParams(req, &dst); err != nil {
		t.Fatalf("BindQueryParams() error = %v", err)
	}
	if dst.Attempt != 0 || dst.Limit != 0 || dst.Ratio != 0 || dst.Score != 0 {
		t.Fatalf("dst = %#v, want zero values after empty inputs", dst)
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
	_ = assertBadRequest(t, BindQueryParams(req, &dst))
}

func TestBindQueryParams_UnsignedAndFloatBindingErrorsAreBadRequest(t *testing.T) {
	t.Run("unsigned parse failure returns bad request", func(t *testing.T) {
		type request struct {
			Attempt uint `query:"attempt"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?attempt=-1", nil)

		var dst request
		_ = assertBadRequest(t, BindQueryParams(req, &dst))
	})

	t.Run("float parse failure returns bad request", func(t *testing.T) {
		type request struct {
			Score float64 `query:"score"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?score=oops", nil)

		var dst request
		_ = assertBadRequest(t, BindQueryParams(req, &dst))
	})
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

func TestBindHeaders_DuplicateCaseVariantsMergeDeterministically(t *testing.T) {
	type request struct {
		TraceID string   `header:"x-trace-id"`
		Tags    []string `header:"x-tags"`
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header = http.Header{
		"x-trace-id": {"lower-trace"},
		"X-TRACE-ID": {"upper-trace"},
		"x-tags":     {"lower-tag"},
		"X-TAGS":     {"upper-tag-1", "upper-tag-2"},
	}

	var dst request
	if err := BindHeaders(req, &dst); err != nil {
		t.Fatalf("BindHeaders() error = %v", err)
	}
	if dst.TraceID != "upper-trace" {
		t.Fatalf("trace_id = %q, want upper-trace from deterministic normalized order", dst.TraceID)
	}
	if !reflect.DeepEqual(dst.Tags, []string{"upper-tag-1", "upper-tag-2", "lower-tag"}) {
		t.Fatalf("tags = %#v, want merged deterministic order", dst.Tags)
	}
}

func TestBindHeaders_BindingErrorsAreBadRequest(t *testing.T) {
	type request struct {
		Retry int `header:"x-retry"`
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Retry", "oops")

	var dst request
	_ = assertBadRequest(t, BindHeaders(req, &dst))
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

func TestBindQueryParams_EmbeddedFieldContracts(t *testing.T) {
	t.Run("rejects tagged anonymous embedded struct", func(t *testing.T) {
		type Embedded struct {
			Name string
		}
		type request struct {
			Embedded `query:"name"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?name=kanata", nil)

		var dst request
		_ = assertBadRequest(t, BindQueryParams(req, &dst))
	})

	t.Run("rejects tagged anonymous embedded pointer even when nil", func(t *testing.T) {
		type Embedded struct {
			Name string
		}
		type request struct {
			*Embedded `query:"name"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?name=kanata", nil)

		var dst request
		_ = assertBadRequest(t, BindQueryParams(req, &dst))
	})

	t.Run("traverses non nil anonymous embedded pointer", func(t *testing.T) {
		type Embedded struct {
			Name string `query:"name"`
		}
		type request struct {
			*Embedded
		}

		req := httptest.NewRequest(http.MethodGet, "/?name=kanata", nil)
		dst := request{Embedded: &Embedded{}}
		if err := BindQueryParams(req, &dst); err != nil {
			t.Fatalf("BindQueryParams() error = %v", err)
		}
		if dst.Embedded == nil || dst.Name != "kanata" {
			t.Fatalf("dst = %#v, want embedded name to bind", dst)
		}
	})
}

func TestBindQueryParams_DecodeFailuresAreBadRequest(t *testing.T) {
	t.Run("nested struct parse failures bubble up", func(t *testing.T) {
		type nested struct {
			Age int `query:"age"`
		}
		type request struct {
			Nested nested
		}

		req := httptest.NewRequest(http.MethodGet, "/?age=oops", nil)

		var dst request
		_ = assertBadRequest(t, BindQueryParams(req, &dst))
	})

	t.Run("custom single value decoder failures are bad request", func(t *testing.T) {
		type request struct {
			Custom customParamValue `query:"custom"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?custom=x", nil)
		dst := request{Custom: customParamValue{err: errors.New("custom failed")}}

		_ = assertBadRequest(t, BindQueryParams(req, &dst))
	})

	t.Run("custom multi value decoder failures are bad request", func(t *testing.T) {
		type request struct {
			Multi customParamsValue `query:"multi"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?multi=a&multi=b", nil)
		dst := request{Multi: customParamsValue{err: errors.New("multi failed")}}

		_ = assertBadRequest(t, BindQueryParams(req, &dst))
	})

	t.Run("invalid time value returns bad request", func(t *testing.T) {
		type request struct {
			When time.Time `query:"when" format:"2006-01-02"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?when=bad", nil)

		var dst request
		_ = assertBadRequest(t, BindQueryParams(req, &dst))
	})

	t.Run("invalid time slice value returns bad request", func(t *testing.T) {
		type request struct {
			Whens []time.Time `query:"whens" format:"15:04:05"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?whens=10:11:12&whens=bad", nil)

		var dst request
		_ = assertBadRequest(t, BindQueryParams(req, &dst))
	})

	t.Run("invalid slice element returns bad request", func(t *testing.T) {
		type request struct {
			IDs []int `query:"id"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?id=1&id=oops", nil)

		var dst request
		_ = assertBadRequest(t, BindQueryParams(req, &dst))
	})
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

func TestBindQueryParams_BindsPointerSliceFields(t *testing.T) {
	type request struct {
		IDs   []*int       `query:"id"`
		Whens []*time.Time `query:"when" format:"15:04:05"`
	}

	req := httptest.NewRequest(http.MethodGet, "/?id=1&id=2&when=10:11:12&when=13:14:15", nil)

	var dst request
	if err := BindQueryParams(req, &dst); err != nil {
		t.Fatalf("BindQueryParams() error = %v", err)
	}
	if len(dst.IDs) != 2 || dst.IDs[0] == nil || *dst.IDs[0] != 1 || dst.IDs[1] == nil || *dst.IDs[1] != 2 {
		t.Fatalf("IDs = %#v, want pointer slice values [1 2]", dst.IDs)
	}
	if len(dst.Whens) != 2 || dst.Whens[0] == nil || dst.Whens[0].Format("15:04:05") != "10:11:12" || dst.Whens[1] == nil || dst.Whens[1].Format("15:04:05") != "13:14:15" {
		t.Fatalf("Whens = %#v, want pointer slice times 10:11:12 and 13:14:15", dst.Whens)
	}
}

func TestBindQueryParams_PointerFieldPreservation(t *testing.T) {
	type request struct {
		Page *int `query:"page"`
	}

	intPtr := func(v int) *int { return &v }

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

	t.Run("existing pointer value is preserved when query param is missing", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/?other=1", nil)

		dst := request{Page: intPtr(7)}
		if err := BindQueryParams(req, &dst); err != nil {
			t.Fatalf("BindQueryParams() error = %v", err)
		}
		if dst.Page == nil || *dst.Page != 7 {
			t.Fatalf("page = %#v, want &7", dst.Page)
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

	t.Run("empty value overwrites existing pointer with zero", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/?page=", nil)

		dst := request{Page: intPtr(7)}
		if err := BindQueryParams(req, &dst); err != nil {
			t.Fatalf("BindQueryParams() error = %v", err)
		}
		if dst.Page == nil || *dst.Page != 0 {
			t.Fatalf("page = %#v, want &0", dst.Page)
		}
	})
}

func TestBindQueryParams_PointerFieldFailuresDoNotAllocate(t *testing.T) {
	t.Run("scalar parse failure keeps nil pointer nil", func(t *testing.T) {
		type request struct {
			Page *int `query:"page"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?page=oops", nil)

		var dst request
		_ = assertBadRequest(t, BindQueryParams(req, &dst))
		if dst.Page != nil {
			t.Fatalf("Page = %#v, want nil after failed bind", dst.Page)
		}
	})

	t.Run("time parse failure keeps nil pointer nil", func(t *testing.T) {
		type request struct {
			When *time.Time `query:"when" format:"2006-01-02"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?when=bad", nil)

		var dst request
		_ = assertBadRequest(t, BindQueryParams(req, &dst))
		if dst.When != nil {
			t.Fatalf("When = %#v, want nil after failed bind", dst.When)
		}
	})

	t.Run("custom single-value failure keeps nil pointer nil", func(t *testing.T) {
		type request struct {
			Custom *alwaysFailingParamValue `query:"custom"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?custom=x", nil)

		var dst request
		_ = assertBadRequest(t, BindQueryParams(req, &dst))
		if dst.Custom != nil {
			t.Fatalf("Custom = %#v, want nil after failed bind", dst.Custom)
		}
	})

	t.Run("custom multi-value failure keeps nil pointer nil", func(t *testing.T) {
		type request struct {
			Custom *alwaysFailingParamsValue `query:"custom"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?custom=a&custom=b", nil)

		var dst request
		_ = assertBadRequest(t, BindQueryParams(req, &dst))
		if dst.Custom != nil {
			t.Fatalf("Custom = %#v, want nil after failed bind", dst.Custom)
		}
	})
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
		Whens  []time.Time       `query:"whens" format:"15:04:05"`
		Custom customParamValue  `query:"custom"`
		Multi  customParamsValue `query:"multi"`
		State  customTextValue   `query:"state"`
	}

	req := httptest.NewRequest(
		http.MethodGet,
		"/?name=kanata&age=17&id=1&id=2&when=2026-04-09&whens=10:11:12&whens=13:14:15&custom=x&multi=a&multi=b&state=open",
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
		_ = assertBadRequest(t, err)
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
		_ = assertBadRequest(t, err)
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
		_ = assertBadRequest(t, err)
		if dst.TraceID != "req-1" || dst.Retry != 3 || dst.Region != "existing-region" {
			t.Fatalf("dst = %#v, want earlier header writes preserved and later field untouched", dst)
		}
	})
}
