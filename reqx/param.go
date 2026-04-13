package reqx

import (
	"errors"
	"net/http"
	"strings"
)

var errInvalidParamValue = errors.New("invalid param value")

type paramLookupFunc func(r *http.Request, name string) ([]string, bool)

// Param 表示一个待解析的 path/query 单参数。
type Param struct {
	r      *http.Request
	name   string
	input  string
	lookup paramLookupFunc
}

// Path 创建 path 单参数读取与校验 builder。
func Path(r *http.Request, name string) *Param {
	return &Param{
		r:      r,
		name:   strings.TrimSpace(name),
		input:  ViolationInPath,
		lookup: pathParamValues,
	}
}

// Query 创建 query 单参数读取与校验 builder。
func Query(r *http.Request, name string) *Param {
	return &Param{
		r:      r,
		name:   strings.TrimSpace(name),
		input:  ViolationInQuery,
		lookup: queryParamValues,
	}
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

func queryParamValues(r *http.Request, name string) ([]string, bool) {
	if r == nil || r.URL == nil {
		return nil, false
	}
	values, exists := r.URL.Query()[name]
	return values, exists
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

type paramSpec struct {
	r      *http.Request
	name   string
	input  string
	lookup paramLookupFunc
}

func (s paramSpec) values() ([]string, bool, error) {
	if s.lookup == nil || s.input == "" {
		return nil, false, errorsf("param builder must be created with Path or Query")
	}
	if s.r == nil {
		return nil, false, errorsf("request must not be nil")
	}
	if s.name == "" {
		return nil, false, errorsf("parameter name must not be empty")
	}

	values, exists := s.lookup(s.r, s.name)
	return values, exists, nil
}

type paramValue[T any] struct {
	spec         paramSpec
	parse        func(string) (T, error)
	required     bool
	hasDefault   bool
	defaultValue T
	checks       []func(T) error
	usageErr     error
}

func newParamValue[T any](p *Param, parse func(string) (T, error)) paramValue[T] {
	if p == nil {
		return paramValue[T]{usageErr: errorsf("param builder must not be nil")}
	}

	return paramValue[T]{
		spec: paramSpec{
			r:      p.r,
			name:   p.name,
			input:  p.input,
			lookup: p.lookup,
		},
		parse: parse,
	}
}

func (p *paramValue[T]) setUsageErr(err error) {
	if p.usageErr == nil {
		p.usageErr = err
	}
}

func (p *paramValue[T]) setRequired() {
	if p.hasDefault {
		p.setUsageErr(errorsf("required and default are mutually exclusive"))
		return
	}
	p.required = true
}

func (p *paramValue[T]) setDefault(value T) {
	if p.required {
		p.setUsageErr(errorsf("required and default are mutually exclusive"))
		return
	}
	p.hasDefault = true
	p.defaultValue = value
}

func (p *paramValue[T]) addCheck(check func(T) error) {
	if check == nil {
		p.setUsageErr(errorsf("check must not be nil"))
		return
	}
	p.checks = append(p.checks, check)
}

func (p *paramValue[T]) resolve() (T, error) {
	var zero T
	if p.usageErr != nil {
		return zero, p.usageErr
	}

	values, exists, err := p.spec.values()
	if err != nil {
		return zero, err
	}

	if !exists || len(values) == 0 {
		switch {
		case p.hasDefault:
			return p.runChecks(p.defaultValue)
		case p.required:
			return zero, InvalidRequest(newViolation(p.spec.name, p.spec.input, ViolationCodeRequired, ""))
		default:
			return zero, nil
		}
	}

	value, err := p.parse(values[0])
	if err != nil {
		return zero, InvalidRequest(newViolation(p.spec.name, p.spec.input, ViolationCodeInvalid, ""))
	}

	return p.runChecks(value)
}

func (p *paramValue[T]) runChecks(value T) (T, error) {
	for _, check := range p.checks {
		if err := check(value); err != nil {
			detail := ""
			if !errors.Is(err, errInvalidParamValue) {
				detail = strings.TrimSpace(err.Error())
			}
			return value, InvalidRequest(newViolation(p.spec.name, p.spec.input, ViolationCodeInvalid, detail))
		}
	}
	return value, nil
}
