package bind

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"
)

// 本文件负责 bind 包的公开 API 入口、默认绑定阶段编排，以及共享前置校验。
//
// 这里集中放：
//   - 对外公开的核心入口：Bind、BindBody、BindQueryParams、BindPathValues、BindHeaders
//   - 默认 binder 的阶段顺序：path -> query(GET/DELETE/HEAD) -> body
//   - 绑定目标的公共前置校验

const defaultMaxBodyBytes int64 = 1 << 20

const (
	// CodeInvalidJSON 表示请求 body 不是合法 JSON。
	CodeInvalidJSON = "invalid_json"
	// CodeUnsupportedMediaType 表示请求 body 的 Content-Type 不受支持。
	CodeUnsupportedMediaType = "unsupported_media_type"
	// CodeRequestTooLarge 表示请求 body 超出默认大小限制。
	CodeRequestTooLarge = "request_too_large"
)

// BindUnmarshaler 允许字段从单个字符串输入值自定义解码。
type BindUnmarshaler interface {
	UnmarshalParam(param string) error
}

// Bind 按默认顺序绑定请求数据：path -> query(GET/DELETE/HEAD) -> body。
func Bind(r *http.Request, target any) error {
	return bindDefault(r, target)
}

// BindBody 只从请求 body 绑定数据。
func BindBody(r *http.Request, target any) error {
	if err := validateBindInputs(r, target); err != nil {
		return err
	}

	return bindBody(r, target)
}

// BindQueryParams 只从 query 参数绑定数据。
func BindQueryParams(r *http.Request, target any) error {
	if err := validateBindInputs(r, target); err != nil {
		return err
	}

	return bindQueryParamsDefault(r, target)
}

// BindPathValues 只从 path 参数绑定数据。
func BindPathValues(r *http.Request, target any) error {
	if err := validateBindInputs(r, target); err != nil {
		return err
	}

	return bindPathValuesDefault(r, target)
}

// BindHeaders 只从 header 绑定数据。
func BindHeaders(r *http.Request, target any) error {
	if err := validateBindInputs(r, target); err != nil {
		return err
	}

	return bindHeadersDefault(r, target)
}

// bindDefault 负责串联默认 binder 的各个阶段。
func bindDefault(r *http.Request, target any) error {
	if err := validateBindInputs(r, target); err != nil {
		return err
	}

	// path 总是先执行，为 query/body 提供可覆盖的基础值。
	if err := bindPathValuesDefault(r, target); err != nil {
		return err
	}

	method := strings.ToUpper(strings.TrimSpace(r.Method))
	// 只有默认允许从 URL 读取语义参数的方法才进入 query 阶段。
	if method == http.MethodGet || method == http.MethodDelete || method == http.MethodHead {
		if err := bindQueryParamsDefault(r, target); err != nil {
			return err
		}
	}

	// body 最后执行，因此它对同名字段拥有最高优先级。
	return bindBody(r, target)
}

// validateBindInputs 统一校验公开 Bind* 入口和内部阶段共享的前置条件。
func validateBindInputs(r *http.Request, target any) error {
	if r == nil {
		return errorsf("request must not be nil")
	}
	return validateBindingDestination(target)
}

// validateBindingDestination 统一校验绑定目标必须是非 nil 指针。
func validateBindingDestination(target any) error {
	if target == nil {
		return errorsf("destination must not be nil")
	}
	value := reflect.ValueOf(target)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		return errorsf("destination must not be nil")
	}
	return nil
}

func errorsf(format string, args ...any) error {
	return fmt.Errorf("bind: "+format, args...)
}
