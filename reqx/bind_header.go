package reqx

import "net/http"

func BindHeaders[T any](r *http.Request, dst *T, opts ...BindHeadersOption) error {
	cfg := applyBindOptions(opts...)
	return bindTaggedValues(r, dst, headerSource, cfg.header)
}
