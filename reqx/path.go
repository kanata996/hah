package reqx

import (
	"net/http"
	"strings"

	ireq "github.com/kanata996/hah/internal/req"
)

// PathParam 表示一个待解析的 path 单参数。
type PathParam struct {
	spec paramSpec
}

// Path 创建 path 单参数读取与校验 builder。
func Path(r *http.Request, name string) *PathParam {
	return &PathParam{
		spec: paramSpec{
			r:      r,
			name:   strings.TrimSpace(name),
			input:  ViolationInPath,
			lookup: pathParamValues,
		},
	}
}

func (p *PathParam) specOrZero() paramSpec {
	if p == nil {
		return paramSpec{}
	}
	return p.spec
}

// String 读取 string 参数。
func (p *PathParam) String() *StringParam {
	return newStringParam(p.specOrZero(), p == nil)
}

// Int 读取 int 参数。
func (p *PathParam) Int() *IntParam {
	return newIntParam(p.specOrZero(), p == nil)
}

// Int64 读取 int64 参数。
func (p *PathParam) Int64() *Int64Param {
	return newInt64Param(p.specOrZero(), p == nil)
}

// Uint64 读取 uint64 参数。
func (p *PathParam) Uint64() *Uint64Param {
	return newUint64Param(p.specOrZero(), p == nil)
}

// Uint 读取 uint 参数。
func (p *PathParam) Uint() *UintParam {
	return newUintParam(p.specOrZero(), p == nil)
}

// UUID 读取 uuid.UUID 参数。
func (p *PathParam) UUID() *UUIDParam {
	return newUUIDParam(p.specOrZero(), p == nil)
}

func pathParamValues(r *http.Request, name string) ([]string, bool) {
	if r == nil {
		return nil, false
	}
	value := r.PathValue(name)
	if value != "" {
		return []string{value}, true
	}
	if ireq.PathHasWildcard(r.Pattern, name) {
		return []string{value}, true
	}
	return nil, false
}
