package hah

import (
	"net/http"

	"github.com/kanata996/hah/internal/errx"
	"github.com/kanata996/hah/internal/resp"
	"github.com/kanata996/hah/reqx"
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
	// Violation 描述单个公开请求违规。
	Violation = errx.Violation
	// HTTPError 表示 HTTP 边界上的公共错误。
	HTTPError = errx.HTTPError
)

// BindBody 只从请求 body 绑定数据。
//
// 绑定会先解码到临时值，成功后再一次性提交到 target。
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

// BadRequest 构造 400 Bad Request 公共错误。
func BadRequest(code, detail string) *HTTPError {
	return errx.BadRequest(code, detail)
}

// Unauthorized 构造 401 Unauthorized 公共错误。
func Unauthorized(code, detail string) *HTTPError {
	return errx.Unauthorized(code, detail)
}

// Forbidden 构造 403 Forbidden 公共错误。
func Forbidden(code, detail string) *HTTPError {
	return errx.Forbidden(code, detail)
}

// NotFound 构造 404 Not Found 公共错误。
func NotFound(code, detail string) *HTTPError {
	return errx.NotFound(code, detail)
}

// MethodNotAllowed 构造 405 Method Not Allowed 公共错误。
func MethodNotAllowed(code, detail string) *HTTPError {
	return errx.MethodNotAllowed(code, detail)
}

// Conflict 构造 409 Conflict 公共错误。
func Conflict(code, detail string) *HTTPError {
	return errx.Conflict(code, detail)
}

// UnprocessableEntity 构造 422 Unprocessable Entity 公共错误。
func UnprocessableEntity(code, detail string) *HTTPError {
	return errx.UnprocessableEntity(code, detail)
}

// TooManyRequests 构造 429 Too Many Requests 公共错误。
func TooManyRequests(code, detail string) *HTTPError {
	return errx.TooManyRequests(code, detail)
}

// WriteError 按统一错误对象写回响应。
func WriteError(w http.ResponseWriter, err error, code ...int) error {
	return resp.WriteError(w, err, code...)
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
