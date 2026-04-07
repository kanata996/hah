package reqx

import (
	"encoding"
	"fmt"
	"net/http"
	"net/textproto"
	"net/url"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
)

var (
	textUnmarshalerType   = reflect.TypeFor[encoding.TextUnmarshaler]()
	queryDecodePlanCache  sync.Map
	pathDecodePlanCache   sync.Map
	headerDecodePlanCache sync.Map
)

type valueSource struct {
	name                  string
	tag                   string
	cache                 *sync.Map
	normalizeKey          func(string) string
	allowUnknownByDefault bool
}

var (
	querySource = valueSource{
		name:  "query",
		tag:   "query",
		cache: &queryDecodePlanCache,
	}
	pathSource = valueSource{
		name:                  "path",
		tag:                   "param",
		cache:                 &pathDecodePlanCache,
		allowUnknownByDefault: true,
	}
	headerSource = valueSource{
		name:                  "header",
		tag:                   "header",
		cache:                 &headerDecodePlanCache,
		normalizeKey:          textproto.CanonicalMIMEHeaderKey,
		allowUnknownByDefault: true,
	}
)

func BindQueryParams[T any](r *http.Request, dst *T, opts ...BindQueryParamsOption) error {
	cfg := applyBindOptions(opts...)
	return bindTaggedValues(r, dst, querySource, cfg.query)
}

func bindTaggedValues[T any](r *http.Request, dst *T, source valueSource, cfg bindValuesConfig) error {
	if r == nil {
		return errorsf("request must not be nil")
	}

	return bindIntoClone(dst, func(bound *T) error {
		return bindTaggedValuesInPlace(r, bound, source, cfg)
	})
}

func bindTaggedValuesInPlace[T any](r *http.Request, dst *T, source valueSource, cfg bindValuesConfig) error {
	dstValue := reflect.ValueOf(dst).Elem()
	if dstValue.Kind() != reflect.Struct {
		return fmt.Errorf("reqx: destination must point to a struct")
	}

	if source.allowUnknownByDefault {
		cfg.allowUnknownFields = true
	}

	plan, err := loadValueDecodePlan(dstValue.Type(), source)
	if err != nil {
		return err
	}

	violations, err := decodeValuesInto(dstValue, sourceValues(r, source, plan), plan, cfg, violationInForValueSource(source))
	if err != nil {
		return err
	}
	if len(violations) == 0 {
		return nil
	}

	return invalidFieldsError(violations)
}

type valueFieldSpec struct {
	index int
	name  string
}

type valueDecodePlan struct {
	fields      []valueFieldSpec
	knownFields map[string]struct{}
}

type valueDecodePlanResult struct {
	plan *valueDecodePlan
	err  error
}

func loadValueDecodePlan(t reflect.Type, source valueSource) (*valueDecodePlan, error) {
	if cached, ok := source.cache.Load(t); ok {
		result := cached.(valueDecodePlanResult)
		return result.plan, result.err
	}

	plan, err := buildValueDecodePlan(t, source)
	result := valueDecodePlanResult{plan: plan, err: err}

	actual, _ := source.cache.LoadOrStore(t, result)
	stored := actual.(valueDecodePlanResult)
	return stored.plan, stored.err
}

func buildValueDecodePlan(t reflect.Type, source valueSource) (*valueDecodePlan, error) {
	spec := make([]valueFieldSpec, 0, t.NumField())
	known := make(map[string]struct{}, t.NumField())
	seen := make(map[string]string, t.NumField())

	for i := range t.NumField() {
		field := t.Field(i)
		name, ok := valueFieldName(field, source)
		if !ok {
			continue
		}
		if !field.IsExported() {
			return nil, fmt.Errorf("reqx: %s field %q must be exported", source.name, field.Name)
		}
		if err := validateValueFieldType(field.Type, field.Name, source.name); err != nil {
			return nil, err
		}
		if prev, exists := seen[name]; exists {
			return nil, fmt.Errorf("reqx: duplicate %s field %q on %s and %s", source.name, name, prev, field.Name)
		}

		seen[name] = field.Name
		known[name] = struct{}{}
		spec = append(spec, valueFieldSpec{index: i, name: name})
	}

	return &valueDecodePlan{fields: spec, knownFields: known}, nil
}

func valueFieldName(field reflect.StructField, source valueSource) (string, bool) {
	tag := field.Tag.Get(source.tag)
	if tag == "" {
		return "", false
	}

	name, _, _ := strings.Cut(tag, ",")
	name = strings.TrimSpace(name)
	if name == "" || name == "-" {
		return "", false
	}
	if source.normalizeKey != nil {
		name = source.normalizeKey(name)
	}
	return name, true
}

func validateValueFieldType(t reflect.Type, fieldName, sourceName string) error {
	if supportsTextUnmarshaler(t) {
		return nil
	}

	switch t.Kind() {
	case reflect.Pointer, reflect.Slice:
		return validateValueFieldType(t.Elem(), fieldName, sourceName)
	case reflect.String, reflect.Bool:
		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return nil
	case reflect.Float32, reflect.Float64:
		return nil
	default:
		return fmt.Errorf("reqx: field %q has unsupported %s type %s", fieldName, sourceName, t)
	}
}

func decodeValuesInto(dst reflect.Value, values url.Values, plan *valueDecodePlan, cfg bindValuesConfig, inputs ...string) ([]Violation, error) {
	input := firstViolationInput(inputs...)
	violations := make([]Violation, 0)

	for _, fieldSpec := range plan.fields {
		rawValues, ok := values[fieldSpec.name]
		if !ok {
			continue
		}

		fieldValue := dst.Field(fieldSpec.index)
		violation, err := decodeQueryField(fieldValue, rawValues, fieldSpec.name, input)
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
		violations = append(violations, newViolation(key, input, ViolationCodeUnknown, violationDetailUnknownField))
	}

	return violations, nil
}

func sourceValues(r *http.Request, source valueSource, plan *valueDecodePlan) url.Values {
	switch source.tag {
	case pathSource.tag:
		return pathValuesForPlan(r, plan)
	case querySource.tag:
		if r == nil || r.URL == nil {
			return url.Values{}
		}
		return r.URL.Query()
	case headerSource.tag:
		return headerValues(r)
	default:
		panic(fmt.Sprintf("reqx: unsupported value source %q", source.tag))
	}
}

func headerValues(r *http.Request) url.Values {
	if r == nil || len(r.Header) == 0 {
		return url.Values{}
	}

	canonical := true
	for key := range r.Header {
		if key != textproto.CanonicalMIMEHeaderKey(strings.TrimSpace(key)) {
			canonical = false
			break
		}
	}
	if canonical {
		return url.Values(r.Header)
	}

	values := url.Values{}

	for key, rawValues := range r.Header {
		normalizedKey := textproto.CanonicalMIMEHeaderKey(strings.TrimSpace(key))
		if normalizedKey == "" {
			continue
		}
		values[normalizedKey] = append(values[normalizedKey], rawValues...)
	}
	return values
}

func decodeQueryField(dst reflect.Value, values []string, fieldName string, inputs ...string) (*Violation, error) {
	input := firstViolationInput(inputs...)
	if len(values) == 0 {
		return nil, nil
	}

	switch dst.Kind() {
	case reflect.Pointer:
		value := reflect.New(dst.Type().Elem())
		violation, err := decodeQueryField(value.Elem(), values, fieldName, input)
		if err != nil || violation != nil {
			return violation, err
		}
		dst.Set(value)
		return nil, nil
	case reflect.Slice:
		slice := reflect.MakeSlice(dst.Type(), 0, len(values))
		for _, rawValue := range values {
			item := reflect.New(dst.Type().Elem()).Elem()
			violation, err := decodeQueryField(item, []string{rawValue}, fieldName, input)
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
				return multipleValueViolation(fieldName, input), nil
			}
			return decodeTextValue(dst, values[0], fieldName, input)
		}
		if len(values) > 1 {
			return multipleValueViolation(fieldName, input), nil
		}
		return decodeScalarValue(dst, values[0], fieldName, input)
	}
}

func decodeScalarValue(dst reflect.Value, rawValue string, fieldName string, inputs ...string) (*Violation, error) {
	input := firstViolationInput(inputs...)
	switch dst.Kind() {
	case reflect.String:
		dst.SetString(rawValue)
		return nil, nil
	case reflect.Bool:
		value, err := strconv.ParseBool(rawValue)
		if err != nil {
			return invalidValueType(fieldName, input, dst.Type()), nil
		}
		dst.SetBool(value)
		return nil, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		value, err := strconv.ParseInt(rawValue, 10, dst.Type().Bits())
		if err != nil {
			return invalidValueType(fieldName, input, dst.Type()), nil
		}
		dst.SetInt(value)
		return nil, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		value, err := strconv.ParseUint(rawValue, 10, dst.Type().Bits())
		if err != nil {
			return invalidValueType(fieldName, input, dst.Type()), nil
		}
		dst.SetUint(value)
		return nil, nil
	case reflect.Float32, reflect.Float64:
		value, err := strconv.ParseFloat(rawValue, dst.Type().Bits())
		if err != nil {
			return invalidValueType(fieldName, input, dst.Type()), nil
		}
		dst.SetFloat(value)
		return nil, nil
	default:
		return nil, fmt.Errorf("reqx: unsupported destination type %s", dst.Type())
	}
}

func decodeTextValue(dst reflect.Value, rawValue string, fieldName string, inputs ...string) (*Violation, error) {
	input := firstViolationInput(inputs...)
	target := dst
	if dst.Kind() != reflect.Pointer && dst.CanAddr() && dst.Addr().Type().Implements(textUnmarshalerType) {
		target = dst.Addr()
	}

	unmarshaler := target.Interface().(encoding.TextUnmarshaler)
	if err := unmarshaler.UnmarshalText([]byte(rawValue)); err != nil {
		violation := newViolation(fieldName, input, ViolationCodeInvalid, violationDetailInvalid)
		return &violation, nil
	}
	return nil, nil
}

func supportsTextUnmarshaler(t reflect.Type) bool {
	if t.Implements(textUnmarshalerType) {
		return true
	}
	if t.Kind() == reflect.Pointer {
		return false
	}
	return reflect.PointerTo(t).Implements(textUnmarshalerType)
}

func invalidValueType(fieldName, input string, t reflect.Type) *Violation {
	violation := newViolation(fieldName, input, ViolationCodeType, "must be "+describeJSONType(t))
	return &violation
}

func violationInForValueSource(source valueSource) string {
	return violationInForTag(source.tag)
}

func firstViolationInput(inputs ...string) string {
	for _, input := range inputs {
		if strings.TrimSpace(input) != "" {
			return input
		}
	}
	return ViolationInRequest
}

func multipleValueViolation(fieldName, input string) *Violation {
	violation := newViolation(fieldName, input, ViolationCodeMultiple, violationDetailMustNotRepeat)
	return &violation
}
