package reqx

import (
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
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

// String 读取 string 参数。
func (p *Param) String() *StringParam {
	return &StringParam{value: newParamValue(p, parseStringValue)}
}

// Int 读取 int 参数。
func (p *Param) Int() *IntParam {
	return &IntParam{value: newParamValue(p, parseIntValue)}
}

// Int64 读取 int64 参数。
func (p *Param) Int64() *Int64Param {
	return &Int64Param{value: newParamValue(p, parseInt64Value)}
}

// Uint 读取 uint 参数。
func (p *Param) Uint() *UintParam {
	return &UintParam{value: newParamValue(p, parseUintValue)}
}

// Bool 读取 bool 参数。
func (p *Param) Bool() *BoolParam {
	return &BoolParam{value: newParamValue(p, parseBoolValue)}
}

// Float64 读取 float64 参数。
func (p *Param) Float64() *Float64Param {
	return &Float64Param{value: newParamValue(p, parseFloat64Value)}
}

// UUID 读取 uuid.UUID 参数。
func (p *Param) UUID() *UUIDParam {
	return &UUIDParam{value: newParamValue(p, parseUUIDValue)}
}

// Time 按 RFC3339 读取 time.Time 参数。
func (p *Param) Time() *TimeParam {
	return &TimeParam{value: newParamValue(p, parseRFC3339Time)}
}

// UnixTime 按 10 位秒级 Unix 时间戳读取 time.Time 参数。
func (p *Param) UnixTime() *TimeParam {
	return &TimeParam{value: newParamValue(p, parseUnixTime)}
}

// UnixMilliTime 按 13 位毫秒级 Unix 时间戳读取 time.Time 参数。
func (p *Param) UnixMilliTime() *TimeParam {
	return &TimeParam{value: newParamValue(p, parseUnixMilliTime)}
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

// StringParam 读取并校验 string 参数。
type StringParam struct {
	value paramValue[string]
}

func (p *StringParam) Required() *StringParam {
	p.value.setRequired()
	return p
}

func (p *StringParam) Default(value string) *StringParam {
	p.value.setDefault(value)
	return p
}

func (p *StringParam) MinLen(n int) *StringParam {
	if n < 0 {
		p.value.setUsageErr(errorsf("minimum length must be >= 0"))
		return p
	}
	p.value.addCheck(func(value string) error {
		if utf8.RuneCountInString(value) < n {
			return errInvalidParamValue
		}
		return nil
	})
	return p
}

func (p *StringParam) MaxLen(n int) *StringParam {
	if n < 0 {
		p.value.setUsageErr(errorsf("maximum length must be >= 0"))
		return p
	}
	p.value.addCheck(func(value string) error {
		if utf8.RuneCountInString(value) > n {
			return errInvalidParamValue
		}
		return nil
	})
	return p
}

func (p *StringParam) OneOf(values ...string) *StringParam {
	if len(values) == 0 {
		p.value.setUsageErr(errorsf("one-of values must not be empty"))
		return p
	}
	allowed := make(map[string]struct{}, len(values))
	for _, value := range values {
		allowed[value] = struct{}{}
	}
	p.value.addCheck(func(value string) error {
		if _, ok := allowed[value]; !ok {
			return errInvalidParamValue
		}
		return nil
	})
	return p
}

func (p *StringParam) Match(pattern *regexp.Regexp) *StringParam {
	if pattern == nil {
		p.value.setUsageErr(errorsf("match pattern must not be nil"))
		return p
	}
	p.value.addCheck(func(value string) error {
		if !pattern.MatchString(value) {
			return errInvalidParamValue
		}
		return nil
	})
	return p
}

func (p *StringParam) Check(check func(string) error) *StringParam {
	p.value.addCheck(check)
	return p
}

func (p *StringParam) Get() (string, error) {
	return p.value.resolve()
}

// IntParam 读取并校验 int 参数。
type IntParam struct {
	value paramValue[int]
	min   *int
	max   *int
}

func (p *IntParam) Required() *IntParam {
	p.value.setRequired()
	return p
}

func (p *IntParam) Default(value int) *IntParam {
	p.value.setDefault(value)
	return p
}

func (p *IntParam) Min(value int) *IntParam {
	if p.max != nil && value > *p.max {
		p.value.setUsageErr(errorsf("minimum must be less than or equal to maximum"))
		return p
	}
	p.min = &value
	p.value.addCheck(func(v int) error {
		if v < value {
			return errInvalidParamValue
		}
		return nil
	})
	return p
}

func (p *IntParam) Max(value int) *IntParam {
	if p.min != nil && *p.min > value {
		p.value.setUsageErr(errorsf("maximum must be greater than or equal to minimum"))
		return p
	}
	p.max = &value
	p.value.addCheck(func(v int) error {
		if v > value {
			return errInvalidParamValue
		}
		return nil
	})
	return p
}

func (p *IntParam) Check(check func(int) error) *IntParam {
	p.value.addCheck(check)
	return p
}

func (p *IntParam) Get() (int, error) {
	return p.value.resolve()
}

// Int64Param 读取并校验 int64 参数。
type Int64Param struct {
	value paramValue[int64]
	min   *int64
	max   *int64
}

func (p *Int64Param) Required() *Int64Param {
	p.value.setRequired()
	return p
}

func (p *Int64Param) Default(value int64) *Int64Param {
	p.value.setDefault(value)
	return p
}

func (p *Int64Param) Min(value int64) *Int64Param {
	if p.max != nil && value > *p.max {
		p.value.setUsageErr(errorsf("minimum must be less than or equal to maximum"))
		return p
	}
	p.min = &value
	p.value.addCheck(func(v int64) error {
		if v < value {
			return errInvalidParamValue
		}
		return nil
	})
	return p
}

func (p *Int64Param) Max(value int64) *Int64Param {
	if p.min != nil && *p.min > value {
		p.value.setUsageErr(errorsf("maximum must be greater than or equal to minimum"))
		return p
	}
	p.max = &value
	p.value.addCheck(func(v int64) error {
		if v > value {
			return errInvalidParamValue
		}
		return nil
	})
	return p
}

func (p *Int64Param) Check(check func(int64) error) *Int64Param {
	p.value.addCheck(check)
	return p
}

func (p *Int64Param) Get() (int64, error) {
	return p.value.resolve()
}

// UintParam 读取并校验 uint 参数。
type UintParam struct {
	value paramValue[uint]
	min   *uint
	max   *uint
}

func (p *UintParam) Required() *UintParam {
	p.value.setRequired()
	return p
}

func (p *UintParam) Default(value uint) *UintParam {
	p.value.setDefault(value)
	return p
}

func (p *UintParam) Min(value uint) *UintParam {
	if p.max != nil && value > *p.max {
		p.value.setUsageErr(errorsf("minimum must be less than or equal to maximum"))
		return p
	}
	p.min = &value
	p.value.addCheck(func(v uint) error {
		if v < value {
			return errInvalidParamValue
		}
		return nil
	})
	return p
}

func (p *UintParam) Max(value uint) *UintParam {
	if p.min != nil && *p.min > value {
		p.value.setUsageErr(errorsf("maximum must be greater than or equal to minimum"))
		return p
	}
	p.max = &value
	p.value.addCheck(func(v uint) error {
		if v > value {
			return errInvalidParamValue
		}
		return nil
	})
	return p
}

func (p *UintParam) Check(check func(uint) error) *UintParam {
	p.value.addCheck(check)
	return p
}

func (p *UintParam) Get() (uint, error) {
	return p.value.resolve()
}

// BoolParam 读取并校验 bool 参数。
type BoolParam struct {
	value paramValue[bool]
}

func (p *BoolParam) Required() *BoolParam {
	p.value.setRequired()
	return p
}

func (p *BoolParam) Default(value bool) *BoolParam {
	p.value.setDefault(value)
	return p
}

func (p *BoolParam) Check(check func(bool) error) *BoolParam {
	p.value.addCheck(check)
	return p
}

func (p *BoolParam) Get() (bool, error) {
	return p.value.resolve()
}

// Float64Param 读取并校验 float64 参数。
type Float64Param struct {
	value paramValue[float64]
	min   *float64
	max   *float64
}

func (p *Float64Param) Required() *Float64Param {
	p.value.setRequired()
	return p
}

func (p *Float64Param) Default(value float64) *Float64Param {
	p.value.setDefault(value)
	return p
}

func (p *Float64Param) Min(value float64) *Float64Param {
	if p.max != nil && value > *p.max {
		p.value.setUsageErr(errorsf("minimum must be less than or equal to maximum"))
		return p
	}
	p.min = &value
	p.value.addCheck(func(v float64) error {
		if v < value {
			return errInvalidParamValue
		}
		return nil
	})
	return p
}

func (p *Float64Param) Max(value float64) *Float64Param {
	if p.min != nil && *p.min > value {
		p.value.setUsageErr(errorsf("maximum must be greater than or equal to minimum"))
		return p
	}
	p.max = &value
	p.value.addCheck(func(v float64) error {
		if v > value {
			return errInvalidParamValue
		}
		return nil
	})
	return p
}

func (p *Float64Param) Check(check func(float64) error) *Float64Param {
	p.value.addCheck(check)
	return p
}

func (p *Float64Param) Get() (float64, error) {
	return p.value.resolve()
}

// UUIDParam 读取并校验 uuid.UUID 参数。
type UUIDParam struct {
	value paramValue[uuid.UUID]
}

func (p *UUIDParam) Required() *UUIDParam {
	p.value.setRequired()
	return p
}

func (p *UUIDParam) Default(value uuid.UUID) *UUIDParam {
	p.value.setDefault(value)
	return p
}

func (p *UUIDParam) Check(check func(uuid.UUID) error) *UUIDParam {
	p.value.addCheck(check)
	return p
}

func (p *UUIDParam) Get() (uuid.UUID, error) {
	return p.value.resolve()
}

// TimeParam 读取并校验 time.Time 参数。
type TimeParam struct {
	value  paramValue[time.Time]
	after  *time.Time
	before *time.Time
}

func (p *TimeParam) Required() *TimeParam {
	p.value.setRequired()
	return p
}

func (p *TimeParam) Default(value time.Time) *TimeParam {
	p.value.setDefault(value)
	return p
}

func (p *TimeParam) After(value time.Time) *TimeParam {
	if p.before != nil && value.After(*p.before) {
		p.value.setUsageErr(errorsf("after time must be less than or equal to before time"))
		return p
	}
	p.after = &value
	p.value.addCheck(func(v time.Time) error {
		if v.Before(value) {
			return errInvalidParamValue
		}
		return nil
	})
	return p
}

func (p *TimeParam) Before(value time.Time) *TimeParam {
	if p.after != nil && p.after.After(value) {
		p.value.setUsageErr(errorsf("before time must be greater than or equal to after time"))
		return p
	}
	p.before = &value
	p.value.addCheck(func(v time.Time) error {
		if v.After(value) {
			return errInvalidParamValue
		}
		return nil
	})
	return p
}

func (p *TimeParam) Check(check func(time.Time) error) *TimeParam {
	p.value.addCheck(check)
	return p
}

func (p *TimeParam) Get() (time.Time, error) {
	return p.value.resolve()
}

func parseStringValue(value string) (string, error) {
	return value, nil
}

func parseIntValue(value string) (int, error) {
	if value == "" {
		value = "0"
	}
	parsed, err := strconv.ParseInt(value, 10, 0)
	return int(parsed), err
}

func parseInt64Value(value string) (int64, error) {
	if value == "" {
		value = "0"
	}
	return strconv.ParseInt(value, 10, 64)
}

func parseUintValue(value string) (uint, error) {
	if value == "" {
		value = "0"
	}
	parsed, err := strconv.ParseUint(value, 10, 0)
	return uint(parsed), err
}

func parseBoolValue(value string) (bool, error) {
	if value == "" {
		value = "false"
	}
	return strconv.ParseBool(value)
}

func parseFloat64Value(value string) (float64, error) {
	if value == "" {
		value = "0.0"
	}
	return strconv.ParseFloat(value, 64)
}

func parseUUIDValue(value string) (uuid.UUID, error) {
	return uuid.Parse(value)
}

func parseRFC3339Time(value string) (time.Time, error) {
	var parsed time.Time
	if err := parsed.UnmarshalText([]byte(value)); err != nil {
		return time.Time{}, err
	}
	return parsed, nil
}

func parseUnixTime(value string) (time.Time, error) {
	seconds, err := parseFixedWidthTimestamp(value, 10)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(seconds, 0).UTC(), nil
}

func parseUnixMilliTime(value string) (time.Time, error) {
	millis, err := parseFixedWidthTimestamp(value, 13)
	if err != nil {
		return time.Time{}, err
	}
	return time.UnixMilli(millis).UTC(), nil
}

func parseFixedWidthTimestamp(value string, digits int) (int64, error) {
	if len(value) != digits {
		return 0, errors.New("timestamp has invalid width")
	}
	return strconv.ParseInt(value, 10, 64)
}
