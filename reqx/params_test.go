package reqx

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/kanata996/hah/errx"
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
	pathReq := requestWithPathParams(map[string][]string{
		"id": {"oops"},
	})
	queryReq := httptest.NewRequest(http.MethodGet, "/accounts?page=oops", nil)

	if _, err := PathParam[int](pathReq, "id"); err == nil {
		t.Fatal("PathParam[int](invalid) = nil, want error")
	} else {
		_ = assertHTTPError(t, err, http.StatusBadRequest, "bad_request", "Bad Request")
	}

	if _, err := QueryParam[int](queryReq, "page"); err == nil {
		t.Fatal("QueryParam[int](invalid) = nil, want error")
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

func TestBadRequestWrap_PreservesHTTPErrorAndNil(t *testing.T) {
	if err := badRequestWrap(nil); err != nil {
		t.Fatalf("badRequestWrap(nil) = %v, want nil", err)
	}

	want := errx.BadRequest("bad_request", "bad request")
	if got := badRequestWrap(want); got != want {
		t.Fatalf("badRequestWrap(httpErr) = %#v, want same pointer %#v", got, want)
	}

	plain := errors.New("parse error")
	wrapped := badRequestWrap(plain)
	_ = assertHTTPError(t, wrapped, http.StatusBadRequest, "bad_request", "Bad Request")
}

func TestPathParamValues_Branches(t *testing.T) {
	if values, ok := pathParamValues(nil, "id"); ok || values != nil {
		t.Fatalf("pathParamValues(nil) = (%v, %v), want (nil, false)", values, ok)
	}

	reqWithValue := requestWithPathParams(map[string][]string{
		"id": {"u_1"},
	})
	if values, ok := pathParamValues(reqWithValue, "id"); !ok || len(values) != 1 || values[0] != "u_1" {
		t.Fatalf("pathParamValues(value) = (%v, %v), want ([u_1], true)", values, ok)
	}

	reqWithEmpty := requestWithPathParams(map[string][]string{
		"id": {""},
	})
	if values, ok := pathParamValues(reqWithEmpty, "id"); !ok || len(values) != 1 || values[0] != "" {
		t.Fatalf("pathParamValues(empty) = (%v, %v), want ([\"\"], true)", values, ok)
	}

	reqMissing := httptest.NewRequest(http.MethodGet, "/accounts", nil)
	reqMissing.Pattern = "/accounts"
	if values, ok := pathParamValues(reqMissing, "id"); ok || values != nil {
		t.Fatalf("pathParamValues(missing) = (%v, %v), want (nil, false)", values, ok)
	}
}

func TestQueryParamValues_NilRequest(t *testing.T) {
	if values, ok := queryParamValues(nil, "page"); ok || values != nil {
		t.Fatalf("queryParamValues(nil) = (%v, %v), want (nil, false)", values, ok)
	}
}

func TestBindParamValues_UnknownTypeReturnsError(t *testing.T) {
	type unsupported struct {
		Name string
	}

	var dst unsupported
	err := bindParamValues(reflect.ValueOf(&dst).Elem(), []string{"x"})
	if err == nil || err.Error() != "unknown type" {
		t.Fatalf("bindParamValues(unsupported) error = %v, want unknown type", err)
	}
}

func TestUnmarshalInputsToField_NoOpForNonMatchingPointer(t *testing.T) {
	var dst *int
	ok, err := unmarshalInputsToField(reflect.Pointer, []string{"1"}, reflect.ValueOf(&dst).Elem())
	if ok || err != nil {
		t.Fatalf("unmarshalInputsToField(non-matching pointer) = (%v, %v), want (false, nil)", ok, err)
	}
}

func TestUnmarshalInputToField_NoOpForNonMatchingPointer(t *testing.T) {
	var dst *int
	ok, err := unmarshalInputToField(reflect.Pointer, "1", reflect.ValueOf(&dst).Elem())
	if ok || err != nil {
		t.Fatalf("unmarshalInputToField(non-matching pointer) = (%v, %v), want (false, nil)", ok, err)
	}
}

func TestSetWithProperType_SupportsScalarKinds(t *testing.T) {
	t.Run("uint", func(t *testing.T) {
		var dst uint
		if err := setWithProperType(reflect.ValueOf(&dst).Elem(), "7"); err != nil {
			t.Fatalf("setWithProperType(uint) error = %v", err)
		}
		if dst != 7 {
			t.Fatalf("uint = %d, want 7", dst)
		}
	})

	t.Run("bool", func(t *testing.T) {
		var dst bool
		if err := setWithProperType(reflect.ValueOf(&dst).Elem(), "true"); err != nil {
			t.Fatalf("setWithProperType(bool) error = %v", err)
		}
		if !dst {
			t.Fatal("bool = false, want true")
		}
	})

	t.Run("float", func(t *testing.T) {
		var dst float64
		if err := setWithProperType(reflect.ValueOf(&dst).Elem(), "1.5"); err != nil {
			t.Fatalf("setWithProperType(float64) error = %v", err)
		}
		if dst != 1.5 {
			t.Fatalf("float = %v, want 1.5", dst)
		}
	})
}

func TestSetWithProperType_UnknownTypeReturnsError(t *testing.T) {
	type unsupported struct{}
	var dst unsupported

	err := setWithProperType(reflect.ValueOf(&dst).Elem(), "x")
	if err == nil || err.Error() != "unknown type" {
		t.Fatalf("setWithProperType(unsupported) error = %v, want unknown type", err)
	}
}

func TestConcreteFieldValue_NonPointerPreservesField(t *testing.T) {
	field := reflect.ValueOf(new(int)).Elem()
	got, kind := concreteFieldValue(field, reflect.Int)
	if got.Kind() != reflect.Int || kind != reflect.Int {
		t.Fatalf("concreteFieldValue(non-pointer) = (%v, %v), want (int, int)", got.Kind(), kind)
	}
}

func TestScalarParsers_DefaultEmptyAndErrors(t *testing.T) {
	t.Run("setIntField invalid", func(t *testing.T) {
		var dst int
		if err := setIntField("oops", 0, reflect.ValueOf(&dst).Elem()); err == nil {
			t.Fatal("setIntField(invalid) = nil, want error")
		}
	})

	t.Run("setUintField empty becomes zero", func(t *testing.T) {
		dst := uint(9)
		if err := setUintField("", 0, reflect.ValueOf(&dst).Elem()); err != nil {
			t.Fatalf("setUintField(empty) error = %v", err)
		}
		if dst != 0 {
			t.Fatalf("uint = %d, want 0", dst)
		}
	})

	t.Run("setBoolField empty becomes false", func(t *testing.T) {
		dst := true
		if err := setBoolField("", reflect.ValueOf(&dst).Elem()); err != nil {
			t.Fatalf("setBoolField(empty) error = %v", err)
		}
		if dst {
			t.Fatal("bool = true, want false")
		}
	})

	t.Run("setFloatField empty becomes zero", func(t *testing.T) {
		dst := 9.9
		if err := setFloatField("", 64, reflect.ValueOf(&dst).Elem()); err != nil {
			t.Fatalf("setFloatField(empty) error = %v", err)
		}
		if dst != 0 {
			t.Fatalf("float = %v, want 0", dst)
		}
	})
}

func TestPathWildcardNames_Branches(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		want    []string
	}{
		{name: "blank", pattern: "   ", want: nil},
		{name: "no wildcard", pattern: "/accounts", want: []string{}},
		{name: "basic", pattern: "/accounts/{id}", want: []string{"id"}},
		{name: "with method prefix", pattern: "GET /accounts/{id}", want: []string{"id"}},
		{name: "catch all", pattern: "/files/{path...}", want: []string{"path"}},
		{name: "typed wildcard", pattern: "/accounts/{id:[0-9]+}", want: []string{"id"}},
		{name: "skip dollar", pattern: "/{$}", want: []string{}},
		{name: "malformed", pattern: "/accounts/{id", want: []string{}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := pathWildcardNames(tc.pattern); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("pathWildcardNames(%q) = %#v, want %#v", tc.pattern, got, tc.want)
			}
		})
	}
}

func TestBindParamValues_PropagatesMultipleUnmarshallerError(t *testing.T) {
	wantErr := errors.New("boom")
	dst := customParamsValue{err: wantErr}
	err := bindParamValues(reflect.ValueOf(&dst).Elem(), []string{"a", "b"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("bindParamValues(custom multiple) = %v, want %v", err, wantErr)
	}
}

func TestBindParamValues_PropagatesSingleUnmarshallerError(t *testing.T) {
	wantErr := errors.New("boom")
	dst := customParamValue{err: wantErr}
	err := bindParamValues(reflect.ValueOf(&dst).Elem(), []string{"a"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("bindParamValues(custom single) = %v, want %v", err, wantErr)
	}
}

func TestBindParamValues_SliceElementError(t *testing.T) {
	var dst []int
	err := bindParamValues(reflect.ValueOf(&dst).Elem(), []string{"1", "oops"})
	if err == nil {
		t.Fatal("bindParamValues(slice invalid) = nil, want error")
	}
}

func TestSetWithProperType_CoversAllScalarKinds(t *testing.T) {
	t.Run("signed ints", func(t *testing.T) {
		var (
			i   int
			i8  int8
			i16 int16
			i32 int32
			i64 int64
		)
		cases := []struct {
			name  string
			field reflect.Value
			value string
		}{
			{name: "int", field: reflect.ValueOf(&i).Elem(), value: "1"},
			{name: "int8", field: reflect.ValueOf(&i8).Elem(), value: "2"},
			{name: "int16", field: reflect.ValueOf(&i16).Elem(), value: "3"},
			{name: "int32", field: reflect.ValueOf(&i32).Elem(), value: "4"},
			{name: "int64", field: reflect.ValueOf(&i64).Elem(), value: "5"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				if err := setWithProperType(tc.field, tc.value); err != nil {
					t.Fatalf("setWithProperType(%s) error = %v", tc.name, err)
				}
				if got := fmt.Sprint(tc.field.Interface()); got != tc.value {
					t.Fatalf("setWithProperType(%s) value = %s, want %s", tc.name, got, tc.value)
				}
			})
		}
	})

	t.Run("unsigned ints", func(t *testing.T) {
		var (
			u   uint
			u8  uint8
			u16 uint16
			u32 uint32
			u64 uint64
		)
		cases := []struct {
			name  string
			field reflect.Value
			value string
		}{
			{name: "uint", field: reflect.ValueOf(&u).Elem(), value: "1"},
			{name: "uint8", field: reflect.ValueOf(&u8).Elem(), value: "2"},
			{name: "uint16", field: reflect.ValueOf(&u16).Elem(), value: "3"},
			{name: "uint32", field: reflect.ValueOf(&u32).Elem(), value: "4"},
			{name: "uint64", field: reflect.ValueOf(&u64).Elem(), value: "5"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				if err := setWithProperType(tc.field, tc.value); err != nil {
					t.Fatalf("setWithProperType(%s) error = %v", tc.name, err)
				}
				if got := fmt.Sprint(tc.field.Interface()); got != tc.value {
					t.Fatalf("setWithProperType(%s) value = %s, want %s", tc.name, got, tc.value)
				}
			})
		}
	})

	t.Run("floats", func(t *testing.T) {
		var (
			f32 float32
			f64 float64
		)
		if err := setWithProperType(reflect.ValueOf(&f32).Elem(), "1.25"); err != nil {
			t.Fatalf("setWithProperType(float32) error = %v", err)
		}
		if f32 != 1.25 {
			t.Fatalf("float32 = %v, want 1.25", f32)
		}
		if err := setWithProperType(reflect.ValueOf(&f64).Elem(), "2.5"); err != nil {
			t.Fatalf("setWithProperType(float64) error = %v", err)
		}
		if f64 != 2.5 {
			t.Fatalf("float64 = %v, want 2.5", f64)
		}
	})
}

func TestSetWithProperType_PointerTargets(t *testing.T) {
	t.Run("allocates nil pointer", func(t *testing.T) {
		var dst *int
		if err := setWithProperType(reflect.ValueOf(&dst).Elem(), "7"); err != nil {
			t.Fatalf("setWithProperType(*int nil) error = %v", err)
		}
		if dst == nil || *dst != 7 {
			t.Fatalf("dst = %#v, want pointer to 7", dst)
		}
	})

	t.Run("reuses non nil pointer", func(t *testing.T) {
		value := 1
		dst := &value
		if err := setWithProperType(reflect.ValueOf(&dst).Elem(), "9"); err != nil {
			t.Fatalf("setWithProperType(*int non-nil) error = %v", err)
		}
		if dst == nil || *dst != 9 {
			t.Fatalf("dst = %#v, want pointer to 9", dst)
		}
	})
}

func TestSetWithProperType_UsesUnmarshallerDirectly(t *testing.T) {
	var dst customParamValue
	if err := setWithProperType(reflect.ValueOf(&dst).Elem(), "u_1"); err != nil {
		t.Fatalf("setWithProperType(custom unmarshaler) error = %v", err)
	}
	if dst.value != "u_1" {
		t.Fatalf("dst = %#v, want value u_1", dst)
	}
}

func TestSetIntField_EmptyBecomesZero(t *testing.T) {
	dst := 9
	if err := setIntField("", 0, reflect.ValueOf(&dst).Elem()); err != nil {
		t.Fatalf("setIntField(empty) error = %v", err)
	}
	if dst != 0 {
		t.Fatalf("int = %d, want 0", dst)
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

func TestQueryParamValues_Branches(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/items?page=5&tag=a&tag=b", nil)

	t.Run("single value", func(t *testing.T) {
		values, ok := queryParamValues(req, "page")
		if !ok || len(values) != 1 || values[0] != "5" {
			t.Fatalf("queryParamValues(page) = (%v, %v), want ([5], true)", values, ok)
		}
	})

	t.Run("multiple values", func(t *testing.T) {
		values, ok := queryParamValues(req, "tag")
		if !ok || len(values) != 2 || values[0] != "a" || values[1] != "b" {
			t.Fatalf("queryParamValues(tag) = (%v, %v), want ([a b], true)", values, ok)
		}
	})

	t.Run("missing key", func(t *testing.T) {
		values, ok := queryParamValues(req, "missing")
		if ok || values != nil {
			t.Fatalf("queryParamValues(missing) = (%v, %v), want (nil, false)", values, ok)
		}
	})

	t.Run("empty value", func(t *testing.T) {
		emptyReq := httptest.NewRequest(http.MethodGet, "/items?page=", nil)
		values, ok := queryParamValues(emptyReq, "page")
		if !ok || len(values) != 1 || values[0] != "" {
			t.Fatalf("queryParamValues(empty) = (%v, %v), want ([\"\"], true)", values, ok)
		}
	})
}

func TestScalarParsers_InvalidInputReturnsError(t *testing.T) {
	t.Run("setUintField invalid", func(t *testing.T) {
		var dst uint
		if err := setUintField("oops", 0, reflect.ValueOf(&dst).Elem()); err == nil {
			t.Fatal("setUintField(invalid) = nil, want error")
		}
	})

	t.Run("setBoolField invalid", func(t *testing.T) {
		var dst bool
		if err := setBoolField("oops", reflect.ValueOf(&dst).Elem()); err == nil {
			t.Fatal("setBoolField(invalid) = nil, want error")
		}
	})

	t.Run("setFloatField invalid", func(t *testing.T) {
		var dst float64
		if err := setFloatField("oops", 64, reflect.ValueOf(&dst).Elem()); err == nil {
			t.Fatal("setFloatField(invalid) = nil, want error")
		}
	})
}

func mustParseURL(raw string) *url.URL {
	parsed, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	return parsed
}
