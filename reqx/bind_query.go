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
	"github.com/kanata996/hah/internal/errx"
)

var (
	queryTimeType            = reflect.TypeFor[time.Time]()
	queryUUIDType            = reflect.TypeFor[uuid.UUID]()
	queryDurationType        = reflect.TypeFor[time.Duration]()
	queryStringStringMapType = reflect.TypeFor[map[string]string]()
	queryTextUnmarshalerType = reflect.TypeFor[encoding.TextUnmarshaler]()
)

type bindQueryFieldPlan struct {
	index int
	key   string
	set   func(reflect.Value, string) error
}

// BindQuery 只从 query 参数绑定数据。
func BindQuery(r *http.Request, target any) error {
	if err := validateBindInputs(r, target); err != nil {
		return err
	}

	dst := reflect.ValueOf(target).Elem()
	switch dst.Kind() {
	case reflect.Struct:
		plans, err := buildBindQueryPlan(dst.Type())
		if err != nil {
			return err
		}

		source, err := parseQuerySource(r)
		if err != nil {
			return err
		}
		return bindQueryIntoStruct(dst, source, plans)
	case reflect.Map:
		if dst.Type() != queryStringStringMapType {
			return usageErrorf("destination must point to struct or supported map")
		}

		source, err := parseQuerySource(r)
		if err != nil {
			return err
		}
		return bindQueryIntoMap(dst, source)
	default:
		return usageErrorf("destination must point to struct or supported map")
	}
}

// 统一解析当前请求的 RawQuery，并把 malformed query 收敛为稳定 400。
func parseQuerySource(r *http.Request) (url.Values, error) {
	if r.URL == nil {
		return url.Values{}, nil
	}

	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return nil, bindQueryBadRequestError()
	}

	return values, nil
}

// 生成当前请求的单值字符串快照。
// 任一 key 只要出现多个值，整个绑定立即失败且 target 不提交新状态。
func bindQueryIntoMap(dst reflect.Value, source url.Values) error {
	snapshot := make(map[string]string, len(source))
	for key, values := range source {
		if len(values) > 1 {
			return bindQueryBadRequestError()
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

// 先校验 source 满足单值模型，再只重建参与绑定的字段并一次性提交。
func bindQueryIntoStruct(dst reflect.Value, source url.Values, plans []bindQueryFieldPlan) error {
	for _, values := range source {
		if len(values) > 1 {
			return bindQueryBadRequestError()
		}
	}

	temp := reflect.New(dst.Type()).Elem()
	temp.Set(dst)
	for _, plan := range plans {
		temp.Field(plan.index).SetZero()
	}
	for _, plan := range plans {
		values, ok := source[plan.key]
		if !ok || len(values) == 0 {
			continue
		}
		if err := plan.set(temp.Field(plan.index), values[0]); err != nil {
			return err
		}
	}

	dst.Set(temp)
	return nil
}

// 扫描顶层导出字段，预编译出 tag、字段位置和对应 setter。
// 规划阶段同时完成 tag 校验、重复 key 检测和字段类型支持性判断。
func buildBindQueryPlan(t reflect.Type) ([]bindQueryFieldPlan, error) {
	var plans []bindQueryFieldPlan
	seen := map[string]struct{}{}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		tag, ok, err := parseBindQueryTag(field)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}

		setter, err := buildBindQueryFieldSetter(field.Type)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[tag]; exists {
			return nil, usageErrorf("duplicate query field %q", tag)
		}
		seen[tag] = struct{}{}
		plans = append(plans, bindQueryFieldPlan{index: i, key: tag, set: setter})
	}

	return plans, nil
}

// 只接受 query:"name" 和 query:"-" 两种公开 tag 形态。
func parseBindQueryTag(field reflect.StructField) (name string, ok bool, err error) {
	raw, tagged := field.Tag.Lookup("query")
	if !tagged {
		return "", false, nil
	}
	if raw == "-" {
		return "", false, nil
	}
	if raw == "" || strings.TrimSpace(raw) != raw || strings.Contains(raw, ",") {
		return "", false, usageErrorf("invalid query tag")
	}
	return raw, true, nil
}

func disallowedBindQueryDecoder(t reflect.Type) bool {
	ptr := reflect.PointerTo(t)
	if t != queryTimeType && t != queryUUIDType {
		if ptr.Implements(queryTextUnmarshalerType) {
			return true
		}
	}
	return false
}

// 为字段形状生成 setter。
// 一级指针字段会先解码到临时值，成功后再分配并写回，避免失败路径污染 target。
func buildBindQueryFieldSetter(t reflect.Type) (func(reflect.Value, string) error, error) {
	if t.Kind() == reflect.Pointer {
		elemType := t.Elem()
		if elemType.Kind() == reflect.Pointer {
			return nil, unsupportedBindQueryFieldTypeError()
		}

		setLeaf, err := buildBindQueryLeafSetter(elemType)
		if err != nil {
			return nil, err
		}
		return func(field reflect.Value, raw string) error {
			elem := reflect.New(elemType)
			if err := setLeaf(elem.Elem(), raw); err != nil {
				return err
			}
			field.Set(elem)
			return nil
		}, nil
	}

	return buildBindQueryLeafSetter(t)
}

// 这里是受支持叶子类型的唯一分派点：
// 它同时决定“该类型是否支持”以及“原始字符串如何解码并写入字段”。
func buildBindQueryLeafSetter(t reflect.Type) (func(reflect.Value, string) error, error) {
	switch t {
	case queryDurationType:
		return func(field reflect.Value, raw string) error {
			value, err := time.ParseDuration(raw)
			if err != nil {
				return bindQueryBadRequestError()
			}
			field.SetInt(int64(value))
			return nil
		}, nil
	case queryTimeType:
		return func(field reflect.Value, raw string) error {
			value, err := parseRFC3339Time(raw)
			if err != nil {
				return bindQueryBadRequestError()
			}
			field.Set(reflect.ValueOf(value))
			return nil
		}, nil
	case queryUUIDType:
		return func(field reflect.Value, raw string) error {
			value, err := uuid.Parse(raw)
			if err != nil {
				return bindQueryBadRequestError()
			}
			field.Set(reflect.ValueOf(value))
			return nil
		}, nil
	}
	if disallowedBindQueryDecoder(t) {
		return nil, unsupportedBindQueryFieldTypeError()
	}

	switch t.Kind() {
	case reflect.String:
		return func(field reflect.Value, raw string) error {
			field.SetString(raw)
			return nil
		}, nil
	case reflect.Bool:
		return func(field reflect.Value, raw string) error {
			value, err := strconv.ParseBool(raw)
			if err != nil {
				return bindQueryBadRequestError()
			}
			field.SetBool(value)
			return nil
		}, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return func(field reflect.Value, raw string) error {
			value, err := strconv.ParseInt(raw, 10, field.Type().Bits())
			if err != nil {
				return bindQueryBadRequestError()
			}
			field.SetInt(value)
			return nil
		}, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return func(field reflect.Value, raw string) error {
			value, err := strconv.ParseUint(raw, 10, field.Type().Bits())
			if err != nil {
				return bindQueryBadRequestError()
			}
			field.SetUint(value)
			return nil
		}, nil
	case reflect.Float32, reflect.Float64:
		return func(field reflect.Value, raw string) error {
			value, err := parseFloatBits(raw, field.Type().Bits())
			if err != nil {
				return bindQueryBadRequestError()
			}
			field.SetFloat(value)
			return nil
		}, nil
	default:
		return nil, unsupportedBindQueryFieldTypeError()
	}
}

// 统一构造 BindQuery 的稳定客户端输入错误。
func bindQueryBadRequestError() error {
	return errx.NewHTTPError(http.StatusBadRequest, "bad_request", "")
}

// 统一标记 DTO 字段形状不受支持的 usage error。
func unsupportedBindQueryFieldTypeError() error {
	return usageErrorf("unsupported query field type")
}
