package reqx

import (
	"errors"
	"net/http"
	"strings"
)

var errInvalidParamValue = errors.New("invalid param value")

type paramLookupFunc func(r *http.Request, name string) ([]string, bool)

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

func newParamValue[T any](spec paramSpec, builderNil bool, parse func(string) (T, error)) paramValue[T] {
	if builderNil {
		return paramValue[T]{usageErr: errorsf("param builder must not be nil")}
	}

	return paramValue[T]{
		spec:  spec,
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

type multiParamValue[T any] struct {
	spec         paramSpec
	parse        func([]string) (T, error)
	clone        func(T) T
	required     bool
	hasDefault   bool
	defaultValue T
	checks       []func(T) error
	usageErr     error
}

func newMultiParamValue[T any](spec paramSpec, builderNil bool, parse func([]string) (T, error), clone func(T) T) multiParamValue[T] {
	if builderNil {
		return multiParamValue[T]{usageErr: errorsf("param builder must not be nil")}
	}

	return multiParamValue[T]{
		spec:  spec,
		parse: parse,
		clone: clone,
	}
}

func (p *multiParamValue[T]) cloneValue(value T) T {
	if p.clone == nil {
		return value
	}
	return p.clone(value)
}

func (p *multiParamValue[T]) setUsageErr(err error) {
	if p.usageErr == nil {
		p.usageErr = err
	}
}

func (p *multiParamValue[T]) setRequired() {
	if p.hasDefault {
		p.setUsageErr(errorsf("required and default are mutually exclusive"))
		return
	}
	p.required = true
}

func (p *multiParamValue[T]) setDefault(value T) {
	if p.required {
		p.setUsageErr(errorsf("required and default are mutually exclusive"))
		return
	}
	p.hasDefault = true
	p.defaultValue = p.cloneValue(value)
}

func (p *multiParamValue[T]) addCheck(check func(T) error) {
	if check == nil {
		p.setUsageErr(errorsf("check must not be nil"))
		return
	}
	p.checks = append(p.checks, check)
}

func (p *multiParamValue[T]) resolve() (T, error) {
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
			return p.runChecks(p.cloneValue(p.defaultValue))
		case p.required:
			return zero, InvalidRequest(newViolation(p.spec.name, p.spec.input, ViolationCodeRequired, ""))
		default:
			return zero, nil
		}
	}

	value, err := p.parse(values)
	if err != nil {
		return zero, InvalidRequest(newViolation(p.spec.name, p.spec.input, ViolationCodeInvalid, ""))
	}

	return p.runChecks(value)
}

func (p *multiParamValue[T]) runChecks(value T) (T, error) {
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

func cloneStringSlice(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string(nil), values...)
}
