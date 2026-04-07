package reqx

import (
	"net/http"
	"strings"
)

// Bind 按 Echo 的默认顺序绑定请求数据：path -> query(GET/DELETE) -> body。
func Bind[T any](r *http.Request, dst *T, opts ...BindOption) error {
	if r == nil {
		return errorsf("request must not be nil")
	}

	cfg := applyBindOptions(opts...)
	return bindIntoClone(dst, func(bound *T) error {
		if err := bindTaggedValuesInPlace(r, bound, pathSource, bindValuesConfig{allowUnknownFields: true}); err != nil {
			return err
		}
		switch strings.ToUpper(strings.TrimSpace(r.Method)) {
		case http.MethodGet, http.MethodDelete:
			if err := bindTaggedValuesInPlace(r, bound, querySource, cfg.query); err != nil {
				return err
			}
		}

		bodyCfg := cfg.body
		bodyCfg.allowEmptyBody = true

		return bindJSONWithConfig(r, bound, bodyCfg, bodyBindMode{
			ignoreEmptyBody:            true,
			validateContentTypeOnEmpty: false,
		})
	})
}

func bindIntoClone[T any](dst *T, fn func(*T) error) error {
	if dst == nil {
		return errorsf("destination must not be nil")
	}

	bound := cloneBindingTarget(dst)
	if err := fn(bound); err != nil {
		return err
	}

	*dst = *bound
	return nil
}
