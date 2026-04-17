package reqx

import (
	"net/http"
	"strings"

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
func (p *PathParam) Int() *IntParam {
	return newIntParam(p.spec)
}

// Int64 读取 int64 参数。
func (p *PathParam) Int64() *Int64Param {
	return newInt64Param(p.spec)
}

// Uint64 读取 uint64 参数。
func (p *PathParam) Uint64() *Uint64Param {
	return newUint64Param(p.spec)
}

// Uint 读取 uint 参数。
func (p *PathParam) Uint() *UintParam {
	return newUintParam(p.spec)
}

// UUID 读取 uuid.UUID 参数。
func (p *PathParam) UUID() *UUIDParam {
	return newUUIDParam(p.spec)
}

func pathParamValues(r *http.Request, name string) ([]string, bool) {
	if r == nil {
		return nil, false
	}
	value := r.PathValue(name)
	if value != "" {
		return []string{value}, true
	}
	if pathHasWildcard(r.Pattern, name) {
		return []string{value}, true
	}
	return nil, false
}

func pathHasWildcard(pattern, name string) bool {
	pattern = strings.TrimSpace(pattern)
	name = strings.TrimSpace(name)
	if pattern == "" || name == "" {
		return false
	}

	for i := 0; i < len(pattern); i++ {
		if pattern[i] != '{' {
			continue
		}

		end := strings.IndexByte(pattern[i+1:], '}')
		if end < 0 {
			break
		}

		token := strings.TrimSpace(pattern[i+1 : i+1+end])
		token = strings.TrimSuffix(token, "...")
		token = strings.TrimSpace(token)
		if token == "$" {
			token = ""
		}
		if token == name {
			return true
		}

		i += end + 1
	}

	return false
}
