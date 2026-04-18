package reqx

import (
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/kanata996/hah/errx"
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
			input:  errx.InPath,
			lookup: pathParamValues,
		},
	}
}

// String 读取 string 参数。
func (p *PathParam) String() *StringParam {
	return newStringParam(p.spec)
}

// Int 读取 int 参数。
func (p *PathParam) Int() *OrderedParam[int] {
	return newOrderedParam(p.spec, parseIntValue)
}

// Int64 读取 int64 参数。
func (p *PathParam) Int64() *OrderedParam[int64] {
	return newOrderedParam(p.spec, parseInt64Value)
}

// Uint64 读取 uint64 参数。
func (p *PathParam) Uint64() *OrderedParam[uint64] {
	return newOrderedParam(p.spec, parseUint64Value)
}

// Uint 读取 uint 参数。
func (p *PathParam) Uint() *OrderedParam[uint] {
	return newOrderedParam(p.spec, parseUintValue)
}

// UUID 读取 uuid.UUID 参数。
func (p *PathParam) UUID() *ValueParam[uuid.UUID] {
	return newValueParam(p.spec, parseUUIDValue)
}

func pathParamValues(r *http.Request, name string) ([]string, bool) {
	if r == nil {
		return nil, false
	}
	value := r.PathValue(name)
	if value == "" {
		return nil, false
	}
	return []string{value}, true
}
