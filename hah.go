package hah

import (
	"net/http"

	"github.com/kanata996/hah/bind"
	"github.com/kanata996/hah/reqx"
	"github.com/kanata996/hah/resp"
)

type (
	// BindUnmarshaler 允许字段从单个字符串输入值自定义解码。
	BindUnmarshaler = bind.BindUnmarshaler
	// RequestValidator 允许 DTO 在 binding 之后声明请求级规则。
	RequestValidator = reqx.RequestValidator
	// Normalizer 允许 DTO 在校验前做标准化处理。
	Normalizer = reqx.Normalizer
	// ValueBinder 提供 path/query 单字段链式绑定与 source-aware violation 收集。
	ValueBinder = reqx.ValueBinder
	// ErrorResponder 协调错误收敛、错误响应写回与 5xx 独立错误日志。
	ErrorResponder = resp.ErrorResponder
)

// Bind 按默认顺序绑定请求数据：path -> query(GET/DELETE/HEAD) -> body。
func Bind(r *http.Request, target any) error {
	return bind.Bind(r, target)
}

// BindBody 只从请求 body 绑定数据。
func BindBody(r *http.Request, target any) error {
	return bind.BindBody(r, target)
}

// PathParam 按当前请求参数转换规则读取并解析单个 path 参数。
func PathParam[T any](r *http.Request, name string) (T, error) {
	return reqx.PathParam[T](r, name)
}

// QueryParam 按当前请求参数转换规则读取并解析单个 query 参数。
func QueryParam[T any](r *http.Request, name string) (T, error) {
	return reqx.QueryParam[T](r, name)
}

// PathValuesBinder 创建 path 参数值绑定器。
func PathValuesBinder(r *http.Request) *ValueBinder {
	return reqx.PathValuesBinder(r)
}

// QueryParamsBinder 创建 query 参数值绑定器。
func QueryParamsBinder(r *http.Request) *ValueBinder {
	return reqx.QueryParamsBinder(r)
}

// BindAndValidate 绑定后执行 Normalize、请求级规则和字段校验。
func BindAndValidate(r *http.Request, target any) error {
	return reqx.BindAndValidate(r, target)
}

// RequireBody 按默认 binder 契约要求请求必须显式提交 body。
func RequireBody(r *http.Request) error {
	return reqx.RequireBody(r)
}

// WriteError 按统一错误对象写回响应。
func WriteError(w http.ResponseWriter, r *http.Request, err error) error {
	return resp.WriteError(w, r, err)
}

// NewErrorResponder 返回默认错误响应器，可按需定制错误归一化与日志策略。
func NewErrorResponder() *ErrorResponder {
	return resp.NewErrorResponder()
}

// JSON 写回 JSON 响应。
func JSON(w http.ResponseWriter, status int, data any) error {
	return resp.JSON(w, status, data)
}

// JSONBlob 直接写回原始 JSON 字节。
func JSONBlob(w http.ResponseWriter, status int, body []byte) error {
	return resp.JSONBlob(w, status, body)
}

// OK 写回 200 成功响应。
func OK(w http.ResponseWriter, data any) error {
	return resp.OK(w, data)
}

// Created 写回 201 成功响应。
func Created(w http.ResponseWriter, data any) error {
	return resp.Created(w, data)
}

// NoContent 写回 204 成功响应。
func NoContent(w http.ResponseWriter) error {
	return resp.NoContent(w)
}
