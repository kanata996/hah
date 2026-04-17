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

	pattern, ok := pathPatternPart(pattern)
	if !ok {
		return false
	}

	matched := false
	seen := make(map[string]struct{})
	segments := strings.Split(pattern, "/")
	for i, segment := range segments {
		if segment == "" {
			continue
		}
		if !strings.ContainsAny(segment, "{}") {
			continue
		}

		wildcard, catchAll, ok := parsePathWildcard(segment)
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
			matched = true
		}
	}

	return matched
}

func pathPatternPart(pattern string) (string, bool) {
	if pattern == "" {
		return "", false
	}
	if pattern[0] == '/' {
		return pattern, !strings.ContainsAny(pattern, " \t")
	}

	split := strings.IndexAny(pattern, " \t")
	if split < 0 {
		return "", false
	}
	method := pattern[:split]
	if !isValidPathPatternMethod(method) {
		return "", false
	}

	pattern = strings.TrimLeft(pattern[split+1:], " \t")
	if pattern == "" || pattern[0] != '/' || strings.ContainsAny(pattern, " \t") {
		return "", false
	}
	return pattern, true
}

func parsePathWildcard(segment string) (name string, catchAll bool, ok bool) {
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
	if !isValidPathWildcardName(token) {
		return "", false, false
	}

	return token, catchAll, true
}

func isValidPathWildcardName(name string) bool {
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

func isValidPathPatternMethod(method string) bool {
	if method == "" {
		return false
	}
	for i := 0; i < len(method); i++ {
		if !isPathPatternMethodByte(method[i]) {
			return false
		}
	}
	return true
}

func isPathPatternMethodByte(b byte) bool {
	if '0' <= b && b <= '9' {
		return true
	}
	if 'A' <= b && b <= 'Z' {
		return true
	}
	if 'a' <= b && b <= 'z' {
		return true
	}

	switch b {
	case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~':
		return true
	default:
		return false
	}
}
