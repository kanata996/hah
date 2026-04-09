package hah

import (
	"net/http"

	"github.com/kanata996/hah/bind"
	"github.com/kanata996/hah/reqx"
	"github.com/kanata996/hah/resp"
)

type (
	// Binder 定义默认请求绑定器接口。
	Binder = bind.Binder
	// DefaultBinder 是默认请求绑定器实现。
	DefaultBinder = bind.DefaultBinder
	// BindUnmarshaler 允许字段从单个字符串输入值自定义解码。
	BindUnmarshaler = bind.BindUnmarshaler
	// RequestValidator 允许 DTO 在 binding 之后声明请求级规则。
	RequestValidator = reqx.RequestValidator
	// Normalizer 允许 DTO 在校验前做标准化处理。
	Normalizer = reqx.Normalizer
)

// Bind 按默认顺序绑定请求数据：path -> query(GET/DELETE/HEAD) -> body。
func Bind(r *http.Request, target any) error {
	return bind.Bind(r, target)
}

// BindBody 只从请求 body 绑定数据。
func BindBody(r *http.Request, target any) error {
	return bind.BindBody(r, target)
}

// BindQueryParams 只从 query 参数绑定数据。
func BindQueryParams(r *http.Request, target any) error {
	return bind.BindQueryParams(r, target)
}

// BindPathValues 只从 path 参数绑定数据。
func BindPathValues(r *http.Request, target any) error {
	return bind.BindPathValues(r, target)
}

// BindHeaders 只从 header 绑定数据。
func BindHeaders(r *http.Request, target any) error {
	return bind.BindHeaders(r, target)
}

// BindAndValidate 绑定后执行 Normalize、请求级规则和字段校验。
func BindAndValidate(r *http.Request, target any) error {
	return reqx.BindAndValidate(r, target)
}

// BindAndValidateBody 从 body 绑定并执行校验。
func BindAndValidateBody(r *http.Request, target any) error {
	return reqx.BindAndValidateBody(r, target)
}

// BindAndValidateQuery 从 query 参数绑定并执行校验。
func BindAndValidateQuery(r *http.Request, target any) error {
	return reqx.BindAndValidateQuery(r, target)
}

// BindAndValidatePath 从 path 参数绑定并执行校验。
func BindAndValidatePath(r *http.Request, target any) error {
	return reqx.BindAndValidatePath(r, target)
}

// BindAndValidateHeaders 从 header 绑定并执行校验。
func BindAndValidateHeaders(r *http.Request, target any) error {
	return reqx.BindAndValidateHeaders(r, target)
}

// RequireBody 按默认 binder 契约要求请求必须显式提交 body。
func RequireBody(r *http.Request) error {
	return reqx.RequireBody(r)
}

// WriteError 按统一错误对象写回响应。
func WriteError(w http.ResponseWriter, r *http.Request, err error) error {
	return resp.WriteError(w, r, err)
}

// JSON 写回 JSON 响应。
func JSON(w http.ResponseWriter, r *http.Request, status int, data any) error {
	return resp.JSON(w, r, status, data)
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
