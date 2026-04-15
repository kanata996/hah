package reqx

import (
	"errors"
	"net/http"
	"strings"

	"github.com/kanata996/hah/errx"
)

var errInvalidParamValue = errors.New("invalid param value")

type paramLookupFunc func(r *http.Request, name string) ([]string, bool)

type paramCheck[T any] struct {
	name string
	fn   func(T) error
}

type paramSpec struct {
	r      *http.Request
	name   string
	input  string
	lookup paramLookupFunc
}

func (s paramSpec) values() ([]string, bool, error) {
	if s.lookup == nil || s.input == "" {
		return nil, false, usageErrorf("param builder must be created with Path or Query")
	}
	if s.r == nil {
		return nil, false, usageErrorf("request must not be nil")
	}
	if s.name == "" {
		return nil, false, usageErrorf("parameter name must not be empty")
	}

	values, exists := s.lookup(s.r, s.name)
	return values, exists, nil
}

type paramState[T any] struct {
	clone        func(T) T
	required     bool
	hasDefault   bool
	defaultValue T
	checks       []paramCheck[T]
	usageErr     error
}

func newParamState[T any](clone func(T) T) paramState[T] {
	return paramState[T]{clone: clone}
}

func (p *paramState[T]) cloneValue(value T) T {
	if p.clone == nil {
		return value
	}
	return p.clone(value)
}

func (p *paramState[T]) setUsageErr(err error) {
	if p.usageErr == nil {
		p.usageErr = err
	}
}

func (p *paramState[T]) setRequired() {
	if p.hasDefault {
		p.setUsageErr(usageErrorf("required and default are mutually exclusive"))
		return
	}
	p.required = true
}

func (p *paramState[T]) setDefault(value T) {
	if p.required {
		p.setUsageErr(usageErrorf("required and default are mutually exclusive"))
		return
	}
	p.hasDefault = true
	p.defaultValue = p.cloneValue(value)
}

func (p *paramState[T]) addCheck(check func(T) error) {
	if check == nil {
		p.setUsageErr(usageErrorf("check must not be nil"))
		return
	}
	p.checks = append(p.checks, paramCheck[T]{fn: check})
}

func (p *paramState[T]) setNamedCheck(name string, check func(T) error) {
	if check == nil {
		panic("reqx: named check must not be nil")
	}

	filtered := p.checks[:0]
	for _, existing := range p.checks {
		if existing.name == name {
			continue
		}
		filtered = append(filtered, existing)
	}
	p.checks = filtered

	p.checks = append(p.checks, paramCheck[T]{
		name: name,
		fn:   check,
	})
}

func (p *paramState[T]) resolveMissing(spec paramSpec) (T, error) {
	var zero T
	switch {
	case p.hasDefault:
		return p.runChecks(spec, p.cloneValue(p.defaultValue))
	case p.required:
		return zero, InvalidRequest(newViolation(spec.name, spec.input, errx.ViolationCodeRequired, ""))
	default:
		return zero, nil
	}
}

func (p *paramState[T]) runChecks(spec paramSpec, value T) (T, error) {
	for _, check := range p.checks {
		if err := check.fn(value); err != nil {
			detail := ""
			if !errors.Is(err, errInvalidParamValue) {
				detail = strings.TrimSpace(err.Error())
			}
			return value, InvalidRequest(newViolation(spec.name, spec.input, errx.ViolationCodeInvalid, detail))
		}
	}
	return value, nil
}

type paramValue[T any] struct {
	spec  paramSpec
	parse func(string) (T, error)
	state paramState[T]
}

func newParamValue[T any](spec paramSpec, parse func(string) (T, error)) paramValue[T] {
	return paramValue[T]{
		spec:  spec,
		parse: parse,
		state: newParamState[T](nil),
	}
}

func (p *paramValue[T]) setUsageErr(err error) {
	p.state.setUsageErr(err)
}

func (p *paramValue[T]) setRequired() {
	p.state.setRequired()
}

func (p *paramValue[T]) setDefault(value T) {
	p.state.setDefault(value)
}

func (p *paramValue[T]) addCheck(check func(T) error) {
	p.state.addCheck(check)
}

func (p *paramValue[T]) resolve() (T, error) {
	var zero T
	if p.state.usageErr != nil {
		return zero, p.state.usageErr
	}

	values, exists, err := p.spec.values()
	if err != nil {
		return zero, err
	}
	if !exists || len(values) == 0 {
		return p.state.resolveMissing(p.spec)
	}

	value, err := p.parse(values[0])
	if err != nil {
		return zero, InvalidRequest(newViolation(p.spec.name, p.spec.input, errx.ViolationCodeInvalid, ""))
	}

	return p.state.runChecks(p.spec, value)
}

type multiParamValue[T any] struct {
	spec  paramSpec
	parse func([]string) T
	state paramState[T]
}

func newMultiParamValue[T any](spec paramSpec, parse func([]string) T, clone func(T) T) multiParamValue[T] {
	return multiParamValue[T]{
		spec:  spec,
		parse: parse,
		state: newParamState(clone),
	}
}

func (p *multiParamValue[T]) setRequired() {
	p.state.setRequired()
}

func (p *multiParamValue[T]) setDefault(value T) {
	p.state.setDefault(value)
}

func (p *multiParamValue[T]) addCheck(check func(T) error) {
	p.state.addCheck(check)
}

func (p *multiParamValue[T]) resolve() (T, error) {
	var zero T
	if p.state.usageErr != nil {
		return zero, p.state.usageErr
	}

	values, exists, err := p.spec.values()
	if err != nil {
		return zero, err
	}
	if !exists || len(values) == 0 {
		return p.state.resolveMissing(p.spec)
	}

	value := p.parse(values)
	return p.state.runChecks(p.spec, value)
}

func cloneStringSlice(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string(nil), values...)
}
