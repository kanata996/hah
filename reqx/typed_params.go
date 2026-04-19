package reqx

import (
	"cmp"
	"errors"
	"regexp"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

func newStringParam(spec paramSpec) *StringParam {
	return &StringParam{value: newParamValue(spec, parseStringValue)}
}

func newValueParam[T any](spec paramSpec, parse func(string) (T, error)) *ValueParam[T] {
	return &ValueParam[T]{value: newParamValue(spec, parse)}
}

func newOrderedParam[T cmp.Ordered](spec paramSpec, parse func(string) (T, error)) *OrderedParam[T] {
	return &OrderedParam[T]{value: newParamValue(spec, parse)}
}

func newTimeParam(spec paramSpec, parse func(string) (time.Time, error)) *TimeParam {
	return &TimeParam{value: newParamValue(spec, parse)}
}

func newMultiParam[T any](spec paramSpec, parse func([]string) []T, clone func([]T) []T) *MultiParam[T] {
	return &MultiParam[T]{
		value: newMultiParamValue(spec, parse, clone),
	}
}

// ValueParam 读取并校验通用单值参数。
type ValueParam[T any] struct {
	value paramValue[T]
}

// OrderedParam 读取并校验可比较范围的单值参数。
type OrderedParam[T cmp.Ordered] struct {
	value paramValue[T]
	min   *T
	max   *T
}

// StringParam 读取并校验 string 参数。
type StringParam struct {
	value  paramValue[string]
	minLen *int
	maxLen *int
	oneOf  []string
	match  *regexp.Regexp
}

// TimeParam 读取并校验 time.Time 参数。
type TimeParam struct {
	value  paramValue[time.Time]
	after  *time.Time
	before *time.Time
}

// MultiParam 读取并校验多值参数。
type MultiParam[T any] struct {
	value paramValue[[]T]
}

func (p *ValueParam[T]) Required() *ValueParam[T] {
	p.value.setRequired()
	return p
}

func (p *ValueParam[T]) Default(value T) *ValueParam[T] {
	p.value.setDefault(value)
	return p
}

func (p *ValueParam[T]) Check(check func(T) error) *ValueParam[T] {
	p.value.addCheck(check)
	return p
}

func (p *ValueParam[T]) Get() (T, error) {
	return p.value.resolve(nil)
}

func (p *OrderedParam[T]) Required() *OrderedParam[T] {
	p.value.setRequired()
	return p
}

func (p *OrderedParam[T]) Default(value T) *OrderedParam[T] {
	p.value.setDefault(value)
	return p
}

func (p *OrderedParam[T]) Min(value T) *OrderedParam[T] {
	p.min = &value
	return p
}

func (p *OrderedParam[T]) Max(value T) *OrderedParam[T] {
	p.max = &value
	return p
}

func (p *OrderedParam[T]) Check(check func(T) error) *OrderedParam[T] {
	p.value.addCheck(check)
	return p
}

func (p *OrderedParam[T]) Get() (T, error) {
	var zero T
	if p.value.usageErr != nil {
		return zero, p.value.usageErr
	}
	if err := p.constraintUsageErr(); err != nil {
		return zero, err
	}
	return p.value.resolve(p.validateBuiltins)
}

func (p *OrderedParam[T]) validateBuiltins(value T) error {
	if p.min != nil && value < *p.min {
		return errInvalidParamValue
	}
	if p.max != nil && value > *p.max {
		return errInvalidParamValue
	}
	return nil
}

func (p *OrderedParam[T]) constraintUsageErr() error {
	if p.min == nil || p.max == nil || *p.min <= *p.max {
		return nil
	}
	return usageErrorf("minimum must be less than or equal to maximum")
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
		p.value.setUsageErr(usageErrorf("minimum length must be >= 0"))
		return p
	}
	p.minLen = &n
	return p
}

func (p *StringParam) MaxLen(n int) *StringParam {
	if n < 0 {
		p.value.setUsageErr(usageErrorf("maximum length must be >= 0"))
		return p
	}
	p.maxLen = &n
	return p
}

func (p *StringParam) OneOf(values ...string) *StringParam {
	if len(values) == 0 {
		p.value.setUsageErr(usageErrorf("one-of values must not be empty"))
		return p
	}
	p.oneOf = append([]string(nil), values...)
	return p
}

func (p *StringParam) Match(pattern *regexp.Regexp) *StringParam {
	if pattern == nil {
		p.value.setUsageErr(usageErrorf("match pattern must not be nil"))
		return p
	}
	p.match = pattern
	return p
}

func (p *StringParam) Check(check func(string) error) *StringParam {
	p.value.addCheck(check)
	return p
}

func (p *StringParam) Get() (string, error) {
	if p.value.usageErr != nil {
		return "", p.value.usageErr
	}
	if err := p.constraintUsageErr(); err != nil {
		return "", err
	}
	return p.value.resolve(p.validateBuiltins)
}

func (p *StringParam) validateBuiltins(value string) error {
	if p.minLen != nil || p.maxLen != nil {
		length := utf8.RuneCountInString(value)
		if p.minLen != nil && length < *p.minLen {
			return errInvalidParamValue
		}
		if p.maxLen != nil && length > *p.maxLen {
			return errInvalidParamValue
		}
	}
	if p.oneOf != nil {
		if !containsString(p.oneOf, value) {
			return errInvalidParamValue
		}
	}
	if p.match != nil && !p.match.MatchString(value) {
		return errInvalidParamValue
	}
	return nil
}

func (p *StringParam) constraintUsageErr() error {
	if p.minLen == nil || p.maxLen == nil || *p.minLen <= *p.maxLen {
		return nil
	}
	return usageErrorf("minimum length must be less than or equal to maximum length")
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
	p.after = &value
	return p
}

func (p *TimeParam) Before(value time.Time) *TimeParam {
	p.before = &value
	return p
}

func (p *TimeParam) Check(check func(time.Time) error) *TimeParam {
	p.value.addCheck(check)
	return p
}

func (p *TimeParam) Get() (time.Time, error) {
	if p.value.usageErr != nil {
		return time.Time{}, p.value.usageErr
	}
	if err := p.constraintUsageErr(); err != nil {
		return time.Time{}, err
	}
	return p.value.resolve(p.validateBuiltins)
}

func (p *TimeParam) validateBuiltins(value time.Time) error {
	if p.after != nil && !value.After(*p.after) {
		return errInvalidParamValue
	}
	if p.before != nil && !value.Before(*p.before) {
		return errInvalidParamValue
	}
	return nil
}

func (p *TimeParam) constraintUsageErr() error {
	if p.after == nil || p.before == nil || p.after.Before(*p.before) {
		return nil
	}
	return usageErrorf("after time must be earlier than before time")
}

func (p *MultiParam[T]) Required() *MultiParam[T] {
	p.value.setRequired()
	return p
}

func (p *MultiParam[T]) Default(value []T) *MultiParam[T] {
	p.value.setDefault(value)
	return p
}

func (p *MultiParam[T]) Check(check func([]T) error) *MultiParam[T] {
	p.value.addCheck(check)
	return p
}

func (p *MultiParam[T]) Get() ([]T, error) {
	return p.value.resolve(nil)
}

func parseStringValue(value string) (string, error) {
	return value, nil
}

func parseIntBits(value string, bits int) (int64, error) {
	return strconv.ParseInt(value, 10, bits)
}

func parseUintBits(value string, bits int) (uint64, error) {
	return strconv.ParseUint(value, 10, bits)
}

func parseFloatBits(value string, bits int) (float64, error) {
	return strconv.ParseFloat(value, bits)
}

func parseIntValue(value string) (int, error) {
	parsed, err := parseIntBits(value, 0)
	return int(parsed), err
}

func parseInt64Value(value string) (int64, error) {
	return parseIntBits(value, 64)
}

func parseUintValue(value string) (uint, error) {
	parsed, err := parseUintBits(value, 0)
	return uint(parsed), err
}

func parseUint64Value(value string) (uint64, error) {
	return parseUintBits(value, 64)
}

func parseBoolValue(value string) (bool, error) {
	return strconv.ParseBool(value)
}

func parseFloat64Value(value string) (float64, error) {
	return parseFloatBits(value, 64)
}

func parseDurationValue(value string) (time.Duration, error) {
	return time.ParseDuration(value)
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

func parseFixedWidthTimestamp(value string, digits int) (int64, error) {
	if len(value) != digits {
		return 0, errors.New("timestamp has invalid width")
	}
	return strconv.ParseInt(value, 10, 64)
}

func cloneSlice[T any](values []T) []T {
	if values == nil {
		return nil
	}
	return append([]T(nil), values...)
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
