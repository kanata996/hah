package resp

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/kanata996/hah/internal/errx"
)

const (
	defaultErrorCodeScale = 100
	minExplicitErrorCode  = 10000
	maxExplicitErrorCode  = 99999
)

type errorBody struct {
	Reason  string            `json:"reason"`
	Details []errx.FieldError `json:"details,omitempty"`
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

	httpErr := errx.NormalizeHTTPError(err)
	if !topCodeSet {
		topCode = httpErr.Status() * defaultErrorCodeScale
	}

	body, encodeErr := encodeJSON(responseEnvelope{
		Code:    topCode,
		Message: httpErr.Detail(),
		Error: &errorBody{
			Reason:  httpErr.Code(),
			Details: httpErr.Errors(),
		},
	})
	if encodeErr != nil {
		return encodeErr
	}

	return writeJSONBytes(w, httpErr.Status(), body)
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
