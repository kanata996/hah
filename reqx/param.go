package reqx

import (
	"errors"
	"net/http"

	"github.com/kanata996/hah/internal/errx"
)

var errInvalidParamValue = errors.New("invalid param value")

type paramLookupFunc func(r *http.Request, name string) ([]string, bool)

type paramCheck[T any] struct {
	fn func(T) error
}

type paramSpec struct {
	r      *http.Request
	name   string
	input  errx.ViolationIn
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

type paramValue[T any] struct {
	spec          paramSpec
	parse         func([]string) (T, error)
	clone         func(T) T
	allowMultiple bool
	required      bool
	hasDefault    bool
	defaultValue  T
	checks        []paramCheck[T]
	usageErr      error
}

func newParamValue[T any](spec paramSpec, parse func(string) (T, error)) paramValue[T] {
	return paramValue[T]{
		spec: spec,
		parse: func(values []string) (T, error) {
			return parse(values[0])
		},
	}
}

func newMultiParamValue[T any](spec paramSpec, parse func([]string) T, clone func(T) T) paramValue[T] {
	return paramValue[T]{
		spec:          spec,
		clone:         clone,
		allowMultiple: true,
		parse: func(values []string) (T, error) {
			return parse(values), nil
		},
	}
}

func (p *paramValue[T]) cloneValue(value T) T {
	if p.clone == nil {
		return value
	}
	return p.clone(value)
}

func (p *paramValue[T]) setUsageErr(err error) {
	if p.usageErr == nil {
		p.usageErr = err
	}
}

func (p *paramValue[T]) setRequired() {
	if p.hasDefault {
		p.setUsageErr(usageErrorf("required and default are mutually exclusive"))
		return
	}
	p.required = true
}

func (p *paramValue[T]) setDefault(value T) {
	if p.required {
		p.setUsageErr(usageErrorf("required and default are mutually exclusive"))
		return
	}
	p.hasDefault = true
	p.defaultValue = p.cloneValue(value)
}

func (p *paramValue[T]) addCheck(check func(T) error) {
	if check == nil {
		p.setUsageErr(usageErrorf("check must not be nil"))
		return
	}
	p.checks = append(p.checks, paramCheck[T]{fn: check})
}

func (p *paramValue[T]) validateDefault(value T, validateBuiltins func(T) error) error {
	if validateBuiltins != nil {
		if err := validateBuiltins(value); err != nil {
			return usageErrorf("default value failed validation")
		}
	}
	for _, check := range p.checks {
		if err := check.fn(value); err != nil {
			return usageErrorf("default value failed validation")
		}
	}
	return nil
}

func (p *paramValue[T]) validateRequest(value T, validateBuiltins func(T) error) error {
	if validateBuiltins != nil {
		if err := validateBuiltins(value); err != nil {
			return InvalidRequest(newViolation(p.spec.name, p.spec.input, errx.CodeInvalid, ""))
		}
	}
	for _, check := range p.checks {
		if err := check.fn(value); err != nil {
			return InvalidRequest(newViolation(p.spec.name, p.spec.input, errx.CodeInvalid, ""))
		}
	}
	return nil
}

func (p *paramValue[T]) resolveMissing(validateBuiltins func(T) error) (T, error) {
	var zero T
	switch {
	case p.hasDefault:
		value := p.cloneValue(p.defaultValue)
		if err := p.validateDefault(value, validateBuiltins); err != nil {
			return zero, err
		}
		return value, nil
	case p.required:
		return zero, InvalidRequest(newViolation(p.spec.name, p.spec.input, errx.CodeRequired, ""))
	default:
		return zero, nil
	}
}

func (p *paramValue[T]) resolve(validateBuiltins func(T) error) (T, error) {
	var zero T
	if p.usageErr != nil {
		return zero, p.usageErr
	}

	values, exists, err := p.spec.values()
	if err != nil {
		return zero, err
	}
	if !exists || len(values) == 0 {
		return p.resolveMissing(validateBuiltins)
	}
	if !p.allowMultiple && len(values) > 1 {
		return zero, InvalidRequest(newViolation(p.spec.name, p.spec.input, errx.CodeMultiple, ""))
	}

	value, err := p.parse(values)
	if err != nil {
		return zero, InvalidRequest(newViolation(p.spec.name, p.spec.input, errx.CodeInvalid, ""))
	}
	if err := p.validateRequest(value, validateBuiltins); err != nil {
		return zero, err
	}
	return value, nil
}
