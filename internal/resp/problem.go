package resp

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/kanata996/hah/internal/errx"
)

const errorCodeBase = 1000

type errorBody struct {
	Title  string       `json:"title"`
	Status int          `json:"status"`
	Code   string       `json:"code"`
	Fields []fieldError `json:"fields,omitempty"`
}

type fieldError struct {
	Field   string `json:"field"`
	In      string `json:"in,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

var encodeErrorEnvelope = func(env responseEnvelope) ([]byte, error) {
	return encodeJSON(env)
}

// WriteError 是 HTTP 错误写回的统一入口。
func WriteError(w http.ResponseWriter, err error, code ...int) error {
	if err == nil {
		return nil
	}

	topCode, topCodeSet, topCodeErr := normalizeTopCode(code)
	if topCodeErr != nil {
		return topCodeErr
	}

	if w == nil {
		return errNilResponseWriter
	}

	httpErr := normalizeHTTPError(err)
	if !topCodeSet {
		topCode = httpErr.Status() * errorCodeBase
	}

	body, encodeErr := encodeErrorEnvelope(responseEnvelope{
		Code:    topCode,
		Message: deriveErrorMessage(httpErr),
		Error:   newErrorBody(httpErr),
	})
	if encodeErr != nil {
		return encodeErr
	}

	return writePreparedJSONBytes(w, httpErr.Status(), jsonContentType, body)
}

func normalizeTopCode(code []int) (value int, ok bool, err error) {
	switch len(code) {
	case 0:
		return 0, false, nil
	case 1:
		if code[0] <= 0 {
			return 0, false, fmt.Errorf("resp: invalid top-level error code %d", code[0])
		}
		return code[0], true, nil
	default:
		return 0, false, errors.New("resp: WriteError accepts at most one top-level error code")
	}
}

func normalizeHTTPError(err error) *errx.HTTPError {
	if httpErr := findHTTPError(err); httpErr != nil {
		return httpErr
	}

	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, context.Canceled):
		status = 499
	case errors.Is(err, context.DeadlineExceeded):
		status = http.StatusGatewayTimeout
	}

	return errx.NewHTTPError(status, "", "")
}

func findHTTPError(err error) *errx.HTTPError {
	if err == nil {
		return nil
	}

	var httpErr *errx.HTTPError
	if errors.As(err, &httpErr) && httpErr != nil {
		return httpErr
	}

	switch wrapped := err.(type) {
	case interface{ Unwrap() []error }:
		for _, child := range wrapped.Unwrap() {
			if httpErr := findHTTPError(child); httpErr != nil {
				return httpErr
			}
		}
	case interface{ Unwrap() error }:
		return findHTTPError(wrapped.Unwrap())
	}

	return nil
}

func deriveErrorMessage(httpErr *errx.HTTPError) string {
	return httpErr.Detail()
}

func newErrorBody(httpErr *errx.HTTPError) *errorBody {
	violations := httpErr.Errors()
	fields := make([]fieldError, 0, len(violations))
	for _, violation := range violations {
		fields = append(fields, fieldError{
			Field:   violation.Field,
			In:      string(violation.In),
			Code:    string(violation.Code),
			Message: violation.Detail,
		})
	}

	return &errorBody{
		Title:  httpErr.Title(),
		Status: httpErr.Status(),
		Code:   httpErr.Code(),
		Fields: fields,
	}
}
