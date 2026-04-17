package hah

import (
	"net/http"

	"github.com/kanata996/hah/errx"
	"github.com/kanata996/hah/reqx"
	"github.com/kanata996/hah/resp"
)

const (
	CodeInvalid  = errx.CodeInvalid
	CodeRequired = errx.CodeRequired
	CodeUnknown  = errx.CodeUnknown
	CodeType     = errx.CodeType
	CodeMultiple = errx.CodeMultiple
)

const (
	InBody   = errx.InBody
	InQuery  = errx.InQuery
	InPath   = errx.InPath
	InHeader = errx.InHeader
)

type (
	// BindUnmarshaler 允许字段从单个字符串输入值自定义解码。
	BindUnmarshaler = reqx.BindUnmarshaler
	// BindMultipleUnmarshaler 允许字段一次性接收同名输入的全部值。
	BindMultipleUnmarshaler = reqx.BindMultipleUnmarshaler
	// Violation 描述单个公开请求违规。
	Violation = errx.Violation
	// HTTPError 表示 HTTP 边界上的公共错误。
	HTTPError = errx.HTTPError
)

// BindBody 只从请求 body 绑定数据。
//
// 解码直接作用在调用方传入的 target 上；JSON 中缺失的字段不会覆盖 target
// 已有值。
func BindBody(r *http.Request, target any) error {
	return reqx.BindBody(r, target)
}

// BindQuery 只从 query 参数绑定数据。
func BindQuery(r *http.Request, target any) error {
	return reqx.BindQuery(r, target)
}

// Path 创建 path 单参数读取与校验 builder。
func Path(r *http.Request, name string) *reqx.PathParam {
	return reqx.Path(r, name)
}

// Query 创建 query 单参数读取与校验 builder。
func Query(r *http.Request, name string) *reqx.QueryParam {
	return reqx.Query(r, name)
}

// RequireBody 按默认 binder 契约要求请求必须显式提交 body。
//
// 它和 BindBody 共享同一个非破坏性 body 探测，因此可按调用方需要在绑定前后调用。
func RequireBody(r *http.Request) error {
	return reqx.RequireBody(r)
}

// InvalidRequest 生成统一的 invalid_request 错误包络。
func InvalidRequest(violations ...Violation) error {
	return reqx.InvalidRequest(violations...)
}

// NewHTTPError 构造一个不带底层 cause 的公共 HTTP 错误。
func NewHTTPError(status int, code, detail string) *HTTPError {
	return errx.NewHTTPError(status, code, detail)
}

// NewHTTPErrorWithCause 基于给定 cause 构造公共 HTTP 错误。
func NewHTTPErrorWithCause(status int, code, detail string, cause error) *HTTPError {
	return errx.NewHTTPErrorWithCause(status, code, detail, cause)
}

// WriteError 按统一错误对象写回响应。
func WriteError(w http.ResponseWriter, err error) error {
	return resp.WriteError(w, err)
}

// JSON 写回 JSON 响应。
func JSON(w http.ResponseWriter, status int, data any) error {
	return resp.JSON(w, status, data)
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
