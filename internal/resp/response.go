package resp

import (
	"errors"
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

const (
	successTopCode = 0
	successMessage = "success"
)

const (
	defaultErrorCodeScale = 100
	minExplicitErrorCode  = 10000
	maxExplicitErrorCode  = 99999
)

// SuccessResponse 导出默认成功响应视图。
func SuccessResponse(status int, data any) (*Response, error) {
	switch status {
	case http.StatusOK, http.StatusAccepted, http.StatusCreated:
		return &Response{
			Status:  status,
			Code:    successTopCode,
			Message: successMessage,
			Data:    data,
		}, nil
	default:
		return nil, fmt.Errorf("resp: unsupported default success status %d", status)
	}
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
		Error: &ErrorBody{
			Reason:  httpErr.Code(),
			Details: httpErr.Errors(),
		},
	}, nil
}

func normalizeTopCode(code []int) (value int, ok bool, err error) {
	switch len(code) {
	case 0:
		return 0, false, nil
	case 1:
		if code[0] < minExplicitErrorCode || code[0] > maxExplicitErrorCode {
			return 0, false, fmt.Errorf("resp: invalid top-level error code %d", code[0])
		}
		return code[0], true, nil
	default:
		return 0, false, errors.New("resp: WriteError accepts at most one top-level error code")
	}
}
