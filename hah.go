package hah

import (
	"net/http"

	"github.com/kanata996/hah/reqx"
	"github.com/kanata996/hah/resp"
)

type (
	// BindUnmarshaler 允许字段从单个字符串输入值自定义解码。
	BindUnmarshaler = reqx.BindUnmarshaler
	// BindMultipleUnmarshaler 允许字段一次性接收同名输入的全部值。
	BindMultipleUnmarshaler = reqx.BindMultipleUnmarshaler
	// PathParam 表示一个待解析的 path 单参数。
	PathParam = reqx.PathParam
	// QueryParam 表示一个待解析的 query 单参数。
	QueryParam = reqx.QueryParam
)

// BindBody 只从请求 body 绑定数据。
func BindBody(r *http.Request, target any) error {
	return reqx.BindBody(r, target)
}

// BindQuery 只从 query 参数绑定数据。
func BindQuery(r *http.Request, target any) error {
	return reqx.BindQuery(r, target)
}

// Path 创建 path 单参数读取与校验 builder。
func Path(r *http.Request, name string) *PathParam {
	return reqx.Path(r, name)
}

// Query 创建 query 单参数读取与校验 builder。
func Query(r *http.Request, name string) *QueryParam {
	return reqx.Query(r, name)
}

// RequireBody 按默认 binder 契约要求请求必须显式提交 body。
func RequireBody(r *http.Request) error {
	return reqx.RequireBody(r)
}

// WriteError 按统一错误对象写回响应。
func WriteError(w http.ResponseWriter, err error) error {
	return resp.WriteError(w, err)
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
