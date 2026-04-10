package bind

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"
)

// 本文件负责 bind 包的公开 API 入口、默认 binder 的阶段编排，以及共享基础配置。
//
// 这里集中放：
//   - 对外公开的核心入口：Bind、BindBody、BindQueryParams、BindPathValues、BindHeaders
//   - 对外公开的核心类型：Binder、DefaultBinder、BindUnmarshaler
//   - 默认 binder 的阶段顺序：path -> query(GET/DELETE/HEAD) -> body
//   - 绑定目标的公共前置校验和默认配置

const defaultMaxBodyBytes int64 = 1 << 20

const (
	mimeApplicationJSON = "application/json"
)

const (
	// CodeInvalidJSON 表示请求 body 不是合法 JSON。
	CodeInvalidJSON = "invalid_json"
	// CodeUnsupportedMediaType 表示请求 body 的 Content-Type 不受支持。
	CodeUnsupportedMediaType = "unsupported_media_type"
	// CodeRequestTooLarge 表示请求 body 超出默认大小限制。
	CodeRequestTooLarge = "request_too_large"
)

// Binder 定义默认请求绑定器接口。
type Binder interface {
	Bind(r *http.Request, target any) error
}

// DefaultBinder 是面向 JSON API 的默认绑定器。
type DefaultBinder struct{}

// BindUnmarshaler 允许字段从单个字符串输入值自定义解码。
type BindUnmarshaler interface {
	UnmarshalParam(param string) error
}

type bindBodyConfig struct {
	maxBodyBytes       int64
	allowUnknownFields bool
}

type bindConfig struct {
	body bindBodyConfig
}

func defaultBindConfig() bindConfig {
	return bindConfig{
		body: bindBodyConfig{
			maxBodyBytes:       defaultMaxBodyBytes,
			allowUnknownFields: true,
		},
	}
}

// Bind 按默认顺序绑定请求数据：path -> query(GET/DELETE/HEAD) -> body。
func Bind(r *http.Request, target any) error {
	if r == nil {
		return errorsf("request must not be nil")
	}
	if target == nil {
		return errorsf("destination must not be nil")
	}

	return bindWithConfig(r, target, defaultBindConfig())
}

// BindBody 只从请求 body 绑定数据。
func BindBody(r *http.Request, target any) error {
	if r == nil {
		return errorsf("request must not be nil")
	}
	if target == nil {
		return errorsf("destination must not be nil")
	}

	return bindBodyDefault(r, target, defaultBindConfig().body)
}

// BindQueryParams 只从 query 参数绑定数据。
func BindQueryParams(r *http.Request, target any) error {
	if r == nil {
		return errorsf("request must not be nil")
	}
	if target == nil {
		return errorsf("destination must not be nil")
	}

	return bindQueryParamsDefault(r, target)
}

// BindPathValues 只从 path 参数绑定数据。
func BindPathValues(r *http.Request, target any) error {
	if r == nil {
		return errorsf("request must not be nil")
	}
	if target == nil {
		return errorsf("destination must not be nil")
	}

	return bindPathValuesDefault(r, target)
}

// BindHeaders 只从 header 绑定数据。
func BindHeaders(r *http.Request, target any) error {
	if r == nil {
		return errorsf("request must not be nil")
	}
	if target == nil {
		return errorsf("destination must not be nil")
	}

	return bindHeadersDefault(r, target)
}

// Bind 实现 Binder 接口，使用 bind 包默认的绑定顺序和 body 契约。
func (b *DefaultBinder) Bind(r *http.Request, target any) error {
	return bindWithConfig(r, target, defaultBindConfig())
}

// bindWithConfig 负责串联默认 binder 的各个阶段。
func bindWithConfig(r *http.Request, target any, cfg bindConfig) error {
	if r == nil {
		return errorsf("request must not be nil")
	}
	if err := validateBindingDestination(target); err != nil {
		return err
	}

	if err := bindPathValuesDefault(r, target); err != nil {
		return err
	}

	method := strings.ToUpper(strings.TrimSpace(r.Method))
	if method == http.MethodGet || method == http.MethodDelete || method == http.MethodHead {
		if err := bindQueryParamsDefault(r, target); err != nil {
			return err
		}
	}

	return bindBodyDefault(r, target, cfg.body)
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
