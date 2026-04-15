package reqx

import (
	"errors"
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

func TestBind_PublicEntryPointsRejectInvalidInputs(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	type destination struct{}

	var typedNil *destination

	entryPoints := []struct {
		name string
		call func(*http.Request, any) error
	}{
		{name: "BindBody", call: BindBody},
		{name: "BindQuery", call: BindQuery},
	}

	invalidInputs := []struct {
		name   string
		req    *http.Request
		target any
		want   string
	}{
		{name: "rejects nil request", req: nil, target: &destination{}, want: wantNilRequestErr},
		{name: "rejects nil destination", req: req, target: nil, want: wantNilDestinationErr},
		{name: "rejects non-pointer destination", req: req, target: destination{}, want: wantNilDestinationErr},
		{name: "rejects typed nil destination", req: req, target: typedNil, want: wantNilDestinationErr},
	}

	for _, entryPoint := range entryPoints {
		t.Run(entryPoint.name, func(t *testing.T) {
			for _, tc := range invalidInputs {
				t.Run(tc.name, func(t *testing.T) {
					assertUsageError(t, entryPoint.call(tc.req, tc.target), tc.want)
				})
			}
		})
	}
}

func TestBindQuery_NoopsOnUnsupportedTargets(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?page=1", nil)

	scalar := 1
	if err := BindQuery(req, &scalar); err != nil {
		t.Fatalf("BindQuery(scalar) error = %v", err)
	}

	unsupportedMap := map[string]int(nil)
	if err := BindQuery(req, &unsupportedMap); err != nil {
		t.Fatalf("BindQuery(map[string]int) error = %v", err)
	}
	if unsupportedMap != nil {
		t.Fatalf("unsupportedMap = %#v, want nil no-op", unsupportedMap)
	}
}

func TestBindQuery_UsesOnlyQuerySource(t *testing.T) {
	type request struct {
		AccountID string `param:"account_id"`
		Actor     string `header:"x-actor"`
		Name      string `json:"name"`
		Cursor    string `query:"cursor"`
	}

	req := httptest.NewRequest(http.MethodPost, "/?cursor=next", nil)
	req.Header.Set("X-Actor", "kanata")
	req.SetPathValue("account_id", "acct_123")
	req.Pattern = "/accounts/{account_id}"
	setRequestBody(req, mimeApplicationJSON, `{"name":"body-name"}`)

	dst := request{
		AccountID: "existing-account",
		Actor:     "existing-actor",
		Name:      "existing-name",
	}
	mustBindQuery(t, req, &dst)
	if dst.AccountID != "existing-account" || dst.Actor != "existing-actor" || dst.Name != "existing-name" || dst.Cursor != "next" {
		t.Fatalf("dst = %#v, want query field only", dst)
	}
}

func TestBindQuery_BindsSupportedTypes(t *testing.T) {
	type request struct {
		Page    int    `query:"page"`
		Search  string `query:"search"`
		Enabled bool   `query:"enabled"`
	}

	req := httptest.NewRequest(http.MethodGet, "/?page=2&search=kanata&enabled=true", nil)

	var dst request
	mustBindQuery(t, req, &dst)
	if dst.Page != 2 || dst.Search != "kanata" || !dst.Enabled {
		t.Fatalf("dst = %#v, want bound query values", dst)
	}
}

func TestBindQuery_BindsUnsignedAndFloatTypes(t *testing.T) {
	type request struct {
		Attempt uint    `query:"attempt"`
		Limit   uint64  `query:"limit"`
		Ratio   float32 `query:"ratio"`
		Score   float64 `query:"score"`
	}

	req := httptest.NewRequest(http.MethodGet, "/?attempt=3&limit=99&ratio=1.5&score=2.25", nil)

	var dst request
	mustBindQuery(t, req, &dst)
	if dst.Attempt != 3 || dst.Limit != 99 || dst.Ratio != 1.5 || dst.Score != 2.25 {
		t.Fatalf("dst = %#v, want bound unsigned and float values", dst)
	}
}

func TestBindQuery_EmptyUnsignedAndFloatValuesBindZero(t *testing.T) {
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

	mustBindQuery(t, req, &dst)
	if dst.Attempt != 0 || dst.Limit != 0 || dst.Ratio != 0 || dst.Score != 0 {
		t.Fatalf("dst = %#v, want zero values after empty inputs", dst)
	}
}

func TestBindQuery_BindsSupportedMapTargets(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?name=kanata&tag=a&tag=b", nil)

	stringMap := map[string]string(nil)
	mustBindQuery(t, req, &stringMap)
	if got := stringMap["name"]; got != "kanata" {
		t.Fatalf("stringMap[name] = %q, want kanata", got)
	}
	if got := stringMap["tag"]; got != "a" {
		t.Fatalf("stringMap[tag] = %q, want first value a", got)
	}

	sliceMap := map[string][]string(nil)
	mustBindQuery(t, req, &sliceMap)
	if !reflect.DeepEqual(sliceMap["tag"], []string{"a", "b"}) {
		t.Fatalf("sliceMap[tag] = %#v, want [a b]", sliceMap["tag"])
	}

	anyMap := map[string]any(nil)
	mustBindQuery(t, req, &anyMap)
	if got := anyMap["name"]; got != "kanata" {
		t.Fatalf("anyMap[name] = %#v, want kanata", got)
	}
}

func TestBindQuery_MissingParamsPreserveExistingValues(t *testing.T) {
	type request struct {
		Page   int    `query:"page"`
		Search string `query:"search"`
	}

	req := httptest.NewRequest(http.MethodGet, "/?other=1", nil)
	dst := request{Page: 3, Search: "existing"}

	mustBindQuery(t, req, &dst)
	if dst.Page != 3 || dst.Search != "existing" {
		t.Fatalf("dst = %#v, want existing values preserved", dst)
	}
}

func TestBindQuery_RepeatedScalarUsesFirstValue(t *testing.T) {
	type request struct {
		Page int `query:"page"`
	}

	req := httptest.NewRequest(http.MethodGet, "/?page=1&page=2", nil)

	var dst request
	mustBindQuery(t, req, &dst)
	if dst.Page != 1 {
		t.Fatalf("page = %d, want 1", dst.Page)
	}
}

func TestBindQuery_NameMatchingIsCaseSensitive(t *testing.T) {
	type request struct {
		Page int `query:"page"`
	}

	req := httptest.NewRequest(http.MethodGet, "/?PAGE=7", nil)

	dst := request{Page: 3}
	mustBindQuery(t, req, &dst)
	if dst.Page != 3 {
		t.Fatalf("page = %d, want existing value preserved", dst.Page)
	}
}

func TestBindQuery_BindingErrorsAreBadRequest(t *testing.T) {
	type request struct {
		Page int `query:"page"`
	}

	req := httptest.NewRequest(http.MethodGet, "/?page=oops", nil)

	var dst request
	_ = assertBadRequest(t, BindQuery(req, &dst))
}

func TestBindQuery_UnsignedAndFloatBindingErrorsAreBadRequest(t *testing.T) {
	cases := []struct {
		name   string
		target string
		field  any
	}{
		{name: "uint", target: "/?attempt=oops", field: &struct {
			Attempt uint `query:"attempt"`
		}{}},
		{name: "uint64", target: "/?limit=oops", field: &struct {
			Limit uint64 `query:"limit"`
		}{}},
		{name: "float32", target: "/?ratio=oops", field: &struct {
			Ratio float32 `query:"ratio"`
		}{}},
		{name: "float64", target: "/?score=oops", field: &struct {
			Score float64 `query:"score"`
		}{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.target, nil)
			_ = assertBadRequest(t, BindQuery(req, tc.field))
		})
	}
}

func TestBindQuery_EmbeddedFieldContracts(t *testing.T) {
	t.Run("rejects tagged anonymous embedded struct", func(t *testing.T) {
		type Embedded struct {
			Name string
		}
		type request struct {
			Embedded `query:"name"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?name=kanata", nil)

		var dst request
		_ = assertBadRequest(t, BindQuery(req, &dst))
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
		_ = assertBadRequest(t, BindQuery(req, &dst))
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
		mustBindQuery(t, req, &dst)
		if dst.Embedded == nil || dst.Name != "kanata" {
			t.Fatalf("dst = %#v, want embedded name to bind", dst)
		}
	})
}

func TestBindQuery_DecodeFailuresAreBadRequest(t *testing.T) {
	t.Run("nested struct parse failures bubble up", func(t *testing.T) {
		type nested struct {
			Age int `query:"age"`
		}
		type request struct {
			Nested nested
		}

		req := httptest.NewRequest(http.MethodGet, "/?age=oops", nil)

		var dst request
		_ = assertBadRequest(t, BindQuery(req, &dst))
	})

	t.Run("custom single value decoder failures are bad request", func(t *testing.T) {
		type request struct {
			Custom customParamValue `query:"custom"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?custom=x", nil)
		dst := request{Custom: customParamValue{err: errors.New("custom failed")}}

		_ = assertBadRequest(t, BindQuery(req, &dst))
	})

	t.Run("custom multi value decoder failures are bad request", func(t *testing.T) {
		type request struct {
			Multi customParamsValue `query:"multi"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?multi=a&multi=b", nil)
		dst := request{Multi: customParamsValue{err: errors.New("multi failed")}}

		_ = assertBadRequest(t, BindQuery(req, &dst))
	})

	t.Run("invalid time value returns bad request", func(t *testing.T) {
		type request struct {
			When time.Time `query:"when" format:"2006-01-02"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?when=bad", nil)

		var dst request
		_ = assertBadRequest(t, BindQuery(req, &dst))
	})

	t.Run("invalid time slice value returns bad request", func(t *testing.T) {
		type request struct {
			Whens []time.Time `query:"whens" format:"15:04:05"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?whens=10:11:12&whens=bad", nil)

		var dst request
		_ = assertBadRequest(t, BindQuery(req, &dst))
	})

	t.Run("invalid slice element returns bad request", func(t *testing.T) {
		type request struct {
			IDs []int `query:"id"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?id=1&id=oops", nil)

		var dst request
		_ = assertBadRequest(t, BindQuery(req, &dst))
	})
}

func TestBindQuery_BindsPointerSliceFields(t *testing.T) {
	type request struct {
		IDs   []*int       `query:"id"`
		Whens []*time.Time `query:"when" format:"15:04:05"`
	}

	req := httptest.NewRequest(http.MethodGet, "/?id=1&id=2&when=10:11:12&when=13:14:15", nil)

	var dst request
	mustBindQuery(t, req, &dst)
	if len(dst.IDs) != 2 || dst.IDs[0] == nil || *dst.IDs[0] != 1 || dst.IDs[1] == nil || *dst.IDs[1] != 2 {
		t.Fatalf("IDs = %#v, want pointer slice values [1 2]", dst.IDs)
	}
	if len(dst.Whens) != 2 || dst.Whens[0] == nil || dst.Whens[0].Format("15:04:05") != "10:11:12" || dst.Whens[1] == nil || dst.Whens[1].Format("15:04:05") != "13:14:15" {
		t.Fatalf("Whens = %#v, want pointer slice times 10:11:12 and 13:14:15", dst.Whens)
	}
}

func TestBindQuery_PointerFieldPreservation(t *testing.T) {
	type request struct {
		Page *int `query:"page"`
	}

	intPtr := func(v int) *int { return &v }

	t.Run("nil pointer preserved when query param is missing", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/?other=1", nil)

		var dst request
		mustBindQuery(t, req, &dst)
		if dst.Page != nil {
			t.Fatalf("page = %#v, want nil", dst.Page)
		}
	})

	t.Run("existing pointer value is preserved when query param is missing", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/?other=1", nil)

		dst := request{Page: intPtr(7)}
		mustBindQuery(t, req, &dst)
		if dst.Page == nil || *dst.Page != 7 {
			t.Fatalf("page = %#v, want &7", dst.Page)
		}
	})

	t.Run("empty value allocates pointer and sets zero", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/?page=", nil)

		var dst request
		mustBindQuery(t, req, &dst)
		if dst.Page == nil || *dst.Page != 0 {
			t.Fatalf("page = %#v, want &0", dst.Page)
		}
	})

	t.Run("empty value overwrites existing pointer with zero", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/?page=", nil)

		dst := request{Page: intPtr(7)}
		mustBindQuery(t, req, &dst)
		if dst.Page == nil || *dst.Page != 0 {
			t.Fatalf("page = %#v, want &0", dst.Page)
		}
	})
}

func TestBindQuery_PointerFieldFailuresDoNotAllocate(t *testing.T) {
	t.Run("scalar parse failure keeps nil pointer nil", func(t *testing.T) {
		type request struct {
			Page *int `query:"page"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?page=oops", nil)

		var dst request
		_ = assertBadRequest(t, BindQuery(req, &dst))
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
		_ = assertBadRequest(t, BindQuery(req, &dst))
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
		_ = assertBadRequest(t, BindQuery(req, &dst))
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
		_ = assertBadRequest(t, BindQuery(req, &dst))
		if dst.Custom != nil {
			t.Fatalf("Custom = %#v, want nil after failed bind", dst.Custom)
		}
	})
}

func TestBindQuery_BindsComplexTypesEndToEnd(t *testing.T) {
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
	mustBindQuery(t, req, &dst)
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

func TestBindQuery_PartialUpdatesPersistOnFieldFailure(t *testing.T) {
	type request struct {
		Name string `query:"name"`
		Page int    `query:"page"`
		Note string `query:"note"`
	}

	req := httptest.NewRequest(http.MethodGet, "/?name=kanata&page=oops&note=after-error", nil)

	dst := request{Name: "existing-name", Page: 3, Note: "existing-note"}
	err := BindQuery(req, &dst)
	_ = assertBadRequest(t, err)
	if dst.Name != "kanata" || dst.Page != 3 || dst.Note != "existing-note" {
		t.Fatalf("dst = %#v, want earlier query writes preserved and later field untouched", dst)
	}
}
