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

func newIntParam(spec paramSpec) *IntParam {
	return &IntParam{value: newOrderedRangeParam(spec, parseIntValue)}
}

func newInt64Param(spec paramSpec) *Int64Param {
	return &Int64Param{value: newOrderedRangeParam(spec, parseInt64Value)}
}

func newUint64Param(spec paramSpec) *Uint64Param {
	return &Uint64Param{value: newOrderedRangeParam(spec, parseUint64Value)}
}

func newUintParam(spec paramSpec) *UintParam {
	return &UintParam{value: newOrderedRangeParam(spec, parseUintValue)}
}

func newBoolParam(spec paramSpec) *BoolParam {
	return &BoolParam{value: newParamValue(spec, parseBoolValue)}
}

func newFloat64Param(spec paramSpec) *Float64Param {
	return &Float64Param{value: newOrderedRangeParam(spec, parseFloat64Value)}
}

func newDurationParam(spec paramSpec) *DurationParam {
	return &DurationParam{value: newOrderedRangeParam(spec, parseDurationValue)}
}

func newUUIDParam(spec paramSpec) *UUIDParam {
	return &UUIDParam{value: newParamValue(spec, parseUUIDValue)}
}

func newTimeParam(spec paramSpec, parse func(string) (time.Time, error)) *TimeParam {
	return &TimeParam{value: newTimeRangeParam(spec, parse)}
}

func newValuesParam(spec paramSpec) *ValuesParam {
	return &ValuesParam{
		value: newMultiParamValue(spec, parseRawValues, cloneStringSlice),
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

type orderedRangeParam[T cmp.Ordered] struct {
	value paramValue[T]
	min   *T
	max   *T
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
	return p.value.resolve()
}

func (p *orderedRangeParam[T]) setMin(value T) {
	if p.max != nil && value > *p.max {
		p.value.setUsageErr(usageErrorf("minimum must be less than or equal to maximum"))
		return
	}
	p.min = &value
	p.value.addCheck(func(v T) error {
		if v < value {
			return errInvalidParamValue
		}
		return nil
	})
}

func (p *orderedRangeParam[T]) setMax(value T) {
	if p.min != nil && *p.min > value {
		p.value.setUsageErr(usageErrorf("maximum must be greater than or equal to minimum"))
		return
	}
	p.max = &value
	p.value.addCheck(func(v T) error {
		if v > value {
			return errInvalidParamValue
		}
		return nil
	})
}

type timeRangeParam struct {
	value  paramValue[time.Time]
	after  *time.Time
	before *time.Time
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
	return p.value.resolve()
}

func (p *timeRangeParam) setAfter(value time.Time) {
	if p.before != nil && value.After(*p.before) {
		p.value.setUsageErr(usageErrorf("after time must be less than or equal to before time"))
		return
	}
	p.after = &value
	p.value.addCheck(func(v time.Time) error {
		if v.Before(value) {
			return errInvalidParamValue
		}
		return nil
	})
}

func (p *timeRangeParam) setBefore(value time.Time) {
	if p.after != nil && p.after.After(value) {
		p.value.setUsageErr(usageErrorf("before time must be greater than or equal to after time"))
		return
	}
	p.before = &value
	p.value.addCheck(func(v time.Time) error {
		if v.After(value) {
			return errInvalidParamValue
		}
		return nil
	})
}

// StringParam 读取并校验 string 参数。
type StringParam struct {
	value paramValue[string]
}

// ValuesParam 读取并校验 query 多值参数的原始 []string。
type ValuesParam struct {
	value multiParamValue[[]string]
}

func (p *ValuesParam) Required() *ValuesParam {
	return requireParam(p, &p.value)
}

func (p *ValuesParam) Default(value []string) *ValuesParam {
	return defaultParam(p, &p.value, value)
}

func (p *ValuesParam) Check(check func([]string) error) *ValuesParam {
	return checkParam(p, p.value.addCheck, check)
}

func (p *ValuesParam) Get() ([]string, error) {
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
		p.value.setUsageErr(usageErrorf("maximum length must be >= 0"))
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
	return getParam(&p.value)
}

// IntParam 读取并校验 int 参数。
type IntParam struct {
	value orderedRangeParam[int]
}

func (p *IntParam) Required() *IntParam {
	return requireParam(p, &p.value)
}

func (p *IntParam) Default(value int) *IntParam {
	return defaultParam(p, &p.value, value)
}

func (p *IntParam) Min(value int) *IntParam {
	p.value.setMin(value)
	return p
}

func (p *IntParam) Max(value int) *IntParam {
	p.value.setMax(value)
	return p
}

func (p *IntParam) Check(check func(int) error) *IntParam {
	return checkParam(p, p.value.addCheck, check)
}

func (p *IntParam) Get() (int, error) {
	return getParam(&p.value)
}

// Int64Param 读取并校验 int64 参数。
type Int64Param struct {
	value orderedRangeParam[int64]
}

func (p *Int64Param) Required() *Int64Param {
	return requireParam(p, &p.value)
}

func (p *Int64Param) Default(value int64) *Int64Param {
	return defaultParam(p, &p.value, value)
}

func (p *Int64Param) Min(value int64) *Int64Param {
	p.value.setMin(value)
	return p
}

func (p *Int64Param) Max(value int64) *Int64Param {
	p.value.setMax(value)
	return p
}

func (p *Int64Param) Check(check func(int64) error) *Int64Param {
	return checkParam(p, p.value.addCheck, check)
}

func (p *Int64Param) Get() (int64, error) {
	return getParam(&p.value)
}

// UintParam 读取并校验 uint 参数。
type UintParam struct {
	value orderedRangeParam[uint]
}

func (p *UintParam) Required() *UintParam {
	return requireParam(p, &p.value)
}

func (p *UintParam) Default(value uint) *UintParam {
	return defaultParam(p, &p.value, value)
}

func (p *UintParam) Min(value uint) *UintParam {
	p.value.setMin(value)
	return p
}

func (p *UintParam) Max(value uint) *UintParam {
	p.value.setMax(value)
	return p
}

func (p *UintParam) Check(check func(uint) error) *UintParam {
	return checkParam(p, p.value.addCheck, check)
}

func (p *UintParam) Get() (uint, error) {
	return getParam(&p.value)
}

// Uint64Param 读取并校验 uint64 参数。
type Uint64Param struct {
	value orderedRangeParam[uint64]
}

func (p *Uint64Param) Required() *Uint64Param {
	return requireParam(p, &p.value)
}

func (p *Uint64Param) Default(value uint64) *Uint64Param {
	return defaultParam(p, &p.value, value)
}

func (p *Uint64Param) Min(value uint64) *Uint64Param {
	p.value.setMin(value)
	return p
}

func (p *Uint64Param) Max(value uint64) *Uint64Param {
	p.value.setMax(value)
	return p
}

func (p *Uint64Param) Check(check func(uint64) error) *Uint64Param {
	return checkParam(p, p.value.addCheck, check)
}

func (p *Uint64Param) Get() (uint64, error) {
	return getParam(&p.value)
}

// BoolParam 读取并校验 bool 参数。
type BoolParam struct {
	value paramValue[bool]
}

func (p *BoolParam) Required() *BoolParam {
	return requireParam(p, &p.value)
}

func (p *BoolParam) Default(value bool) *BoolParam {
	return defaultParam(p, &p.value, value)
}

func (p *BoolParam) Check(check func(bool) error) *BoolParam {
	return checkParam(p, p.value.addCheck, check)
}

func (p *BoolParam) Get() (bool, error) {
	return getParam(&p.value)
}

// Float64Param 读取并校验 float64 参数。
type Float64Param struct {
	value orderedRangeParam[float64]
}

func (p *Float64Param) Required() *Float64Param {
	return requireParam(p, &p.value)
}

func (p *Float64Param) Default(value float64) *Float64Param {
	return defaultParam(p, &p.value, value)
}

func (p *Float64Param) Min(value float64) *Float64Param {
	p.value.setMin(value)
	return p
}

func (p *Float64Param) Max(value float64) *Float64Param {
	p.value.setMax(value)
	return p
}

func (p *Float64Param) Check(check func(float64) error) *Float64Param {
	return checkParam(p, p.value.addCheck, check)
}

func (p *Float64Param) Get() (float64, error) {
	return getParam(&p.value)
}

// DurationParam 读取并校验 time.Duration 参数。
type DurationParam struct {
	value orderedRangeParam[time.Duration]
}

func (p *DurationParam) Required() *DurationParam {
	return requireParam(p, &p.value)
}

func (p *DurationParam) Default(value time.Duration) *DurationParam {
	return defaultParam(p, &p.value, value)
}

func (p *DurationParam) Min(value time.Duration) *DurationParam {
	p.value.setMin(value)
	return p
}

func (p *DurationParam) Max(value time.Duration) *DurationParam {
	p.value.setMax(value)
	return p
}

func (p *DurationParam) Check(check func(time.Duration) error) *DurationParam {
	return checkParam(p, p.value.addCheck, check)
}

func (p *DurationParam) Get() (time.Duration, error) {
	return getParam(&p.value)
}

// UUIDParam 读取并校验 uuid.UUID 参数。
type UUIDParam struct {
	value paramValue[uuid.UUID]
}

func (p *UUIDParam) Required() *UUIDParam {
	return requireParam(p, &p.value)
}

func (p *UUIDParam) Default(value uuid.UUID) *UUIDParam {
	return defaultParam(p, &p.value, value)
}

func (p *UUIDParam) Check(check func(uuid.UUID) error) *UUIDParam {
	return checkParam(p, p.value.addCheck, check)
}

func (p *UUIDParam) Get() (uuid.UUID, error) {
	return getParam(&p.value)
}

// TimeParam 读取并校验 time.Time 参数。
type TimeParam struct {
	value timeRangeParam
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

func parseRawValues(values []string) ([]string, error) {
	return cloneStringSlice(values), nil
}

func parseStringValue(value string) (string, error) {
	return value, nil
}

func defaultEmptyValue(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func parseIntBits(value string, bits int) (int64, error) {
	return strconv.ParseInt(defaultEmptyValue(value, "0"), 10, bits)
}

func parseUintBits(value string, bits int) (uint64, error) {
	return strconv.ParseUint(defaultEmptyValue(value, "0"), 10, bits)
}

func parseFloatBits(value string, bits int) (float64, error) {
	return strconv.ParseFloat(defaultEmptyValue(value, "0.0"), bits)
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
	return strconv.ParseBool(defaultEmptyValue(value, "false"))
}

func parseFloat64Value(value string) (float64, error) {
	return parseFloatBits(value, 64)
}

func parseDurationValue(value string) (time.Duration, error) {
	return time.ParseDuration(defaultEmptyValue(value, "0"))
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
