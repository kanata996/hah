package reqx

import (
	"errors"
	"regexp"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

func newStringParam(spec paramSpec, builderNil bool) *StringParam {
	return &StringParam{value: newParamValue(spec, builderNil, parseStringValue)}
}

func newIntParam(spec paramSpec, builderNil bool) *IntParam {
	return &IntParam{value: newParamValue(spec, builderNil, parseIntValue)}
}

func newInt64Param(spec paramSpec, builderNil bool) *Int64Param {
	return &Int64Param{value: newParamValue(spec, builderNil, parseInt64Value)}
}

func newUint64Param(spec paramSpec, builderNil bool) *Uint64Param {
	return &Uint64Param{value: newParamValue(spec, builderNil, parseUint64Value)}
}

func newUintParam(spec paramSpec, builderNil bool) *UintParam {
	return &UintParam{value: newParamValue(spec, builderNil, parseUintValue)}
}

func newBoolParam(spec paramSpec, builderNil bool) *BoolParam {
	return &BoolParam{value: newParamValue(spec, builderNil, parseBoolValue)}
}

func newFloat64Param(spec paramSpec, builderNil bool) *Float64Param {
	return &Float64Param{value: newParamValue(spec, builderNil, parseFloat64Value)}
}

func newDurationParam(spec paramSpec, builderNil bool) *DurationParam {
	return &DurationParam{value: newParamValue(spec, builderNil, parseDurationValue)}
}

func newUUIDParam(spec paramSpec, builderNil bool) *UUIDParam {
	return &UUIDParam{value: newParamValue(spec, builderNil, parseUUIDValue)}
}

func newTimeParam(spec paramSpec, builderNil bool, parse func(string) (time.Time, error)) *TimeParam {
	return &TimeParam{value: newParamValue(spec, builderNil, parse)}
}

func newValuesParam(spec paramSpec, builderNil bool) *ValuesParam {
	return &ValuesParam{
		value: newMultiParamValue(spec, builderNil, parseRawValues, cloneStringSlice),
	}
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
	p.value.setRequired()
	return p
}

func (p *ValuesParam) Default(value []string) *ValuesParam {
	p.value.setDefault(value)
	return p
}

func (p *ValuesParam) Check(check func([]string) error) *ValuesParam {
	p.value.addCheck(check)
	return p
}

func (p *ValuesParam) Get() ([]string, error) {
	return p.value.resolve()
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

// Uint64Param 读取并校验 uint64 参数。
type Uint64Param struct {
	value paramValue[uint64]
	min   *uint64
	max   *uint64
}

func (p *Uint64Param) Required() *Uint64Param {
	p.value.setRequired()
	return p
}

func (p *Uint64Param) Default(value uint64) *Uint64Param {
	p.value.setDefault(value)
	return p
}

func (p *Uint64Param) Min(value uint64) *Uint64Param {
	if p.max != nil && value > *p.max {
		p.value.setUsageErr(errorsf("minimum must be less than or equal to maximum"))
		return p
	}
	p.min = &value
	p.value.addCheck(func(v uint64) error {
		if v < value {
			return errInvalidParamValue
		}
		return nil
	})
	return p
}

func (p *Uint64Param) Max(value uint64) *Uint64Param {
	if p.min != nil && *p.min > value {
		p.value.setUsageErr(errorsf("maximum must be greater than or equal to minimum"))
		return p
	}
	p.max = &value
	p.value.addCheck(func(v uint64) error {
		if v > value {
			return errInvalidParamValue
		}
		return nil
	})
	return p
}

func (p *Uint64Param) Check(check func(uint64) error) *Uint64Param {
	p.value.addCheck(check)
	return p
}

func (p *Uint64Param) Get() (uint64, error) {
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

// DurationParam 读取并校验 time.Duration 参数。
type DurationParam struct {
	value paramValue[time.Duration]
	min   *time.Duration
	max   *time.Duration
}

func (p *DurationParam) Required() *DurationParam {
	p.value.setRequired()
	return p
}

func (p *DurationParam) Default(value time.Duration) *DurationParam {
	p.value.setDefault(value)
	return p
}

func (p *DurationParam) Min(value time.Duration) *DurationParam {
	if p.max != nil && value > *p.max {
		p.value.setUsageErr(errorsf("minimum must be less than or equal to maximum"))
		return p
	}
	p.min = &value
	p.value.addCheck(func(v time.Duration) error {
		if v < value {
			return errInvalidParamValue
		}
		return nil
	})
	return p
}

func (p *DurationParam) Max(value time.Duration) *DurationParam {
	if p.min != nil && *p.min > value {
		p.value.setUsageErr(errorsf("maximum must be greater than or equal to minimum"))
		return p
	}
	p.max = &value
	p.value.addCheck(func(v time.Duration) error {
		if v > value {
			return errInvalidParamValue
		}
		return nil
	})
	return p
}

func (p *DurationParam) Check(check func(time.Duration) error) *DurationParam {
	p.value.addCheck(check)
	return p
}

func (p *DurationParam) Get() (time.Duration, error) {
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

func parseRawValues(values []string) ([]string, error) {
	return cloneStringSlice(values), nil
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

func parseUint64Value(value string) (uint64, error) {
	if value == "" {
		value = "0"
	}
	return strconv.ParseUint(value, 10, 64)
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

func parseDurationValue(value string) (time.Duration, error) {
	if value == "" {
		value = "0"
	}
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
