package bind

import (
	"encoding"
	"errors"
	"net/http"
	"net/textproto"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kanata996/hah/errx"
	"github.com/kanata996/hah/internal/req"
)

// 本文件负责 path/query/header 这类字符串键值源的默认绑定逻辑和共享反射辅助。
//
// 这里承载的能力包括：
//   - path/query/header 单源 binder 的默认实现
//   - 结构体字段、map 目标、slice 目标的反射写入逻辑
//   - 单值 / 多值自定义解码接口适配
//   - 标量类型转换、重复值处理、缺失值保留、header key 规范化
//   - path pattern 中 wildcard 名称提取

// bindMultipleUnmarshaler 允许字段一次性接收同名输入的全部值。
type bindMultipleUnmarshaler interface {
	UnmarshalParams(params []string) error
}

// bindPathValuesDefault 负责把 path 参数绑定到目标对象。
func bindPathValuesDefault(r *http.Request, target any) error {
	params := map[string][]string{}
	for _, name := range req.PathWildcardNames(r.Pattern) {
		params[name] = []string{r.PathValue(name)}
	}
	return bindStringSourceDefault(target, params, "param")
}

// bindQueryParamsDefault 负责把 query 参数绑定到目标对象。
func bindQueryParamsDefault(r *http.Request, target any) error {
	params := map[string][]string{}
	if r.URL != nil {
		params = r.URL.Query()
	}
	return bindStringSourceDefault(target, params, "query")
}

// bindHeadersDefault 负责把 header 参数绑定到目标对象。
func bindHeadersDefault(r *http.Request, target any) error {
	params := map[string][]string{}
	type headerEntry struct {
		trimmed   string
		canonical string
		values    []string
	}

	grouped := map[string][]headerEntry{}
	for key, values := range r.Header {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			continue
		}
		canonical := textproto.CanonicalMIMEHeaderKey(trimmed)
		grouped[canonical] = append(grouped[canonical], headerEntry{
			trimmed:   trimmed,
			canonical: canonical,
			values:    values,
		})
	}

	for canonical, entries := range grouped {
		sort.Slice(entries, func(i, j int) bool {
			iCanonical := entries[i].trimmed == entries[i].canonical
			jCanonical := entries[j].trimmed == entries[j].canonical
			if iCanonical != jCanonical {
				return iCanonical
			}
			return entries[i].trimmed < entries[j].trimmed
		})

		merged := make([]string, 0)
		for _, entry := range entries {
			merged = append(merged, entry.values...)
		}
		params[canonical] = merged
	}
	return bindStringSourceDefault(target, params, "header")
}

// bindStringSourceDefault 为 path/query/header 这类字符串源复用同一套错误边界。
func bindStringSourceDefault(target any, data map[string][]string, tag string) error {
	err := bindDataDefault(target, data, tag)
	if err == nil {
		return nil
	}

	var httpErr *errx.HTTPError
	if errors.As(err, &httpErr) && httpErr != nil {
		return err
	}

	return errx.NewHTTPErrorWithCause(http.StatusBadRequest, "", "", err)
}

// bindDataDefault 按 tag 和字段类型把字符串输入写入目标对象。
// 调用方保证 destination 已通过公开入口校验，是非 nil 指针。
func bindDataDefault(destination any, data map[string][]string, tag string) error {
	if len(data) == 0 {
		return nil
	}

	val := reflect.ValueOf(destination).Elem()
	typ := val.Type()

	stringType := reflect.TypeOf("")
	sliceOfStringType := reflect.TypeOf([]string(nil))

	// map 目标只支持 string key，并且只接受 bind 契约里显式允许的值类型。
	if typ.Kind() == reflect.Map && typ.Key() == stringType {
		elemType := typ.Elem()
		isElemInterface := elemType.Kind() == reflect.Interface && elemType.NumMethod() == 0
		isElemString := elemType == stringType
		isElemSliceOfStrings := elemType == sliceOfStringType
		if !isElemSliceOfStrings && !isElemString && !isElemInterface {
			return nil
		}
		if val.IsNil() {
			val.Set(reflect.MakeMap(typ))
		}
		for key, values := range data {
			switch {
			case isElemString, isElemInterface:
				if len(values) == 0 {
					continue
				}
				val.SetMapIndex(reflect.ValueOf(key), reflect.ValueOf(values[0]))
			default:
				val.SetMapIndex(reflect.ValueOf(key), reflect.ValueOf(values))
			}
		}
		return nil
	}

	if typ.Kind() != reflect.Struct {
		// 当前字符串源 binder 只支持 struct 和约定的 map 目标；
		// 其它目标按公开契约保持 no-op。
		return nil
	}

	for i := 0; i < typ.NumField(); i++ {
		typeField := typ.Field(i)
		inputFieldName := typeField.Tag.Get(tag)
		if typeField.Anonymous {
			embeddedType := typeField.Type
			if embeddedType.Kind() == reflect.Pointer {
				embeddedType = embeddedType.Elem()
			}
			if embeddedType.Kind() == reflect.Struct && inputFieldName != "" {
				return errors.New("query/param/header tags are not allowed with anonymous struct field")
			}
		}

		structField := val.Field(i)
		if typeField.Anonymous && structField.Kind() == reflect.Pointer {
			if structField.IsNil() {
				continue
			}
			structField = structField.Elem()
		}
		if !structField.CanSet() {
			continue
		}

		structFieldKind := structField.Kind()

		if inputFieldName == "" {
			// 未显式标注输入名时，仅递归进入普通嵌套 struct；自定义解码字段保持自行接管。
			if _, ok := structField.Addr().Interface().(BindUnmarshaler); !ok && structFieldKind == reflect.Struct {
				if err := bindDataDefault(structField.Addr().Interface(), data, tag); err != nil {
					return err
				}
			}
			continue
		}
		if tag == "header" {
			inputFieldName = textproto.CanonicalMIMEHeaderKey(strings.TrimSpace(inputFieldName))
		}

		inputValue, exists := data[inputFieldName]
		if !exists {
			continue
		}
		if len(inputValue) == 0 {
			// Malformed source entries with no concrete values should be ignored
			// instead of panicking or overwriting existing data.
			continue
		}

		writeField, commitWrite := stagedFieldValueDefault(structField)
		writeFieldKind := writeField.Kind()

		// 多值自定义解码拥有最高优先级，用于字段自行决定如何消费重复输入。
		if ok, err := unmarshalInputsToFieldDefault(inputValue, writeField); ok {
			if err != nil {
				return err
			}
			commitWrite()
			continue
		}

		formatTag := typeField.Tag.Get("format")
		// 单值自定义解码和 format 驱动的 time 解析在标量转换前执行。
		if ok, err := unmarshalInputToFieldDefault(inputValue[0], writeField, formatTag); ok {
			if err != nil {
				return err
			}
			commitWrite()
			continue
		}

		if writeFieldKind == reflect.Slice {
			numElems := len(inputValue)
			slice := reflect.MakeSlice(writeField.Type(), numElems, numElems)
			for j := 0; j < numElems; j++ {
				if err := setWithProperTypeDefault(inputValue[j], slice.Index(j), formatTag); err != nil {
					return err
				}
			}
			writeField.Set(slice)
			commitWrite()
			continue
		}

		if err := setWithProperTypeDefault(inputValue[0], writeField, formatTag); err != nil {
			return err
		}
		commitWrite()
	}

	return nil
}

// unmarshalInputsToFieldDefault 优先尝试多值自定义解码接口。
func unmarshalInputsToFieldDefault(values []string, field reflect.Value) (bool, error) {
	unmarshaler, ok := field.Addr().Interface().(bindMultipleUnmarshaler)
	if !ok {
		return false, nil
	}
	return true, unmarshaler.UnmarshalParams(values)
}

// unmarshalInputToFieldDefault 优先尝试单值自定义解码接口和 time format 解析。
func unmarshalInputToFieldDefault(value string, field reflect.Value, formatTag string) (bool, error) {
	fieldIValue := field.Addr().Interface()
	if formatTag != "" {
		if _, isTime := fieldIValue.(*time.Time); isTime {
			t, err := time.Parse(formatTag, value)
			if err != nil {
				return true, err
			}
			field.Set(reflect.ValueOf(t))
			return true, nil
		}
	}

	switch unmarshaler := fieldIValue.(type) {
	case BindUnmarshaler:
		return true, unmarshaler.UnmarshalParam(value)
	case encoding.TextUnmarshaler:
		return true, unmarshaler.UnmarshalText([]byte(value))
	}

	return false, nil
}

// setWithProperTypeDefault 按字段 kind 把单个字符串值转换并写入字段。
func setWithProperTypeDefault(value string, structField reflect.Value, formatTag string) error {
	if structField.Kind() == reflect.Pointer {
		writeField, commitWrite := stagedFieldValueDefault(structField)
		if err := setWithProperTypeDefault(value, writeField, formatTag); err != nil {
			return err
		}
		commitWrite()
		return nil
	}

	if ok, err := unmarshalInputToFieldDefault(value, structField, formatTag); ok {
		return err
	}

	switch structField.Kind() {
	case reflect.Int:
		return setIntFieldDefault(value, 0, structField)
	case reflect.Int8:
		return setIntFieldDefault(value, 8, structField)
	case reflect.Int16:
		return setIntFieldDefault(value, 16, structField)
	case reflect.Int32:
		return setIntFieldDefault(value, 32, structField)
	case reflect.Int64:
		return setIntFieldDefault(value, 64, structField)
	case reflect.Uint:
		return setUintFieldDefault(value, 0, structField)
	case reflect.Uint8:
		return setUintFieldDefault(value, 8, structField)
	case reflect.Uint16:
		return setUintFieldDefault(value, 16, structField)
	case reflect.Uint32:
		return setUintFieldDefault(value, 32, structField)
	case reflect.Uint64:
		return setUintFieldDefault(value, 64, structField)
	case reflect.Bool:
		return setBoolFieldDefault(value, structField)
	case reflect.Float32:
		return setFloatFieldDefault(value, 32, structField)
	case reflect.Float64:
		return setFloatFieldDefault(value, 64, structField)
	case reflect.String:
		structField.SetString(value)
	default:
		return errors.New("unknown type")
	}
	return nil
}

// stagedFieldValueDefault 为 pointer 字段准备一个可写入的具体值。
// 对 nil pointer 会先写入临时值，只有整个字段绑定成功后才提交到原字段。
func stagedFieldValueDefault(field reflect.Value) (reflect.Value, func()) {
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

func setIntFieldDefault(value string, bitSize int, field reflect.Value) error {
	if value == "" {
		value = "0"
	}
	intVal, err := strconv.ParseInt(value, 10, bitSize)
	if err == nil {
		field.SetInt(intVal)
	}
	return err
}

func setUintFieldDefault(value string, bitSize int, field reflect.Value) error {
	if value == "" {
		value = "0"
	}
	uintVal, err := strconv.ParseUint(value, 10, bitSize)
	if err == nil {
		field.SetUint(uintVal)
	}
	return err
}

func setBoolFieldDefault(value string, field reflect.Value) error {
	if value == "" {
		value = "false"
	}
	boolVal, err := strconv.ParseBool(value)
	if err == nil {
		field.SetBool(boolVal)
	}
	return err
}

func setFloatFieldDefault(value string, bitSize int, field reflect.Value) error {
	if value == "" {
		value = "0.0"
	}
	floatVal, err := strconv.ParseFloat(value, bitSize)
	if err == nil {
		field.SetFloat(floatVal)
	}
	return err
}
