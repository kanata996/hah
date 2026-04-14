package resp

import (
	"context"
	"errors"
	"net/http"

	"github.com/kanata996/hah/errx"
)

// asHTTPError 把任意 error 适配为 HTTPError。
// 这是错误响应语义的收敛点，负责得到最终 status/code/detail/errors。
//
// 适配顺序：
//   - 已经是 HTTPError，直接返回；
//   - context.Canceled / context.DeadlineExceeded 走固定 HTTP 语义；
//   - 其余错误统一视为内部错误。
func asHTTPError(err error) *errx.HTTPError {
	if err == nil {
		return nil
	}

	var httpErr *errx.HTTPError
	if errors.As(err, &httpErr) && httpErr != nil {
		return httpErr
	}

	switch {
	case errors.Is(err, context.Canceled):
		return errx.NewHTTPErrorWithCause(499, "client_closed_request", "Client Closed Request", err)
	case errors.Is(err, context.DeadlineExceeded):
		return errx.NewHTTPErrorWithCause(http.StatusGatewayTimeout, "timeout", "", err)
	}

	return errx.NewHTTPErrorWithCause(http.StatusInternalServerError, "", "", err)
}
