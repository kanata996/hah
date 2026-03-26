package reqx

import (
	"encoding"
	"net/url"
	"reflect"
	"testing"
	"time"
)

type queryModeInternal string

var _ encoding.TextUnmarshaler = (*queryModeInternal)(nil)

func (m *queryModeInternal) UnmarshalText(text []byte) error {
	switch value := queryModeInternal(text); value {
	case "basic", "full":
		*m = value
		return nil
	default:
		return simpleError("invalid mode")
	}
}

func TestQueryFieldName(t *testing.T) {
	tests := []struct {
		name  string
		field reflect.StructField
		want  string
		ok    bool
	}{
		{
			name:  "missing",
			field: reflect.TypeOf(struct{ Value string }{}).Field(0),
			want:  "",
			ok:    false,
		},
		{
			name: "dash",
			field: reflect.TypeOf(struct {
				Value string `query:"-"`
			}{}).Field(0),
			want: "",
			ok:   false,
		},
		{
			name: "named",
			field: reflect.TypeOf(struct {
				Value string `query:"value"`
			}{}).Field(0),
			want: "value",
			ok:   true,
		},
		{
			name: "with option",
			field: reflect.TypeOf(struct {
				Value string `query:"value,omitempty"`
			}{}).Field(0),
			want: "value",
			ok:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := queryFieldName(tt.field)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("queryFieldName() = (%q, %t), want (%q, %t)", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestSupportsTextUnmarshaler(t *testing.T) {
	tests := []struct {
		name string
		typ  reflect.Type
		want bool
	}{
		{name: "nil", typ: nil, want: false},
		{name: "time", typ: reflect.TypeOf(time.Time{}), want: true},
		{name: "time pointer", typ: reflect.TypeOf((*time.Time)(nil)), want: true},
		{name: "string", typ: reflect.TypeOf(""), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := supportsTextUnmarshaler(tt.typ); got != tt.want {
				t.Fatalf("supportsTextUnmarshaler() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestValidateQueryFieldType(t *testing.T) {
	tests := []struct {
		name      string
		typ       reflect.Type
		fieldName string
		wantErr   string
	}{
		{
			name:      "pointer recursion",
			typ:       reflect.TypeOf((*int)(nil)),
			fieldName: "Limit",
		},
		{
			name:      "slice recursion",
			typ:       reflect.TypeOf([]uint{}),
			fieldName: "IDs",
		},
		{
			name:      "float supported",
			typ:       reflect.TypeOf(float64(0)),
			fieldName: "Ratio",
		},
		{
			name:      "text unmarshaler supported",
			typ:       reflect.TypeOf(queryModeInternal("")),
			fieldName: "Mode",
		},
		{
			name:      "unsupported map",
			typ:       reflect.TypeOf(map[string]string{}),
			fieldName: "Meta",
			wantErr:   `hah/reqx: field "Meta" has unsupported query type map[string]string`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateQueryFieldType(tt.typ, tt.fieldName)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateQueryFieldType() error = %v, want nil", err)
				}
				return
			}
			if err == nil || err.Error() != tt.wantErr {
				t.Fatalf("validateQueryFieldType() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestDecodeQueryIntoReturnsDecodeError(t *testing.T) {
	dst := struct {
		Value chan int
	}{}

	violations, err := decodeQueryInto(
		reflect.ValueOf(&dst).Elem(),
		url.Values{"value": []string{"x"}},
		&queryDecodePlan{
			fields:      []queryFieldSpec{{index: 0, name: "value"}},
			knownFields: map[string]struct{}{"value": {}},
		},
		queryConfig{},
	)
	if err == nil || err.Error() != "hah/reqx: unsupported query destination type chan int" {
		t.Fatalf("decodeQueryInto() error = %v, want unsupported query destination type", err)
	}
	if violations != nil {
		t.Fatalf("violations = %#v, want nil", violations)
	}
}

func TestDecodeQueryIntoHandlesNilPlan(t *testing.T) {
	dst := struct{}{}

	violations, err := decodeQueryInto(
		reflect.ValueOf(&dst).Elem(),
		url.Values{"extra": []string{"x"}},
		nil,
		queryConfig{},
	)
	if err != nil {
		t.Fatalf("decodeQueryInto() error = %v, want nil", err)
	}
	if len(violations) != 1 {
		t.Fatalf("violations len = %d, want 1", len(violations))
	}
	if violations[0] != (Violation{
		Field:   "extra",
		Code:    ViolationCodeUnknown,
		Message: "unknown field",
	}) {
		t.Fatalf("violation = %#v, want unknown extra field", violations[0])
	}
}

func TestLoadQueryDecodePlanCachesByType(t *testing.T) {
	typ := reflect.TypeOf(struct {
		ID   string `query:"id"`
		Page int    `query:"page"`
	}{})

	first, err := loadQueryDecodePlan(typ)
	if err != nil {
		t.Fatalf("loadQueryDecodePlan() error = %v, want nil", err)
	}
	second, err := loadQueryDecodePlan(typ)
	if err != nil {
		t.Fatalf("loadQueryDecodePlan() second error = %v, want nil", err)
	}

	if first == nil {
		t.Fatal("loadQueryDecodePlan() = nil, want non-nil plan")
	}
	if first != second {
		t.Fatalf("cached plan pointers differ: first=%p second=%p", first, second)
	}
	if len(first.fields) != 2 {
		t.Fatalf("plan.fields len = %d, want 2", len(first.fields))
	}
	if _, ok := first.knownFields["id"]; !ok {
		t.Fatalf("knownFields missing id: %#v", first.knownFields)
	}
	if _, ok := first.knownFields["page"]; !ok {
		t.Fatalf("knownFields missing page: %#v", first.knownFields)
	}
}

func TestDecodeQueryFieldBranches(t *testing.T) {
	t.Run("empty values", func(t *testing.T) {
		value := reflect.New(reflect.TypeOf("")).Elem()
		violation, err := decodeQueryField(value, nil, "name")
		if err != nil {
			t.Fatalf("decodeQueryField() error = %v, want nil", err)
		}
		if violation != nil {
			t.Fatalf("violation = %#v, want nil", violation)
		}
	})

	t.Run("pointer violation", func(t *testing.T) {
		value := reflect.New(reflect.TypeOf((*int)(nil)).Elem())
		ptr := reflect.New(value.Type())
		violation, err := decodeQueryField(ptr.Elem(), []string{"bad"}, "limit")
		if err != nil {
			t.Fatalf("decodeQueryField() error = %v, want nil", err)
		}
		if violation == nil || violation.Code != "type" {
			t.Fatalf("violation = %#v, want type violation", violation)
		}
		if !ptr.Elem().IsNil() {
			t.Fatalf("pointer field = %#v, want nil", ptr.Elem().Interface())
		}
	})

	t.Run("slice success", func(t *testing.T) {
		value := reflect.New(reflect.TypeOf([]uint{})).Elem()
		violation, err := decodeQueryField(value, []string{"1", "2"}, "ids")
		if err != nil {
			t.Fatalf("decodeQueryField() error = %v, want nil", err)
		}
		if violation != nil {
			t.Fatalf("violation = %#v, want nil", violation)
		}
		if got := value.Interface().([]uint); len(got) != 2 || got[0] != 1 || got[1] != 2 {
			t.Fatalf("slice = %#v, want []uint{1, 2}", got)
		}
	})

	t.Run("slice violation", func(t *testing.T) {
		value := reflect.New(reflect.TypeOf([]uint{})).Elem()
		violation, err := decodeQueryField(value, []string{"1", "bad"}, "ids")
		if err != nil {
			t.Fatalf("decodeQueryField() error = %v, want nil", err)
		}
		if violation == nil || violation.Code != "type" {
			t.Fatalf("violation = %#v, want type violation", violation)
		}
	})

	t.Run("text unmarshaler repeated", func(t *testing.T) {
		value := reflect.New(reflect.TypeOf(queryModeInternal(""))).Elem()
		violation, err := decodeQueryField(value, []string{"basic", "full"}, "mode")
		if err != nil {
			t.Fatalf("decodeQueryField() error = %v, want nil", err)
		}
		if violation == nil || violation.Code != "multiple" {
			t.Fatalf("violation = %#v, want multiple violation", violation)
		}
	})
}

func TestDecodeScalarValue(t *testing.T) {
	t.Run("bool invalid", func(t *testing.T) {
		value := reflect.New(reflect.TypeOf(true)).Elem()
		violation, err := decodeScalarValue(value, "nope", "active")
		if err != nil {
			t.Fatalf("decodeScalarValue() error = %v, want nil", err)
		}
		if violation == nil || violation.Code != "type" {
			t.Fatalf("violation = %#v, want type violation", violation)
		}
	})

	t.Run("uint success and invalid", func(t *testing.T) {
		value := reflect.New(reflect.TypeOf(uint(0))).Elem()
		violation, err := decodeScalarValue(value, "42", "limit")
		if err != nil || violation != nil {
			t.Fatalf("decodeScalarValue() = (%#v, %v), want (nil, nil)", violation, err)
		}
		if got := value.Uint(); got != 42 {
			t.Fatalf("value = %d, want 42", got)
		}

		violation, err = decodeScalarValue(value, "-1", "limit")
		if err != nil {
			t.Fatalf("decodeScalarValue() error = %v, want nil", err)
		}
		if violation == nil || violation.Code != "type" {
			t.Fatalf("violation = %#v, want type violation", violation)
		}
	})

	t.Run("float success and invalid", func(t *testing.T) {
		value := reflect.New(reflect.TypeOf(float64(0))).Elem()
		violation, err := decodeScalarValue(value, "3.14", "ratio")
		if err != nil || violation != nil {
			t.Fatalf("decodeScalarValue() = (%#v, %v), want (nil, nil)", violation, err)
		}
		if got := value.Float(); got != 3.14 {
			t.Fatalf("value = %f, want 3.14", got)
		}

		violation, err = decodeScalarValue(value, "bad", "ratio")
		if err != nil {
			t.Fatalf("decodeScalarValue() error = %v, want nil", err)
		}
		if violation == nil || violation.Code != "type" {
			t.Fatalf("violation = %#v, want type violation", violation)
		}
	})

	t.Run("unsupported destination", func(t *testing.T) {
		value := reflect.ValueOf(make(chan int))
		violation, err := decodeScalarValue(value, "x", "value")
		if err == nil || err.Error() != "hah/reqx: unsupported query destination type chan int" {
			t.Fatalf("decodeScalarValue() error = %v, want unsupported destination error", err)
		}
		if violation != nil {
			t.Fatalf("violation = %#v, want nil", violation)
		}
	})
}

func TestDecodeTextValue(t *testing.T) {
	t.Run("addressable value", func(t *testing.T) {
		value := reflect.New(reflect.TypeOf(queryModeInternal(""))).Elem()
		violation, err := decodeTextValue(value, "full", "mode")
		if err != nil || violation != nil {
			t.Fatalf("decodeTextValue() = (%#v, %v), want (nil, nil)", violation, err)
		}
		if got := value.Interface().(queryModeInternal); got != "full" {
			t.Fatalf("value = %q, want full", got)
		}
	})

	t.Run("unsupported destination", func(t *testing.T) {
		value := reflect.New(reflect.TypeOf("")).Elem()
		violation, err := decodeTextValue(value, "full", "mode")
		if err == nil || err.Error() != "hah/reqx: unsupported text unmarshaler destination type string" {
			t.Fatalf("decodeTextValue() error = %v, want unsupported destination error", err)
		}
		if violation != nil {
			t.Fatalf("violation = %#v, want nil", violation)
		}
	})
}
