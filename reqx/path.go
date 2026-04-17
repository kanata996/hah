package reqx

import (
	"net/http"
	"strings"
	"unicode"

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

	path, ok := serveMuxPatternPath(pattern)
	if !ok {
		return false
	}

	seen := make(map[string]struct{})
	segments := strings.Split(path, "/")
	for i, segment := range segments {
		if segment == "" || segment == "{$}" {
			continue
		}
		if !strings.ContainsAny(segment, "{}") {
			continue
		}

		wildcard, catchAll, ok := parseServeMuxWildcard(segment)
		if !ok {
			return false
		}
		if catchAll && i != len(segments)-1 {
			return false
		}
		if _, exists := seen[wildcard]; exists {
			return false
		}
		seen[wildcard] = struct{}{}
		if wildcard == name {
			return true
		}
	}

	return false
}

func serveMuxPatternPath(pattern string) (string, bool) {
	slash := strings.IndexByte(pattern, '/')
	if slash < 0 {
		return "", false
	}
	if strings.ContainsAny(pattern[:slash], "{}") {
		return "", false
	}
	return pattern[slash:], true
}

func parseServeMuxWildcard(segment string) (name string, catchAll bool, ok bool) {
	if len(segment) < 3 || segment[0] != '{' || segment[len(segment)-1] != '}' {
		return "", false, false
	}

	token := segment[1 : len(segment)-1]
	if token == "" || token == "$" {
		return "", false, false
	}
	if strings.HasSuffix(token, "...") {
		catchAll = true
		token = strings.TrimSuffix(token, "...")
	}
	if !isValidServeMuxWildcardName(token) {
		return "", false, false
	}

	return token, catchAll, true
}

func isValidServeMuxWildcardName(name string) bool {
	for i, r := range name {
		if i == 0 {
			if r != '_' && !unicode.IsLetter(r) {
				return false
			}
			continue
		}
		if r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}
