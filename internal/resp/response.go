package resp

import (
	"fmt"
	"net/http"

	"github.com/kanata996/hah/internal/errx"
)

// Response 表示默认 JSON 协议对应的可导出响应视图。
type Response struct {
	Status  int        `json:"-"`
	Code    int        `json:"code"`
	Message string     `json:"message"`
	Data    any        `json:"data,omitempty"`
	Error   *ErrorBody `json:"error,omitempty"`
}

// ErrorBody 表示默认协议中的错误对象。
type ErrorBody struct {
	Reason  string            `json:"reason"`
	Details []errx.FieldError `json:"details,omitempty"`
}

// SuccessResponse 导出默认成功响应视图。
func SuccessResponse(status int, data any) (*Response, error) {
	if err := validateSuccessStatus(status); err != nil {
		return nil, err
	}
	return newSuccessResponse(status, data), nil
}

// ErrorResponse 导出默认错误响应视图。
func ErrorResponse(err error, code ...int) (*Response, error) {
	if err == nil {
		return nil, nil
	}

	topCode, topCodeSet, topCodeErr := normalizeTopCode(code)
	if topCodeErr != nil {
		return nil, topCodeErr
	}

	httpErr := errx.NormalizeHTTPError(err)
	if !topCodeSet {
		topCode = httpErr.Status() * defaultErrorCodeScale
	}

	return &Response{
		Status:  httpErr.Status(),
		Code:    topCode,
		Message: httpErr.Detail(),
		Error:   newErrorBody(httpErr),
	}, nil
}

func validateSuccessStatus(status int) error {
	switch status {
	case http.StatusOK, http.StatusAccepted, http.StatusCreated:
		return nil
	default:
		return fmt.Errorf("resp: unsupported default success status %d", status)
	}
}

func newSuccessResponse(status int, data any) *Response {
	return &Response{
		Status:  status,
		Code:    successTopCode,
		Message: successMessage,
		Data:    data,
	}
}
