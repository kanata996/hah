package errx

import (
	"net/http"
	"reflect"
	"strings"
)

// HTTPError 表示 HTTP 边界上的公共错误。
// cause 仅用于内部保留原始错误链，真正返回给客户端的是
// title/status/detail/code/errors。
type HTTPError struct {
	// status 始终收敛到“可公开返回的错误状态码”范围内。
	status int
	// code 是机器可读错误码；构造时会做 trim 和缺省补齐。
	code string
	// detail 是公开错误详情；缺省时会回退到稳定的标题文案。
	detail string
	// errors 是公开结构化错误详情列表；构造与读取时都会对公开 JSON 容器做递归拷贝。
	errors []any
	// cause 仅用于内部错误链，不直接暴露给客户端。
	cause error
}

// NewHTTPError 构造一个不带底层 cause 的公共 HTTP 错误。
// 它会统一补齐默认 status/code/detail，并对 errors 做防御性拷贝。
func NewHTTPError(status int, code, detail string, errors ...any) *HTTPError {
	return NewHTTPErrorWithCause(status, code, detail, nil, errors...)
}

// NewHTTPErrorWithCause 基于给定 cause 构造公共 HTTP 错误。
// 非法状态码会被归一化，缺省 code/detail 也会按状态码补全。
// 这里在入口一次性做标准化，保证后续使用时不会因为原始输入脏数据而漂移。
func NewHTTPErrorWithCause(status int, code, detail string, cause error, errors ...any) *HTTPError {
	status = normalizeErrorStatus(status)
	return &HTTPError{
		status: status,
		code:   normalizeErrorCode(status, code),
		detail: normalizeErrorDetail(status, detail),
		errors: cloneErrors(errors),
		cause:  cause,
	}
}

// Error 实现 error 接口。
// 若保留了底层 cause，则优先返回 cause 的文本，便于日志和 errors.Is/As 诊断；
// 若 cause 文本为空白或其 Error() 实现不安全，则回退为当前对象稳定的公开 Detail。
func (e *HTTPError) Error() (message string) {
	if e == nil {
		return ""
	}
	if e.cause != nil {
		defer func() {
			if recover() != nil {
				message = e.Detail()
			}
		}()
		if message = strings.TrimSpace(e.cause.Error()); message != "" {
			return message
		}
	}
	return e.Detail()
}

// Unwrap 暴露底层 cause，便于调用方继续使用 errors.Is / errors.As。
func (e *HTTPError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// Status 返回可公开返回的 HTTP 错误状态码。
// 即使内部字段被错误写入，也会再次收敛到安全范围。
func (e *HTTPError) Status() int {
	if e == nil {
		return http.StatusInternalServerError
	}
	return normalizeErrorStatus(e.status)
}

// Code 返回机器可读错误码。
// 若构造时未显式提供，或内部字段被写成空白值，会按最终状态码补齐默认值。
func (e *HTTPError) Code() string {
	if e == nil {
		return normalizeErrorCode(http.StatusInternalServerError, "")
	}
	return normalizeErrorCode(e.Status(), e.code)
}

// Title 返回公开错误标题。
// 这里不读取 detail/cause，只取“状态码对应的稳定标题”。
func (e *HTTPError) Title() string {
	if e == nil {
		return normalizeErrorTitle(http.StatusInternalServerError)
	}
	return normalizeErrorTitle(e.Status())
}

// Detail 返回公开错误详情。
// 若 detail 为空白，则回退到与 Title 对齐的稳定默认文案。
func (e *HTTPError) Detail() string {
	if e == nil {
		return normalizeErrorDetail(http.StatusInternalServerError, "")
	}
	return normalizeErrorDetail(e.Status(), e.detail)
}

// Errors 返回公开结构化错误详情列表的防御性拷贝。
// 调用方修改返回切片时，不会影响已构造的 HTTPError。
func (e *HTTPError) Errors() []any {
	if e == nil || len(e.errors) == 0 {
		return nil
	}
	return cloneErrors(e.errors)
}

// BadRequest 构造 400 Bad Request 公共错误。
func BadRequest(code, detail string, errors ...any) *HTTPError {
	return NewHTTPError(http.StatusBadRequest, code, detail, errors...)
}

// Unauthorized 构造 401 Unauthorized 公共错误。
func Unauthorized(code, detail string, errors ...any) *HTTPError {
	return NewHTTPError(http.StatusUnauthorized, code, detail, errors...)
}

// Forbidden 构造 403 Forbidden 公共错误。
func Forbidden(code, detail string, errors ...any) *HTTPError {
	return NewHTTPError(http.StatusForbidden, code, detail, errors...)
}

// NotFound 构造 404 Not Found 公共错误。
func NotFound(code, detail string, errors ...any) *HTTPError {
	return NewHTTPError(http.StatusNotFound, code, detail, errors...)
}

// MethodNotAllowed 构造 405 Method Not Allowed 公共错误。
func MethodNotAllowed(code, detail string, errors ...any) *HTTPError {
	return NewHTTPError(http.StatusMethodNotAllowed, code, detail, errors...)
}

// Conflict 构造 409 Conflict 公共错误。
func Conflict(code, detail string, errors ...any) *HTTPError {
	return NewHTTPError(http.StatusConflict, code, detail, errors...)
}

// UnprocessableEntity 构造 422 Unprocessable Entity 公共错误。
func UnprocessableEntity(code, detail string, errors ...any) *HTTPError {
	return NewHTTPError(http.StatusUnprocessableEntity, code, detail, errors...)
}

// TooManyRequests 构造 429 Too Many Requests 公共错误。
func TooManyRequests(code, detail string, errors ...any) *HTTPError {
	return NewHTTPError(http.StatusTooManyRequests, code, detail, errors...)
}

// cloneErrors 返回 errors 的递归拷贝，避免公开 JSON 容器与调用方共享状态。
// 这里只处理 map/slice/array 这类容器，并保留其原始动态类型；其他值保持原样。
func cloneErrors(errors []any) []any {
	if len(errors) == 0 {
		return nil
	}

	cloned := make([]any, len(errors))
	seen := make(map[cloneVisit]reflect.Value)
	for i, value := range errors {
		cloned[i] = cloneErrorInterface(value, seen)
	}
	return cloned
}

type cloneVisit struct {
	typ reflect.Type
	ptr uintptr
	len int
}

func cloneErrorInterface(value any, seen map[cloneVisit]reflect.Value) any {
	cloned := cloneErrorValue(reflect.ValueOf(value), seen)
	if !cloned.IsValid() {
		return nil
	}
	return cloned.Interface()
}

func cloneErrorValue(value reflect.Value, seen map[cloneVisit]reflect.Value) reflect.Value {
	if !value.IsValid() {
		return reflect.Value{}
	}

	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		return cloneErrorValue(value.Elem(), seen)
	case reflect.Map:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		visit := cloneVisit{typ: value.Type(), ptr: value.Pointer()}
		if cloned, ok := seen[visit]; ok {
			return cloned
		}
		cloned := reflect.MakeMapWithSize(value.Type(), value.Len())
		seen[visit] = cloned
		iter := value.MapRange()
		for iter.Next() {
			cloned.SetMapIndex(iter.Key(), cloneAssignableValue(iter.Value(), value.Type().Elem(), seen))
		}
		return cloned
	case reflect.Slice:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		visit := cloneVisit{typ: value.Type(), ptr: value.Pointer(), len: value.Len()}
		if cloned, ok := seen[visit]; ok {
			return cloned
		}
		cloned := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		seen[visit] = cloned
		for i := 0; i < value.Len(); i++ {
			cloned.Index(i).Set(cloneAssignableValue(value.Index(i), value.Type().Elem(), seen))
		}
		return cloned
	case reflect.Array:
		cloned := reflect.New(value.Type()).Elem()
		for i := 0; i < value.Len(); i++ {
			cloned.Index(i).Set(cloneAssignableValue(value.Index(i), value.Type().Elem(), seen))
		}
		return cloned
	default:
		return value
	}
}

func cloneAssignableValue(value reflect.Value, target reflect.Type, seen map[cloneVisit]reflect.Value) reflect.Value {
	cloned := cloneErrorValue(value, seen)
	if !cloned.IsValid() {
		return reflect.Zero(target)
	}
	if cloned.Type().AssignableTo(target) {
		return cloned
	}
	return value
}

// normalizeErrorStatus 把非法、未知或越界状态码收敛到 500。
// 这里仅允许标准 4xx/5xx，以及显式白名单的 499。
func normalizeErrorStatus(status int) int {
	if status == 499 {
		return status
	}
	if status < 400 || status > 599 {
		return http.StatusInternalServerError
	}
	if http.StatusText(status) == "" {
		return http.StatusInternalServerError
	}
	return status
}

// normalizeErrorCode 根据状态码补齐默认机器可读错误码。
func normalizeErrorCode(status int, code string) string {
	if trimmed := strings.TrimSpace(code); trimmed != "" {
		return trimmed
	}
	status = normalizeErrorStatus(status)

	switch status {
	case 499:
		return "client_closed_request"
	case http.StatusBadRequest:
		return "bad_request"
	case http.StatusUnauthorized:
		return "unauthorized"
	case http.StatusForbidden:
		return "forbidden"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusMethodNotAllowed:
		return "method_not_allowed"
	case http.StatusConflict:
		return "conflict"
	case http.StatusUnprocessableEntity:
		return "unprocessable_entity"
	case http.StatusTooManyRequests:
		return "too_many_requests"
	// 503/504 通常由基础设施层（网关/反向代理）生成，业务代码不常直接构造，
	// 因此只保留标准化映射，不提供快捷构造器。
	case http.StatusServiceUnavailable:
		return "service_unavailable"
	case http.StatusGatewayTimeout:
		return "timeout"
	default:
		if status >= 500 {
			return "internal_error"
		}
		return "client_error"
	}
}

// normalizeErrorTitle 根据状态码生成公开错误标题。
// 对标准状态优先使用 net/http 的状态文本；对 499 等非标准状态做显式补齐。
func normalizeErrorTitle(status int) string {
	status = normalizeErrorStatus(status)
	switch status {
	case 499:
		return "Client Closed Request"
	}
	if text := http.StatusText(status); text != "" {
		return text
	}
	return http.StatusText(http.StatusInternalServerError)
}

// normalizeErrorDetail 根据状态码补齐默认公共错误详情。
// 显式 detail 优先；若为空白则与 Title 保持一致，避免对外语义出现空 detail。
func normalizeErrorDetail(status int, detail string) string {
	if trimmed := strings.TrimSpace(detail); trimmed != "" {
		return trimmed
	}
	return normalizeErrorTitle(status)
}
