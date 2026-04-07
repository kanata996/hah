package hah

import (
	"net/http"

	"github.com/kanata996/hah/reqx"
	"github.com/kanata996/hah/resp"
)

// DefaultMaxBodyBytes 是请求体默认读取上限。
const DefaultMaxBodyBytes = reqx.DefaultMaxBodyBytes

type (
	// BindOption 自定义绑定行为。
	BindOption = reqx.BindOption
	// BindBodyOption 自定义 body 绑定行为。
	BindBodyOption = reqx.BindBodyOption
	// BindQueryParamsOption 自定义 query 绑定行为。
	BindQueryParamsOption = reqx.BindQueryParamsOption
	// BindHeadersOption 自定义 header 绑定行为。
	BindHeadersOption = reqx.BindHeadersOption
	// Normalizer 允许 DTO 在校验前做标准化处理。
	Normalizer = reqx.Normalizer
)

// Bind 按 Echo 风格顺序绑定请求数据：path -> query(GET/DELETE) -> body。
func Bind[T any](r *http.Request, dst *T, opts ...BindOption) error {
	return reqx.Bind(r, dst, opts...)
}

// BindBody 只从请求 body 绑定数据。
func BindBody[T any](r *http.Request, dst *T, opts ...BindBodyOption) error {
	return reqx.BindBody(r, dst, opts...)
}

// BindQueryParams 只从 query 参数绑定数据。
func BindQueryParams[T any](r *http.Request, dst *T, opts ...BindQueryParamsOption) error {
	return reqx.BindQueryParams(r, dst, opts...)
}

// BindPathValues 只从 path 参数绑定数据。
func BindPathValues[T any](r *http.Request, dst *T) error {
	return reqx.BindPathValues(r, dst)
}

// BindHeaders 只从 header 绑定数据。
func BindHeaders[T any](r *http.Request, dst *T, opts ...BindHeadersOption) error {
	return reqx.BindHeaders(r, dst, opts...)
}

// BindAndValidate 绑定并执行 Normalize/validator 校验。
func BindAndValidate[T any](r *http.Request, dst *T, opts ...BindOption) error {
	return reqx.BindAndValidate(r, dst, opts...)
}

// BindAndValidateBody 从 body 绑定并执行校验。
func BindAndValidateBody[T any](r *http.Request, dst *T, opts ...BindBodyOption) error {
	return reqx.BindAndValidateBody(r, dst, opts...)
}

// BindAndValidateQuery 从 query 参数绑定并执行校验。
func BindAndValidateQuery[T any](r *http.Request, dst *T, opts ...BindQueryParamsOption) error {
	return reqx.BindAndValidateQuery(r, dst, opts...)
}

// BindAndValidatePath 从 path 参数绑定并执行校验。
func BindAndValidatePath[T any](r *http.Request, dst *T) error {
	return reqx.BindAndValidatePath(r, dst)
}

// BindAndValidateHeaders 从 header 绑定并执行校验。
func BindAndValidateHeaders[T any](r *http.Request, dst *T, opts ...BindHeadersOption) error {
	return reqx.BindAndValidateHeaders(r, dst, opts...)
}

// ParamString 读取必填 path 字符串参数。
func ParamString(r *http.Request, name string) (string, error) {
	return reqx.ParamString(r, name)
}

// ParamInt 读取必填 path 整数参数。
func ParamInt(r *http.Request, name string) (int, error) {
	return reqx.ParamInt(r, name)
}

// ParamUUID 读取并标准化必填 path UUID 参数。
func ParamUUID(r *http.Request, name string) (string, error) {
	return reqx.ParamUUID(r, name)
}

// WriteError 按统一错误对象写回响应。
func WriteError(w http.ResponseWriter, r *http.Request, err error) error {
	return resp.WriteError(w, r, err)
}

// JSON 按 Echo 风格写回 JSON 响应。
func JSON(w http.ResponseWriter, r *http.Request, status int, data any) error {
	return resp.JSON(w, r, status, data)
}

// JSONPretty 按指定缩进写回 pretty JSON 响应。
func JSONPretty(w http.ResponseWriter, r *http.Request, status int, data any, indent string) error {
	return resp.JSONPretty(w, r, status, data, indent)
}

// JSONBlob 直接写回原始 JSON 字节。
func JSONBlob(w http.ResponseWriter, r *http.Request, status int, body []byte) error {
	return resp.JSONBlob(w, r, status, body)
}

// OK 写回 200 成功响应。
func OK(w http.ResponseWriter, r *http.Request, data any) error {
	return resp.OK(w, r, data)
}

// Created 写回 201 成功响应。
func Created(w http.ResponseWriter, r *http.Request, data any) error {
	return resp.Created(w, r, data)
}

// NoContent 写回 204 成功响应。
func NoContent(w http.ResponseWriter, r *http.Request) error {
	return resp.NoContent(w, r)
}

// WithMaxBodyBytes 设置 body 读取上限。
func WithMaxBodyBytes(limit int64) BindOption {
	return reqx.WithMaxBodyBytes(limit)
}
