package reqx

import (
	"encoding"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
)

var textUnmarshalerType = reflect.TypeFor[encoding.TextUnmarshaler]()
var queryDecodePlanCache sync.Map

type queryConfig struct {
	allowUnknownFields bool
}

// QueryOption customizes URL query decoding behavior.
type QueryOption func(*queryConfig)

// AllowUnknownQueryFields disables strict unknown-field rejection for query
// parameters.
func AllowUnknownQueryFields() QueryOption {
	return func(cfg *queryConfig) {
		cfg.allowUnknownFields = true
	}
}

// DecodeQuery decodes URL query parameters into `query`-tagged struct fields in
// dst and returns reqx Problems for request-shape failures.
//
// Supported field types are:
//
//   - string, bool, integer, unsigned integer, and float fields
//   - pointers to supported scalar or slice fields
//   - slices of supported scalar fields
//   - types implementing encoding.TextUnmarshaler
func DecodeQuery[T any](r *http.Request, dst *T, opts ...QueryOption) error {
	if r == nil {
		return fmt.Errorf("hah/reqx: request must not be nil")
	}
	if dst == nil {
		return fmt.Errorf("hah/reqx: destination must not be nil")
	}

	cfg := queryConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	dstValue := reflect.ValueOf(dst).Elem()
	if dstValue.Kind() != reflect.Struct {
		return fmt.Errorf("hah/reqx: destination must point to a struct")
	}

	plan, err := loadQueryDecodePlan(dstValue.Type())
	if err != nil {
		return err
	}

	values := url.Values{}
	if r.URL != nil {
		values = r.URL.Query()
	}

	violations, err := decodeQueryInto(dstValue, values, plan, cfg)
	if err != nil {
		return err
	}
	if len(violations) > 0 {
		return invalidFieldsError(violations)
	}

	return nil
}

// DecodeAndValidateQuery decodes URL query parameters, then runs validation.
func DecodeAndValidateQuery[T any](r *http.Request, dst *T, fn ValidateFunc[T], opts ...QueryOption) error {
	if err := DecodeQuery(r, dst, opts...); err != nil {
		return err
	}
	return Validate(dst, fn)
}

type queryFieldSpec struct {
	index int
	name  string
}

type queryDecodePlan struct {
	fields      []queryFieldSpec
	knownFields map[string]struct{}
}

type queryDecodePlanResult struct {
	plan *queryDecodePlan
	err  error
}

func loadQueryDecodePlan(t reflect.Type) (*queryDecodePlan, error) {
	if cached, ok := queryDecodePlanCache.Load(t); ok {
		result := cached.(queryDecodePlanResult)
		return result.plan, result.err
	}

	plan, err := buildQueryDecodePlan(t)
	result := queryDecodePlanResult{
		plan: plan,
		err:  err,
	}

	actual, _ := queryDecodePlanCache.LoadOrStore(t, result)
	stored := actual.(queryDecodePlanResult)
	return stored.plan, stored.err
}

func buildQueryDecodePlan(t reflect.Type) (*queryDecodePlan, error) {
	spec := make([]queryFieldSpec, 0, t.NumField())
	known := make(map[string]struct{}, t.NumField())
	seen := make(map[string]string, t.NumField())

	for i := range t.NumField() {
		field := t.Field(i)
		name, ok := queryFieldName(field)
		if !ok {
			continue
		}
		if !field.IsExported() {
			return nil, fmt.Errorf("hah/reqx: query field %q must be exported", field.Name)
		}
		if err := validateQueryFieldType(field.Type, field.Name); err != nil {
			return nil, err
		}
		if prev, exists := seen[name]; exists {
			return nil, fmt.Errorf("hah/reqx: duplicate query field %q on %s and %s", name, prev, field.Name)
		}

		seen[name] = field.Name
		known[name] = struct{}{}
		spec = append(spec, queryFieldSpec{
			index: i,
			name:  name,
		})
	}

	return &queryDecodePlan{
		fields:      spec,
		knownFields: known,
	}, nil
}

func queryFieldName(field reflect.StructField) (string, bool) {
	tag := field.Tag.Get("query")
	if tag == "" {
		return "", false
	}

	name, _, _ := strings.Cut(tag, ",")
	name = strings.TrimSpace(name)
	if name == "" || name == "-" {
		return "", false
	}

	return name, true
}

func validateQueryFieldType(t reflect.Type, fieldName string) error {
	if supportsTextUnmarshaler(t) {
		return nil
	}

	switch t.Kind() {
	case reflect.Pointer, reflect.Slice:
		return validateQueryFieldType(t.Elem(), fieldName)
	case reflect.String:
		return nil
	case reflect.Bool:
		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return nil
	case reflect.Float32, reflect.Float64:
		return nil
	default:
		return fmt.Errorf("hah/reqx: field %q has unsupported query type %s", fieldName, t)
	}
}

func decodeQueryInto(dst reflect.Value, values url.Values, plan *queryDecodePlan, cfg queryConfig) ([]Violation, error) {
	violations := make([]Violation, 0)
	if plan == nil {
		plan = &queryDecodePlan{}
	}

	for _, fieldSpec := range plan.fields {
		rawValues, ok := values[fieldSpec.name]
		if !ok {
			continue
		}

		fieldValue := dst.Field(fieldSpec.index)
		violation, err := decodeQueryField(fieldValue, rawValues, fieldSpec.name)
		if err != nil {
			return nil, err
		}
		if violation != nil {
			violations = append(violations, *violation)
		}
	}

	if cfg.allowUnknownFields {
		return violations, nil
	}

	unknownKeys := make([]string, 0)
	for key := range values {
		if _, ok := plan.knownFields[key]; !ok {
			unknownKeys = append(unknownKeys, key)
		}
	}
	sort.Strings(unknownKeys)

	for _, key := range unknownKeys {
		violations = append(violations, Violation{
			Field:   key,
			Code:    ViolationCodeUnknown,
			Message: "unknown field",
		})
	}

	return violations, nil
}

func decodeQueryField(dst reflect.Value, values []string, fieldName string) (*Violation, error) {
	if len(values) == 0 {
		return nil, nil
	}

	switch dst.Kind() {
	case reflect.Pointer:
		value := reflect.New(dst.Type().Elem())
		violation, err := decodeQueryField(value.Elem(), values, fieldName)
		if err != nil || violation != nil {
			return violation, err
		}
		dst.Set(value)
		return nil, nil
	case reflect.Slice:
		slice := reflect.MakeSlice(dst.Type(), 0, len(values))
		for _, rawValue := range values {
			item := reflect.New(dst.Type().Elem()).Elem()
			violation, err := decodeQueryField(item, []string{rawValue}, fieldName)
			if err != nil || violation != nil {
				return violation, err
			}
			slice = reflect.Append(slice, item)
		}
		dst.Set(slice)
		return nil, nil
	default:
		if supportsTextUnmarshaler(dst.Type()) {
			if len(values) > 1 {
				return &Violation{
					Field:   fieldName,
					Code:    ViolationCodeMultiple,
					Message: "must not be repeated",
				}, nil
			}
			return decodeTextValue(dst, values[0], fieldName)
		}
		if len(values) > 1 {
			return &Violation{
				Field:   fieldName,
				Code:    ViolationCodeMultiple,
				Message: "must not be repeated",
			}, nil
		}
		return decodeScalarValue(dst, values[0], fieldName)
	}
}

func decodeScalarValue(dst reflect.Value, rawValue string, fieldName string) (*Violation, error) {
	switch dst.Kind() {
	case reflect.String:
		dst.SetString(rawValue)
		return nil, nil
	case reflect.Bool:
		value, err := strconv.ParseBool(rawValue)
		if err != nil {
			return invalidQueryType(fieldName, dst.Type()), nil
		}
		dst.SetBool(value)
		return nil, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		value, err := strconv.ParseInt(rawValue, 10, dst.Type().Bits())
		if err != nil {
			return invalidQueryType(fieldName, dst.Type()), nil
		}
		dst.SetInt(value)
		return nil, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		value, err := strconv.ParseUint(rawValue, 10, dst.Type().Bits())
		if err != nil {
			return invalidQueryType(fieldName, dst.Type()), nil
		}
		dst.SetUint(value)
		return nil, nil
	case reflect.Float32, reflect.Float64:
		value, err := strconv.ParseFloat(rawValue, dst.Type().Bits())
		if err != nil {
			return invalidQueryType(fieldName, dst.Type()), nil
		}
		dst.SetFloat(value)
		return nil, nil
	default:
		return nil, fmt.Errorf("hah/reqx: unsupported query destination type %s", dst.Type())
	}
}

func decodeTextValue(dst reflect.Value, rawValue string, fieldName string) (*Violation, error) {
	target := dst
	if dst.Kind() != reflect.Pointer && dst.CanAddr() && dst.Addr().Type().Implements(textUnmarshalerType) {
		target = dst.Addr()
	}

	unmarshaler, ok := target.Interface().(encoding.TextUnmarshaler)
	if !ok {
		return nil, fmt.Errorf("hah/reqx: unsupported text unmarshaler destination type %s", dst.Type())
	}
	if err := unmarshaler.UnmarshalText([]byte(rawValue)); err != nil {
		return &Violation{
			Field:   fieldName,
			Code:    ViolationCodeInvalid,
			Message: "is invalid",
		}, nil
	}

	return nil, nil
}

func supportsTextUnmarshaler(t reflect.Type) bool {
	if t == nil {
		return false
	}
	if t.Implements(textUnmarshalerType) {
		return true
	}
	if t.Kind() == reflect.Pointer {
		return false
	}
	return reflect.PointerTo(t).Implements(textUnmarshalerType)
}

func invalidQueryType(fieldName string, t reflect.Type) *Violation {
	return &Violation{
		Field:   fieldName,
		Code:    ViolationCodeType,
		Message: "must be " + describeJSONType(t),
	}
}
