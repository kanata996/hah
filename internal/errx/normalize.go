package errx

import (
	"context"
	"errors"
	"net/http"
)

// NormalizeHTTPError 收敛任意错误链，返回可公开写回的 HTTPError。
func NormalizeHTTPError(err error) *HTTPError {
	if httpErr := findHTTPError(err); httpErr != nil {
		return httpErr
	}

	switch {
	case errors.Is(err, context.Canceled):
		return NewHTTPError(499, "", "")
	case errors.Is(err, context.DeadlineExceeded):
		return NewHTTPError(http.StatusGatewayTimeout, "", "")
	default:
		return NewHTTPError(http.StatusInternalServerError, "", "")
	}
}

func findHTTPError(err error) *HTTPError {
	if err == nil {
		return nil
	}

	var httpErr *HTTPError
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
