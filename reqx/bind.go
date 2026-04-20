package reqx

import (
	"net/http"
	"reflect"
)

// 本文件只放 query/body 入口共享的公共前置校验与通用错误格式。

// validateBindInputs 统一校验公开 BindQuery / BindBody 入口共享的前置条件。
func validateBindInputs(r *http.Request, target any) error {
	if r == nil {
		return usageErrorf("request must not be nil")
	}
	return validateBindingDestination(target)
}

// validateBindingDestination 统一校验绑定目标必须是非 nil 指针。
func validateBindingDestination(target any) error {
	if target == nil {
		return usageErrorf("destination must not be nil")
	}
	value := reflect.ValueOf(target)
	if value.Kind() != reflect.Pointer {
		return usageErrorf("destination must be a non-nil pointer")
	}
	if value.IsNil() {
		return usageErrorf("destination must be a non-nil pointer")
	}
	return nil
}
