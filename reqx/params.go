package reqx

import (
	"encoding"
	"errors"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/kanata996/hah/bind"
	"github.com/kanata996/hah/errx"
)

// PathParam 按当前请求参数转换规则读取并解析单个 path 参数。
func PathParam[T any](r *http.Request, name string) (T, error) {
	values, exists := pathParamValues(r, name)
	return parseRequestParam[T](r, values, exists)
}

// QueryParam 按当前请求参数转换规则读取并解析单个 query 参数。
func QueryParam[T any](r *http.Request, name string) (T, error) {
	values, exists := queryParamValues(r, name)
	return parseRequestParam[T](r, values, exists)
}

type bindMultipleUnmarshaler interface {
	UnmarshalParams(params []string) error
}

func parseRequestParam[T any](r *http.Request, values []string, exists bool) (T, error) {
	var target T
	if r == nil {
		return target, errorsf("request must not be nil")
	}
	if !exists || len(values) == 0 {
		return target, nil
	}

	value := reflect.ValueOf(&target).Elem()
	if err := bindParamValues(value, values); err != nil {
		return target, badRequestWrap(err)
	}
	return target, nil
}

func badRequestWrap(err error) error {
	if err == nil {
		return nil
	}

	var httpErr *errx.HTTPError
	if errors.As(err, &httpErr) && httpErr != nil {
		return err
	}

	return errx.NewHTTPErrorWithCause(http.StatusBadRequest, "", "", err)
}

func pathParamValues(r *http.Request, name string) ([]string, bool) {
	if r == nil {
		return nil, false
	}
	if value := r.PathValue(name); value != "" {
		return []string{value}, true
	}
	for _, wildcard := range pathWildcardNames(r.Pattern) {
		if wildcard == name {
			return []string{r.PathValue(name)}, true
		}
	}
	return nil, false
}

func queryParamValues(r *http.Request, name string) ([]string, bool) {
	if r == nil || r.URL == nil {
		return nil, false
	}
	values, exists := r.URL.Query()[name]
	return values, exists
}

func bindParamValues(field reflect.Value, values []string) error {
	valueKind := field.Kind()

	if ok, err := unmarshalInputsToField(valueKind, values, field); ok {
		return err
	}
	if ok, err := unmarshalInputToField(valueKind, values[0], field); ok {
		return err
	}

	field, valueKind = concreteFieldValue(field, valueKind)

	if valueKind == reflect.Slice {
		slice := reflect.MakeSlice(field.Type(), len(values), len(values))
		for i := range values {
			if err := setWithProperType(slice.Index(i), values[i]); err != nil {
				return err
			}
		}
		field.Set(slice)
		return nil
	}

	return setWithProperType(field, values[0])
}

func unmarshalInputsToField(valueKind reflect.Kind, values []string, field reflect.Value) (bool, error) {
	if valueKind == reflect.Pointer &&
		!reflect.PointerTo(field.Type().Elem()).Implements(reflect.TypeFor[bindMultipleUnmarshaler]()) {
		return false, nil
	}

	field, _ = concreteFieldValue(field, valueKind)
	unmarshaler, ok := field.Addr().Interface().(bindMultipleUnmarshaler)
	if !ok {
		return false, nil
	}
	return true, unmarshaler.UnmarshalParams(values)
}

func unmarshalInputToField(valueKind reflect.Kind, value string, field reflect.Value) (bool, error) {
	if valueKind == reflect.Pointer {
		elemType := reflect.PointerTo(field.Type().Elem())
		if !elemType.Implements(reflect.TypeFor[bind.BindUnmarshaler]()) &&
			!elemType.Implements(reflect.TypeFor[encoding.TextUnmarshaler]()) {
			return false, nil
		}
	}

	field, _ = concreteFieldValue(field, valueKind)

	switch unmarshaler := field.Addr().Interface().(type) {
	case bind.BindUnmarshaler:
		return true, unmarshaler.UnmarshalParam(value)
	case encoding.TextUnmarshaler:
		return true, unmarshaler.UnmarshalText([]byte(value))
	}

	return false, nil
}

func setWithProperType(field reflect.Value, value string) error {
	valueKind := field.Kind()
	if ok, err := unmarshalInputToField(valueKind, value, field); ok {
		return err
	}

	field, valueKind = concreteFieldValue(field, valueKind)

	switch valueKind {
	case reflect.Int:
		return setIntField(value, 0, field)
	case reflect.Int8:
		return setIntField(value, 8, field)
	case reflect.Int16:
		return setIntField(value, 16, field)
	case reflect.Int32:
		return setIntField(value, 32, field)
	case reflect.Int64:
		return setIntField(value, 64, field)
	case reflect.Uint:
		return setUintField(value, 0, field)
	case reflect.Uint8:
		return setUintField(value, 8, field)
	case reflect.Uint16:
		return setUintField(value, 16, field)
	case reflect.Uint32:
		return setUintField(value, 32, field)
	case reflect.Uint64:
		return setUintField(value, 64, field)
	case reflect.Bool:
		return setBoolField(value, field)
	case reflect.Float32:
		return setFloatField(value, 32, field)
	case reflect.Float64:
		return setFloatField(value, 64, field)
	case reflect.String:
		field.SetString(value)
		return nil
	default:
		return errors.New("unknown type")
	}
}

func concreteFieldValue(field reflect.Value, kind reflect.Kind) (reflect.Value, reflect.Kind) {
	if kind != reflect.Pointer {
		return field, kind
	}
	if field.IsNil() {
		field.Set(reflect.New(field.Type().Elem()))
	}
	field = field.Elem()
	return field, field.Kind()
}

func setIntField(value string, bitSize int, field reflect.Value) error {
	if value == "" {
		value = "0"
	}
	intVal, err := strconv.ParseInt(value, 10, bitSize)
	if err == nil {
		field.SetInt(intVal)
	}
	return err
}

func setUintField(value string, bitSize int, field reflect.Value) error {
	if value == "" {
		value = "0"
	}
	uintVal, err := strconv.ParseUint(value, 10, bitSize)
	if err == nil {
		field.SetUint(uintVal)
	}
	return err
}

func setBoolField(value string, field reflect.Value) error {
	if value == "" {
		value = "false"
	}
	boolVal, err := strconv.ParseBool(value)
	if err == nil {
		field.SetBool(boolVal)
	}
	return err
}

func setFloatField(value string, bitSize int, field reflect.Value) error {
	if value == "" {
		value = "0.0"
	}
	floatVal, err := strconv.ParseFloat(value, bitSize)
	if err == nil {
		field.SetFloat(floatVal)
	}
	return err
}

func pathWildcardNames(pattern string) []string {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return nil
	}

	names := make([]string, 0, 2)
	for i := 0; i < len(pattern); i++ {
		if pattern[i] != '{' {
			continue
		}

		end := strings.IndexByte(pattern[i+1:], '}')
		if end < 0 {
			break
		}

		token := strings.TrimSpace(pattern[i+1 : i+1+end])
		token = strings.TrimSuffix(token, "...")
		token, _, _ = strings.Cut(token, ":")
		token = strings.TrimSpace(token)
		if token != "" && token != "$" {
			names = append(names, token)
		}

		i += end + 1
	}

	return names
}
