package resp

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/kanata996/hah/internal/errx"
)

const (
	defaultTopCodeScale = 100
	minExplicitTopCode  = 10000
	maxExplicitTopCode  = 99999
)

type errorBody struct {
	Reason  string            `json:"reason"`
	Details []errx.FieldError `json:"details,omitempty"`
}

// WriteError 是 HTTP 错误写回的统一入口。
func WriteError(w http.ResponseWriter, err error, topCode ...int) error {
	if err == nil {
		return nil
	}

	resolvedTopCode, topCodeSet, topCodeErr := normalizeTopCode(topCode)
	if topCodeErr != nil {
		return topCodeErr
	}

	if w == nil {
		return errNilResponseWriter
	}

	httpErr := errx.NormalizeHTTPError(err)
	if !topCodeSet {
		resolvedTopCode = httpErr.Status() * defaultTopCodeScale
	}

	body, encodeErr := encodeJSON(responseEnvelope{
		Code:    resolvedTopCode,
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

func normalizeTopCode(topCode []int) (value int, ok bool, err error) {
	switch len(topCode) {
	case 0:
		return 0, false, nil
	case 1:
		if topCode[0] < minExplicitTopCode || topCode[0] > maxExplicitTopCode {
			return 0, false, fmt.Errorf("resp: invalid top-level error code %d", topCode[0])
		}
		return topCode[0], true, nil
	default:
		return 0, false, errors.New("resp: WriteError accepts at most one top-level error code")
	}
}
