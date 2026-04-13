package reqx

import (
	"net/http"
	"strings"
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
	if value := r.PathValue(name); value != "" {
		return []string{value}, true
	}
	for _, wildcard := range pathWildcardNames(r.Pattern) {
		if wildcard == name {
			return []string{r.PathValue(name)}, true
		}
	}
	return nil, false
}

func pathWildcardNames(pattern string) []string {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return nil
	}

	names := make([]string, 0, 2)
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
		token, _, _ = strings.Cut(token, ":")
		token = strings.TrimSpace(token)
		if token != "" && token != "$" {
			names = append(names, token)
		}

		i += end + 1
	}

	return names
}
