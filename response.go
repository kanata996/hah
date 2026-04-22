package hah

import "github.com/kanata996/hah/internal/resp"

// Response 表示默认 JSON 协议的可导出响应视图。
type Response = resp.Response

// ResponseError 表示默认协议中的错误对象。
type ResponseError = resp.ErrorBody

// SuccessResponse 导出默认成功响应视图。
func SuccessResponse(status int, data any) (*Response, error) {
	return resp.SuccessResponse(status, data)
}

// ErrorResponse 导出默认错误响应视图。
func ErrorResponse(err error, code ...int) (*Response, error) {
	return resp.ErrorResponse(err, code...)
}
