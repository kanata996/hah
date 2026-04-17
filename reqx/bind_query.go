package reqx

import (
	"encoding"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kanata996/hah/errx"
)

// BindUnmarshaler 保留为兼容接口，但默认 BindQuery 契约不支持自定义 decoder。
type BindUnmarshaler interface {
	UnmarshalParam(param string) error
}

// BindMultipleUnmarshaler 保留为兼容接口，但默认 BindQuery 契约不支持多值 decoder。
type BindMultipleUnmarshaler interface {
	UnmarshalParams(params []string) error
}

var (
	queryTimeType            = reflect.TypeOf(time.Time{})
	queryUUIDType            = reflect.TypeOf(uuid.UUID{})
	queryDurationType        = reflect.TypeOf(time.Duration(0))
	queryStringStringMapType = reflect.TypeOf(map[string]string{})
	queryTextUnmarshalerType = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()
	queryBindUnmarshalerType = reflect.TypeOf((*BindUnmarshaler)(nil)).Elem()
)

type bindQueryFieldPlan struct {
	index []int
	key   string
	typ   reflect.Type
}

// BindQuery 只从 query 参数绑定数据。
func BindQuery(r *http.Request, target any) error {
	if err := validateBindInputs(r, target); err != nil {
		return err
	}

	source, err := parseQuerySource(r)
	if err != nil {
		return err
	}

	dst := reflect.ValueOf(target).Elem()
	switch dst.Kind() {
	case reflect.Struct:
		return bindQueryIntoStruct(dst, source)
	case reflect.Map:
		if dst.Type() != queryStringStringMapType {
			return usageErrorf("destination must point to struct or supported map")
		}
		return bindQueryIntoMap(dst, source)
	default:
		return usageErrorf("destination must point to struct or supported map")
	}
}

func parseQuerySource(r *http.Request) (url.Values, error) {
	if r.URL == nil {
		return url.Values{}, nil
	}

	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return nil, errx.NewHTTPError(http.StatusBadRequest, "bad_request", "Bad Request")
	}

	return values, nil
}

func bindQueryIntoMap(dst reflect.Value, source url.Values) error {
	snapshot := make(map[string]string, len(source))
	for key, values := range source {
		if len(values) > 1 {
			return errx.NewHTTPError(http.StatusBadRequest, "bad_request", "Bad Request")
		}
		if len(values) == 1 {
			snapshot[key] = values[0]
		}
	}

	dst.Set(reflect.MakeMapWithSize(dst.Type(), len(snapshot)))
	for key, value := range snapshot {
		dst.SetMapIndex(reflect.ValueOf(key), reflect.ValueOf(value))
	}
	return nil
}

func bindQueryIntoStruct(dst reflect.Value, source url.Values) error {
	plans, err := buildBindQueryPlan(dst.Type(), nil, map[string]struct{}{})
	if err != nil {
		return err
	}

	for _, values := range source {
		if len(values) > 1 {
			return errx.NewHTTPError(http.StatusBadRequest, "bad_request", "Bad Request")
		}
	}

	temp := reflect.New(dst.Type()).Elem()
	for _, plan := range plans {
		values, ok := source[plan.key]
		if !ok || len(values) == 0 {
			continue
		}
		if err := setBindQueryPlannedField(temp, plan, values[0]); err != nil {
			return err
		}
	}

	dst.Set(temp)
	return nil
}

func buildBindQueryPlan(t reflect.Type, prefix []int, seen map[string]struct{}) ([]bindQueryFieldPlan, error) {
	var plans []bindQueryFieldPlan

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		tag, ok, inline, err := parseBindQueryTag(field)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}

		index := append(append([]int(nil), prefix...), i)
		if inline {
			inlineType := field.Type
			if inlineType.Kind() == reflect.Pointer {
				inlineType = inlineType.Elem()
			}
			if inlineType.Kind() != reflect.Struct {
				return nil, usageErrorf("unsupported query field type")
			}

			childPlans, err := buildBindQueryPlan(inlineType, index, seen)
			if err != nil {
				return nil, err
			}
			if len(childPlans) == 0 {
				return nil, usageErrorf("inline field must expose at least one bindable child")
			}
			plans = append(plans, childPlans...)
			continue
		}

		if err := validateBindQueryLeafType(field.Type); err != nil {
			return nil, err
		}
		if _, exists := seen[tag]; exists {
			return nil, usageErrorf("duplicate query field %q", tag)
		}
		seen[tag] = struct{}{}
		plans = append(plans, bindQueryFieldPlan{index: index, key: tag, typ: field.Type})
	}

	return plans, nil
}

func parseBindQueryTag(field reflect.StructField) (name string, ok bool, inline bool, err error) {
	raw, tagged := field.Tag.Lookup("query")
	if !tagged {
		return "", false, false, nil
	}

	switch raw {
	case "-":
		return "", false, false, nil
	case ",inline":
		return "", true, true, nil
	}

	if raw == "" || strings.TrimSpace(raw) != raw || strings.Contains(raw, ",") {
		return "", false, false, usageErrorf("invalid query tag")
	}
	return raw, true, false, nil
}

func validateBindQueryLeafType(t reflect.Type) error {
	base := t
	if base.Kind() == reflect.Pointer {
		base = base.Elem()
		if base.Kind() == reflect.Pointer {
			return usageErrorf("unsupported query field type")
		}
	}

	if isExplicitBindQuerySpecialType(base) {
		return nil
	}
	if disallowedBindQueryDecoder(base) {
		return usageErrorf("unsupported query field type")
	}

	switch base.Kind() {
	case reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return nil
	default:
		return usageErrorf("unsupported query field type")
	}
}

func isExplicitBindQuerySpecialType(t reflect.Type) bool {
	return t == queryDurationType || t == queryTimeType || t == queryUUIDType
}

func disallowedBindQueryDecoder(t reflect.Type) bool {
	ptr := reflect.PointerTo(t)
	if t != queryTimeType && t != queryUUIDType {
		if ptr.Implements(queryTextUnmarshalerType) || ptr.Implements(queryBindUnmarshalerType) {
			return true
		}
	}
	return false
}

func setBindQueryPlannedField(dst reflect.Value, plan bindQueryFieldPlan, raw string) error {
	field, err := fieldByIndexForSet(dst, plan.index)
	if err != nil {
		return err
	}

	if field.Kind() == reflect.Pointer {
		elem := reflect.New(field.Type().Elem())
		if err := setBindQueryLeaf(elem.Elem(), raw); err != nil {
			return err
		}
		field.Set(elem)
		return nil
	}

	return setBindQueryLeaf(field, raw)
}

func fieldByIndexForSet(v reflect.Value, index []int) (reflect.Value, error) {
	current := v
	for _, i := range index {
		field := current.Field(i)
		if field.Kind() == reflect.Pointer {
			if field.Type().Elem().Kind() != reflect.Struct {
				return field, nil
			}
			if field.IsNil() {
				field.Set(reflect.New(field.Type().Elem()))
			}
			current = field.Elem()
			continue
		}
		current = field
	}
	return current, nil
}

func setBindQueryLeaf(field reflect.Value, raw string) error {
	if field.Type() == queryDurationType {
		value, err := time.ParseDuration(raw)
		if err != nil {
			return errx.NewHTTPError(http.StatusBadRequest, "bad_request", "Bad Request")
		}
		field.SetInt(int64(value))
		return nil
	}
	if field.Type() == queryTimeType {
		value, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return errx.NewHTTPError(http.StatusBadRequest, "bad_request", "Bad Request")
		}
		field.Set(reflect.ValueOf(value))
		return nil
	}
	if field.Type() == queryUUIDType {
		value, err := uuid.Parse(raw)
		if err != nil {
			return errx.NewHTTPError(http.StatusBadRequest, "bad_request", "Bad Request")
		}
		field.Set(reflect.ValueOf(value))
		return nil
	}

	switch field.Kind() {
	case reflect.String:
		field.SetString(raw)
		return nil
	case reflect.Bool:
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return errx.NewHTTPError(http.StatusBadRequest, "bad_request", "Bad Request")
		}
		field.SetBool(value)
		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		value, err := strconv.ParseInt(raw, 10, field.Type().Bits())
		if err != nil {
			return errx.NewHTTPError(http.StatusBadRequest, "bad_request", "Bad Request")
		}
		field.SetInt(value)
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		value, err := strconv.ParseUint(raw, 10, field.Type().Bits())
		if err != nil {
			return errx.NewHTTPError(http.StatusBadRequest, "bad_request", "Bad Request")
		}
		field.SetUint(value)
		return nil
	case reflect.Float32, reflect.Float64:
		value, err := strconv.ParseFloat(raw, field.Type().Bits())
		if err != nil {
			return errx.NewHTTPError(http.StatusBadRequest, "bad_request", "Bad Request")
		}
		field.SetFloat(value)
		return nil
	default:
		return usageErrorf("unsupported query field type")
	}
}
