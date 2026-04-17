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
	return &OrderedParam[T]{value: newOrderedRangeParam(spec, parse)}
}

func newTimeParam(spec paramSpec, parse func(string) (time.Time, error)) *TimeParam {
	return &TimeParam{value: newTimeRangeParam(spec, parse)}
}

func newMultiParam[T any](spec paramSpec, parse func([]string) []T) *MultiParam[T] {
	return &MultiParam[T]{
		value: newMultiParamValue(spec, parse, cloneSlice[T]),
	}
}

type requiredSetter interface {
	setRequired()
}

type defaultSetter[T any] interface {
	setDefault(T)
}

type resolver[T any] interface {
	resolve() (T, error)
}

func requireParam[P any](p *P, value requiredSetter) *P {
	value.setRequired()
	return p
}

func defaultParam[P any, T any](p *P, value defaultSetter[T], defaultValue T) *P {
	value.setDefault(defaultValue)
	return p
}

func checkParam[P any, T any](p *P, addCheck func(func(T) error), check func(T) error) *P {
	addCheck(check)
	return p
}

func getParam[T any](value resolver[T]) (T, error) {
	return value.resolve()
}

type rangeConstraint int

const (
	rangeConstraintNone rangeConstraint = iota
	rangeConstraintMin
	rangeConstraintMax
	rangeConstraintAfter
	rangeConstraintBefore
	rangeConstraintMinLen
	rangeConstraintMaxLen
)

type orderedRangeParam[T cmp.Ordered] struct {
	value          paramValue[T]
	min            *T
	max            *T
	lastConstraint rangeConstraint
}

func newOrderedRangeParam[T cmp.Ordered](spec paramSpec, parse func(string) (T, error)) orderedRangeParam[T] {
	return orderedRangeParam[T]{value: newParamValue(spec, parse)}
}

func (p *orderedRangeParam[T]) setRequired() {
	p.value.setRequired()
}

func (p *orderedRangeParam[T]) setDefault(value T) {
	p.value.setDefault(value)
}

func (p *orderedRangeParam[T]) addCheck(check func(T) error) {
	p.value.addCheck(check)
}

func (p *orderedRangeParam[T]) resolve() (T, error) {
	var zero T
	if p.value.state.usageErr != nil {
		return zero, p.value.state.usageErr
	}
	if err := p.constraintUsageErr(); err != nil {
		return zero, err
	}
	return p.value.resolve()
}

func (p *orderedRangeParam[T]) setMin(value T) {
	p.min = &value
	p.lastConstraint = rangeConstraintMin
	p.value.state.setNamedCheck("min", func(v T) error {
		if v < value {
			return errInvalidParamValue
		}
		return nil
	})
}

func (p *orderedRangeParam[T]) setMax(value T) {
	p.max = &value
	p.lastConstraint = rangeConstraintMax
	p.value.state.setNamedCheck("max", func(v T) error {
		if v > value {
			return errInvalidParamValue
		}
		return nil
	})
}

func (p *orderedRangeParam[T]) constraintUsageErr() error {
	if p.min == nil || p.max == nil || *p.min <= *p.max {
		return nil
	}

	if p.lastConstraint == rangeConstraintMin {
		return usageErrorf("minimum must be less than or equal to maximum")
	}
	return usageErrorf("maximum must be greater than or equal to minimum")
}

type timeRangeParam struct {
	value          paramValue[time.Time]
	after          *time.Time
	before         *time.Time
	lastConstraint rangeConstraint
}

func newTimeRangeParam(spec paramSpec, parse func(string) (time.Time, error)) timeRangeParam {
	return timeRangeParam{value: newParamValue(spec, parse)}
}

func (p *timeRangeParam) setRequired() {
	p.value.setRequired()
}

func (p *timeRangeParam) setDefault(value time.Time) {
	p.value.setDefault(value)
}

func (p *timeRangeParam) addCheck(check func(time.Time) error) {
	p.value.addCheck(check)
}

func (p *timeRangeParam) resolve() (time.Time, error) {
	if p.value.state.usageErr != nil {
		return time.Time{}, p.value.state.usageErr
	}
	if err := p.constraintUsageErr(); err != nil {
		return time.Time{}, err
	}
	return p.value.resolve()
}

func (p *timeRangeParam) setAfter(value time.Time) {
	p.after = &value
	p.lastConstraint = rangeConstraintAfter
	p.value.state.setNamedCheck("after", func(v time.Time) error {
		if !v.After(value) {
			return errInvalidParamValue
		}
		return nil
	})
}

func (p *timeRangeParam) setBefore(value time.Time) {
	p.before = &value
	p.lastConstraint = rangeConstraintBefore
	p.value.state.setNamedCheck("before", func(v time.Time) error {
		if !v.Before(value) {
			return errInvalidParamValue
		}
		return nil
	})
}

func (p *timeRangeParam) constraintUsageErr() error {
	if p.after == nil || p.before == nil || p.after.Before(*p.before) {
		return nil
	}

	if p.lastConstraint == rangeConstraintAfter {
		return usageErrorf("after time must be earlier than before time")
	}
	return usageErrorf("before time must be later than after time")
}

// ValueParam 读取并校验通用单值参数。
type ValueParam[T any] struct {
	value paramValue[T]
}

// OrderedParam 读取并校验可比较范围的单值参数。
type OrderedParam[T cmp.Ordered] struct {
	value orderedRangeParam[T]
}

// StringParam 读取并校验 string 参数。
type StringParam struct {
	value          paramValue[string]
	minLen         *int
	maxLen         *int
	lastConstraint rangeConstraint
}

// TimeParam 读取并校验 time.Time 参数。
type TimeParam struct {
	value timeRangeParam
}

// MultiParam 读取并校验多值参数。
type MultiParam[T any] struct {
	value multiParamValue[[]T]
}

func (p *ValueParam[T]) Required() *ValueParam[T] {
	return requireParam(p, &p.value)
}

func (p *ValueParam[T]) Default(value T) *ValueParam[T] {
	return defaultParam(p, &p.value, value)
}

func (p *ValueParam[T]) Check(check func(T) error) *ValueParam[T] {
	return checkParam(p, p.value.addCheck, check)
}

func (p *ValueParam[T]) Get() (T, error) {
	return getParam(&p.value)
}

func (p *OrderedParam[T]) Required() *OrderedParam[T] {
	return requireParam(p, &p.value)
}

func (p *OrderedParam[T]) Default(value T) *OrderedParam[T] {
	return defaultParam(p, &p.value, value)
}

func (p *OrderedParam[T]) Min(value T) *OrderedParam[T] {
	p.value.setMin(value)
	return p
}

func (p *OrderedParam[T]) Max(value T) *OrderedParam[T] {
	p.value.setMax(value)
	return p
}

func (p *OrderedParam[T]) Check(check func(T) error) *OrderedParam[T] {
	return checkParam(p, p.value.addCheck, check)
}

func (p *OrderedParam[T]) Get() (T, error) {
	return getParam(&p.value)
}

func (p *StringParam) Required() *StringParam {
	return requireParam(p, &p.value)
}

func (p *StringParam) Default(value string) *StringParam {
	return defaultParam(p, &p.value, value)
}

func (p *StringParam) MinLen(n int) *StringParam {
	if n < 0 {
		p.value.setUsageErr(usageErrorf("minimum length must be >= 0"))
		return p
	}
	p.minLen = &n
	p.lastConstraint = rangeConstraintMinLen
	p.value.state.setNamedCheck("min_length", func(value string) error {
		if utf8.RuneCountInString(value) < n {
			return errInvalidParamValue
		}
		return nil
	})
	return p
}

func (p *StringParam) MaxLen(n int) *StringParam {
	if n < 0 {
		p.value.setUsageErr(usageErrorf("maximum length must be >= 0"))
		return p
	}
	p.maxLen = &n
	p.lastConstraint = rangeConstraintMaxLen
	p.value.state.setNamedCheck("max_length", func(value string) error {
		if utf8.RuneCountInString(value) > n {
			return errInvalidParamValue
		}
		return nil
	})
	return p
}

func (p *StringParam) OneOf(values ...string) *StringParam {
	if len(values) == 0 {
		p.value.setUsageErr(usageErrorf("one-of values must not be empty"))
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
		p.value.setUsageErr(usageErrorf("match pattern must not be nil"))
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
	return checkParam(p, p.value.addCheck, check)
}

func (p *StringParam) Get() (string, error) {
	if p.value.state.usageErr != nil {
		return "", p.value.state.usageErr
	}
	if err := p.constraintUsageErr(); err != nil {
		return "", err
	}
	return getParam(&p.value)
}

func (p *StringParam) constraintUsageErr() error {
	if p.minLen == nil || p.maxLen == nil || *p.minLen <= *p.maxLen {
		return nil
	}

	if p.lastConstraint == rangeConstraintMinLen {
		return usageErrorf("minimum length must be less than or equal to maximum length")
	}
	return usageErrorf("maximum length must be greater than or equal to minimum length")
}

func (p *TimeParam) Required() *TimeParam {
	return requireParam(p, &p.value)
}

func (p *TimeParam) Default(value time.Time) *TimeParam {
	return defaultParam(p, &p.value, value)
}

func (p *TimeParam) After(value time.Time) *TimeParam {
	p.value.setAfter(value)
	return p
}

func (p *TimeParam) Before(value time.Time) *TimeParam {
	p.value.setBefore(value)
	return p
}

func (p *TimeParam) Check(check func(time.Time) error) *TimeParam {
	return checkParam(p, p.value.addCheck, check)
}

func (p *TimeParam) Get() (time.Time, error) {
	return getParam(&p.value)
}

func (p *MultiParam[T]) Required() *MultiParam[T] {
	return requireParam(p, &p.value)
}

func (p *MultiParam[T]) Default(value []T) *MultiParam[T] {
	return defaultParam(p, &p.value, value)
}

func (p *MultiParam[T]) Check(check func([]T) error) *MultiParam[T] {
	return checkParam(p, p.value.addCheck, check)
}

func (p *MultiParam[T]) Get() ([]T, error) {
	return getParam(&p.value)
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

func cloneSlice[T any](values []T) []T {
	if values == nil {
		return nil
	}
	return append([]T(nil), values...)
}
