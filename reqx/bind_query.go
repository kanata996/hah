package reqx

import (
	"encoding"
	"errors"
	"net/http"
	"reflect"
	"strconv"
	"time"

	"github.com/kanata996/hah/errx"
)

var (
	queryMapKeyType    = reflect.TypeOf("")
	queryStringMapType = reflect.TypeOf([]string(nil))
)

type queryMapBindingMode int

const (
	queryMapBindingFirstValue queryMapBindingMode = iota + 1
	queryMapBindingAllValues
)

// BindUnmarshaler 允许字段从单个字符串输入值自定义解码。
type BindUnmarshaler interface {
	UnmarshalParam(param string) error
}

// bindMultipleUnmarshaler 允许字段一次性接收同名输入的全部值。
type bindMultipleUnmarshaler interface {
	UnmarshalParams(params []string) error
}

// BindQuery 只从 query 参数绑定数据。
func BindQuery(r *http.Request, target any) error {
	if err := validateBindInputs(r, target); err != nil {
		return err
	}

	return bindQuery(r, target)
}

func bindQuery(r *http.Request, target any) error {
	params := map[string][]string{}
	if r.URL != nil {
		params = r.URL.Query()
	}

	err := bindQueryData(target, params)
	if err == nil {
		return nil
	}

	var usageErr usageError
	if errors.As(err, &usageErr) {
		return err
	}

	var httpErr *errx.HTTPError
	if errors.As(err, &httpErr) && httpErr != nil {
		return err
	}

	return errx.NewHTTPErrorWithCause(http.StatusBadRequest, "", "", err)
}

func bindQueryData(target any, data map[string][]string) error {
	destination := reflect.ValueOf(target).Elem()
	switch destination.Kind() {
	case reflect.Map:
		mode, err := classifyQueryMapBinding(destination.Type())
		if err != nil {
			return err
		}
		if len(data) == 0 {
			return nil
		}
		return bindQueryMap(destination, data, mode)
	case reflect.Struct:
		if len(data) == 0 {
			return nil
		}
		return bindQueryStruct(destination, data)
	default:
		return usageErrorf("destination must point to struct or supported map")
	}
}

func classifyQueryMapBinding(mapType reflect.Type) (queryMapBindingMode, error) {
	if mapType.Key() != queryMapKeyType {
		return 0, usageErrorf("destination must point to struct or supported map")
	}

	switch elemType := mapType.Elem(); {
	case elemType == queryMapKeyType:
		return queryMapBindingFirstValue, nil
	case elemType == queryStringMapType:
		return queryMapBindingAllValues, nil
	case elemType.Kind() == reflect.Interface && elemType.NumMethod() == 0:
		return queryMapBindingFirstValue, nil
	default:
		return 0, usageErrorf("destination must point to struct or supported map")
	}
}

func bindQueryMap(destination reflect.Value, data map[string][]string, mode queryMapBindingMode) error {
	switch mode {
	case queryMapBindingFirstValue:
		return bindQueryFirstValueMap(destination, data)
	case queryMapBindingAllValues:
		return bindQueryValuesMap(destination, data)
	default:
		return usageErrorf("destination must point to struct or supported map")
	}
}

func bindQueryFirstValueMap(destination reflect.Value, data map[string][]string) error {
	ensureMap(destination)
	for key, values := range data {
		if len(values) == 0 {
			continue
		}
		destination.SetMapIndex(reflect.ValueOf(key), reflect.ValueOf(values[0]))
	}
	return nil
}

func bindQueryValuesMap(destination reflect.Value, data map[string][]string) error {
	ensureMap(destination)
	for key, values := range data {
		destination.SetMapIndex(reflect.ValueOf(key), reflect.ValueOf(values))
	}
	return nil
}

func ensureMap(destination reflect.Value) {
	if destination.IsNil() {
		destination.Set(reflect.MakeMap(destination.Type()))
	}
}

func bindQueryStruct(destination reflect.Value, data map[string][]string) error {
	destinationType := destination.Type()
	for i := 0; i < destinationType.NumField(); i++ {
		if err := bindQueryStructField(destination.Field(i), destinationType.Field(i), data); err != nil {
			return err
		}
	}
	return nil
}

func bindQueryStructField(field reflect.Value, typeField reflect.StructField, data map[string][]string) error {
	fieldName := typeField.Tag.Get("query")
	if err := validateAnonymousQueryField(typeField, fieldName); err != nil {
		return err
	}

	field = dereferenceAnonymousQueryField(typeField, field)
	if !field.IsValid() || !field.CanSet() {
		return nil
	}

	if fieldName == "" {
		return bindNestedQueryStruct(field, data)
	}

	values, ok := lookupQueryFieldValues(data, fieldName)
	if !ok {
		return nil
	}

	return bindTaggedQueryField(field, values, typeField.Tag.Get("format"))
}

func validateAnonymousQueryField(typeField reflect.StructField, fieldName string) error {
	if !typeField.Anonymous || fieldName == "" {
		return nil
	}

	embeddedType := typeField.Type
	if embeddedType.Kind() == reflect.Pointer {
		embeddedType = embeddedType.Elem()
	}
	if embeddedType.Kind() == reflect.Struct {
		return usageErrorf("query tags are not allowed with anonymous struct field")
	}

	return nil
}

func dereferenceAnonymousQueryField(typeField reflect.StructField, field reflect.Value) reflect.Value {
	if !typeField.Anonymous || field.Kind() != reflect.Pointer {
		return field
	}
	if field.IsNil() {
		return reflect.Value{}
	}
	return field.Elem()
}

func bindNestedQueryStruct(field reflect.Value, data map[string][]string) error {
	if field.Kind() != reflect.Struct {
		return nil
	}
	if _, ok := field.Addr().Interface().(BindUnmarshaler); ok {
		return nil
	}
	return bindQueryData(field.Addr().Interface(), data)
}

func lookupQueryFieldValues(data map[string][]string, name string) ([]string, bool) {
	values, ok := data[name]
	if !ok || len(values) == 0 {
		return nil, false
	}
	return values, true
}

func bindTaggedQueryField(field reflect.Value, values []string, formatTag string) error {
	writeField, commitWrite := stagedFieldValue(field)

	if ok, err := unmarshalMultipleInputs(values, writeField); ok {
		if err != nil {
			return err
		}
		commitWrite()
		return nil
	}

	if ok, err := unmarshalSingleInput(values[0], writeField, formatTag); ok {
		if err != nil {
			return err
		}
		commitWrite()
		return nil
	}

	if writeField.Kind() == reflect.Slice {
		if err := bindSliceField(writeField, values, formatTag); err != nil {
			return err
		}
		commitWrite()
		return nil
	}

	if err := setFieldValue(values[0], writeField, formatTag); err != nil {
		return err
	}
	commitWrite()
	return nil
}

func bindSliceField(field reflect.Value, values []string, formatTag string) error {
	slice := reflect.MakeSlice(field.Type(), len(values), len(values))
	for i, value := range values {
		if err := setFieldValue(value, slice.Index(i), formatTag); err != nil {
			return err
		}
	}
	field.Set(slice)
	return nil
}

func unmarshalMultipleInputs(values []string, field reflect.Value) (bool, error) {
	unmarshaler, ok := field.Addr().Interface().(bindMultipleUnmarshaler)
	if !ok {
		return false, nil
	}
	return true, unmarshaler.UnmarshalParams(values)
}

func unmarshalSingleInput(value string, field reflect.Value, formatTag string) (bool, error) {
	fieldValue := field.Addr().Interface()
	if formatTag != "" {
		if _, isTime := fieldValue.(*time.Time); isTime {
			t, err := time.Parse(formatTag, value)
			if err != nil {
				return true, err
			}
			field.Set(reflect.ValueOf(t))
			return true, nil
		}
	}

	switch unmarshaler := fieldValue.(type) {
	case BindUnmarshaler:
		return true, unmarshaler.UnmarshalParam(value)
	case encoding.TextUnmarshaler:
		return true, unmarshaler.UnmarshalText([]byte(value))
	}

	return false, nil
}

func setFieldValue(value string, field reflect.Value, formatTag string) error {
	if field.Kind() == reflect.Pointer {
		writeField, commitWrite := stagedFieldValue(field)
		if err := setFieldValue(value, writeField, formatTag); err != nil {
			return err
		}
		commitWrite()
		return nil
	}

	if ok, err := unmarshalSingleInput(value, field, formatTag); ok {
		return err
	}

	switch field.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return setIntField(value, field)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return setUintField(value, field)
	case reflect.Bool:
		return setBoolField(value, field)
	case reflect.Float32, reflect.Float64:
		return setFloatField(value, field)
	case reflect.String:
		field.SetString(value)
		return nil
	default:
		return usageErrorf("unsupported query field type: %s", field.Type())
	}
}

func stagedFieldValue(field reflect.Value) (reflect.Value, func()) {
	if field.Kind() != reflect.Pointer {
		return field, func() {}
	}

	if field.IsNil() {
		staged := reflect.New(field.Type().Elem())
		return staged.Elem(), func() {
			field.Set(staged)
		}
	}

	return field.Elem(), func() {}
}

func setIntField(value string, field reflect.Value) error {
	if value == "" {
		value = "0"
	}
	intValue, err := strconv.ParseInt(value, 10, field.Type().Bits())
	if err == nil {
		field.SetInt(intValue)
	}
	return err
}

func setUintField(value string, field reflect.Value) error {
	if value == "" {
		value = "0"
	}
	uintValue, err := strconv.ParseUint(value, 10, field.Type().Bits())
	if err == nil {
		field.SetUint(uintValue)
	}
	return err
}

func setBoolField(value string, field reflect.Value) error {
	if value == "" {
		value = "false"
	}
	boolValue, err := strconv.ParseBool(value)
	if err == nil {
		field.SetBool(boolValue)
	}
	return err
}

func setFloatField(value string, field reflect.Value) error {
	if value == "" {
		value = "0.0"
	}
	floatValue, err := strconv.ParseFloat(value, field.Type().Bits())
	if err == nil {
		field.SetFloat(floatValue)
	}
	return err
}
