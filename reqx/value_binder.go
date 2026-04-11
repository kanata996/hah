package reqx

import (
	"encoding"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/kanata996/hah/bind"
)

type valueLookupFunc func(r *http.Request, name string) ([]string, bool)

// ValueBinder 提供 path/query 单字段链式绑定与 source-aware violation 收集。
type ValueBinder struct {
	r          *http.Request
	input      string
	lookup     valueLookupFunc
	failFast   bool
	usageErr   error
	violations []Violation
}

// PathValuesBinder 创建 path 参数值绑定器。
func PathValuesBinder(r *http.Request) *ValueBinder {
	return &ValueBinder{
		r:        r,
		input:    ViolationInPath,
		lookup:   pathParamValues,
		failFast: true,
	}
}

// QueryParamsBinder 创建 query 参数值绑定器。
func QueryParamsBinder(r *http.Request) *ValueBinder {
	return &ValueBinder{
		r:        r,
		input:    ViolationInQuery,
		lookup:   queryParamValues,
		failFast: true,
	}
}

// FailFast 控制出现绑定错误后，后续绑定步骤是否直接跳过。
func (b *ValueBinder) FailFast(value bool) *ValueBinder {
	if b == nil {
		return nil
	}
	b.failFast = value
	return b
}

// Bind 在参数存在时把值绑定到目标指针；缺失时保持 no-op。
func (b *ValueBinder) Bind(name string, dest any) *ValueBinder {
	return b.bind(name, dest, false)
}

// MustBind 要求参数存在；缺失时记录 required violation。
func (b *ValueBinder) MustBind(name string, dest any) *ValueBinder {
	return b.bind(name, dest, true)
}

// String 在参数存在时把值绑定到 string 变量；缺失时保持 no-op。
func (b *ValueBinder) String(name string, dest *string) *ValueBinder {
	return bindTypedValue(b, name, dest, false)
}

// MustString 要求参数存在并绑定到 string 变量。
func (b *ValueBinder) MustString(name string, dest *string) *ValueBinder {
	return bindTypedValue(b, name, dest, true)
}

// Strings 在参数存在时把值绑定到 []string 变量；缺失时保持 no-op。
func (b *ValueBinder) Strings(name string, dest *[]string) *ValueBinder {
	return bindTypedValue(b, name, dest, false)
}

// MustStrings 要求参数存在并绑定到 []string 变量。
func (b *ValueBinder) MustStrings(name string, dest *[]string) *ValueBinder {
	return bindTypedValue(b, name, dest, true)
}

// Int 在参数存在时把值绑定到 int 变量；缺失时保持 no-op。
func (b *ValueBinder) Int(name string, dest *int) *ValueBinder {
	return bindTypedValue(b, name, dest, false)
}

// MustInt 要求参数存在并绑定到 int 变量。
func (b *ValueBinder) MustInt(name string, dest *int) *ValueBinder {
	return bindTypedValue(b, name, dest, true)
}

// Ints 在参数存在时把值绑定到 []int 变量；缺失时保持 no-op。
func (b *ValueBinder) Ints(name string, dest *[]int) *ValueBinder {
	return bindTypedValue(b, name, dest, false)
}

// MustInts 要求参数存在并绑定到 []int 变量。
func (b *ValueBinder) MustInts(name string, dest *[]int) *ValueBinder {
	return bindTypedValue(b, name, dest, true)
}

// Int64 在参数存在时把值绑定到 int64 变量；缺失时保持 no-op。
func (b *ValueBinder) Int64(name string, dest *int64) *ValueBinder {
	return bindTypedValue(b, name, dest, false)
}

// MustInt64 要求参数存在并绑定到 int64 变量。
func (b *ValueBinder) MustInt64(name string, dest *int64) *ValueBinder {
	return bindTypedValue(b, name, dest, true)
}

// Int64s 在参数存在时把值绑定到 []int64 变量；缺失时保持 no-op。
func (b *ValueBinder) Int64s(name string, dest *[]int64) *ValueBinder {
	return bindTypedValue(b, name, dest, false)
}

// MustInt64s 要求参数存在并绑定到 []int64 变量。
func (b *ValueBinder) MustInt64s(name string, dest *[]int64) *ValueBinder {
	return bindTypedValue(b, name, dest, true)
}

// Uint 在参数存在时把值绑定到 uint 变量；缺失时保持 no-op。
func (b *ValueBinder) Uint(name string, dest *uint) *ValueBinder {
	return bindTypedValue(b, name, dest, false)
}

// MustUint 要求参数存在并绑定到 uint 变量。
func (b *ValueBinder) MustUint(name string, dest *uint) *ValueBinder {
	return bindTypedValue(b, name, dest, true)
}

// Uints 在参数存在时把值绑定到 []uint 变量；缺失时保持 no-op。
func (b *ValueBinder) Uints(name string, dest *[]uint) *ValueBinder {
	return bindTypedValue(b, name, dest, false)
}

// MustUints 要求参数存在并绑定到 []uint 变量。
func (b *ValueBinder) MustUints(name string, dest *[]uint) *ValueBinder {
	return bindTypedValue(b, name, dest, true)
}

// Bool 在参数存在时把值绑定到 bool 变量；缺失时保持 no-op。
func (b *ValueBinder) Bool(name string, dest *bool) *ValueBinder {
	return bindTypedValue(b, name, dest, false)
}

// MustBool 要求参数存在并绑定到 bool 变量。
func (b *ValueBinder) MustBool(name string, dest *bool) *ValueBinder {
	return bindTypedValue(b, name, dest, true)
}

// Bools 在参数存在时把值绑定到 []bool 变量；缺失时保持 no-op。
func (b *ValueBinder) Bools(name string, dest *[]bool) *ValueBinder {
	return bindTypedValue(b, name, dest, false)
}

// MustBools 要求参数存在并绑定到 []bool 变量。
func (b *ValueBinder) MustBools(name string, dest *[]bool) *ValueBinder {
	return bindTypedValue(b, name, dest, true)
}

// Float64 在参数存在时把值绑定到 float64 变量；缺失时保持 no-op。
func (b *ValueBinder) Float64(name string, dest *float64) *ValueBinder {
	return bindTypedValue(b, name, dest, false)
}

// MustFloat64 要求参数存在并绑定到 float64 变量。
func (b *ValueBinder) MustFloat64(name string, dest *float64) *ValueBinder {
	return bindTypedValue(b, name, dest, true)
}

// Float64s 在参数存在时把值绑定到 []float64 变量；缺失时保持 no-op。
func (b *ValueBinder) Float64s(name string, dest *[]float64) *ValueBinder {
	return bindTypedValue(b, name, dest, false)
}

// MustFloat64s 要求参数存在并绑定到 []float64 变量。
func (b *ValueBinder) MustFloat64s(name string, dest *[]float64) *ValueBinder {
	return bindTypedValue(b, name, dest, true)
}

// BindUnmarshaler 在参数存在时把值绑定到自定义单值解码目标；缺失时保持 no-op。
func (b *ValueBinder) BindUnmarshaler(name string, dest bind.BindUnmarshaler) *ValueBinder {
	return bindInterfaceValue(b, name, dest, false)
}

// MustBindUnmarshaler 要求参数存在并绑定到自定义单值解码目标。
func (b *ValueBinder) MustBindUnmarshaler(name string, dest bind.BindUnmarshaler) *ValueBinder {
	return bindInterfaceValue(b, name, dest, true)
}

// TextUnmarshaler 在参数存在时把值绑定到 TextUnmarshaler 目标；缺失时保持 no-op。
func (b *ValueBinder) TextUnmarshaler(name string, dest encoding.TextUnmarshaler) *ValueBinder {
	return bindInterfaceValue(b, name, dest, false)
}

// MustTextUnmarshaler 要求参数存在并绑定到 TextUnmarshaler 目标。
func (b *ValueBinder) MustTextUnmarshaler(name string, dest encoding.TextUnmarshaler) *ValueBinder {
	return bindInterfaceValue(b, name, dest, true)
}

// UUID 在参数存在时把值绑定到 uuid.UUID 变量；缺失时保持 no-op。
func (b *ValueBinder) UUID(name string, dest *uuid.UUID) *ValueBinder {
	return bindTypedValue(b, name, dest, false)
}

// MustUUID 要求参数存在并绑定到 uuid.UUID 变量。
func (b *ValueBinder) MustUUID(name string, dest *uuid.UUID) *ValueBinder {
	return bindTypedValue(b, name, dest, true)
}

// Time 在参数存在时按 RFC3339 绑定到 time.Time 变量；缺失时保持 no-op。
func (b *ValueBinder) Time(name string, dest *time.Time) *ValueBinder {
	return b.bindParsedTime(name, dest, false, parseRFC3339Time)
}

// MustTime 要求参数存在，并按 RFC3339 绑定到 time.Time 变量。
func (b *ValueBinder) MustTime(name string, dest *time.Time) *ValueBinder {
	return b.bindParsedTime(name, dest, true, parseRFC3339Time)
}

// UnixTime 在参数存在时按 10 位秒级 Unix 时间戳绑定到 time.Time 变量；缺失时保持 no-op。
func (b *ValueBinder) UnixTime(name string, dest *time.Time) *ValueBinder {
	return b.bindParsedTime(name, dest, false, parseUnixTime)
}

// MustUnixTime 要求参数存在，并按 10 位秒级 Unix 时间戳绑定到 time.Time 变量。
func (b *ValueBinder) MustUnixTime(name string, dest *time.Time) *ValueBinder {
	return b.bindParsedTime(name, dest, true, parseUnixTime)
}

// UnixMilliTime 在参数存在时按 13 位毫秒级 Unix 时间戳绑定到 time.Time 变量；缺失时保持 no-op。
func (b *ValueBinder) UnixMilliTime(name string, dest *time.Time) *ValueBinder {
	return b.bindParsedTime(name, dest, false, parseUnixMilliTime)
}

// MustUnixMilliTime 要求参数存在，并按 13 位毫秒级 Unix 时间戳绑定到 time.Time 变量。
func (b *ValueBinder) MustUnixMilliTime(name string, dest *time.Time) *ValueBinder {
	return b.bindParsedTime(name, dest, true, parseUnixMilliTime)
}

// BindError 返回首个收集到的错误，并清空内部错误状态。
func (b *ValueBinder) BindError() error {
	if b == nil {
		return nil
	}

	err := b.usageErr
	if err == nil && len(b.violations) > 0 {
		err = InvalidRequest(b.violations[0])
	}
	b.resetErrors()
	return err
}

// BindErrors 返回聚合后的 invalid_request 错误，并清空内部错误状态。
func (b *ValueBinder) BindErrors() error {
	if b == nil {
		return nil
	}

	err := b.usageErr
	if err == nil && len(b.violations) > 0 {
		err = InvalidRequest(b.violations...)
	}
	b.resetErrors()
	return err
}

func (b *ValueBinder) bind(name string, dest any, required bool) *ValueBinder {
	if b == nil {
		return nil
	}
	if b.shouldSkip() {
		return b
	}
	if !b.ensureConfigured() {
		return b
	}
	if b.r == nil {
		b.usageErr = errorsf("request must not be nil")
		return b
	}

	target, err := binderDestinationValue(dest)
	if err != nil {
		b.usageErr = err
		return b
	}

	values, exists := b.lookup(b.r, name)
	if !exists || len(values) == 0 {
		if required {
			b.addViolation(name, ViolationCodeRequired)
		}
		return b
	}

	if err := bindParamValues(target, values); err != nil {
		if isUnsupportedBindingDestination(err) {
			b.usageErr = errorsf("destination type %s is not supported", target.Type())
			return b
		}
		b.addViolation(name, ViolationCodeInvalid)
	}
	return b
}

func (b *ValueBinder) shouldSkip() bool {
	if !b.failFast {
		return false
	}
	return b.usageErr != nil || len(b.violations) > 0
}

func (b *ValueBinder) addViolation(field, code string) {
	b.violations = append(b.violations, Violation{
		Field: field,
		In:    b.input,
		Code:  code,
	})
}

func (b *ValueBinder) resetErrors() {
	b.usageErr = nil
	b.violations = nil
}

func (b *ValueBinder) ensureConfigured() bool {
	if b.lookup != nil && b.input != "" {
		return true
	}

	b.usageErr = errorsf("binder must be created with PathValuesBinder or QueryParamsBinder")
	return false
}

func binderDestinationValue(dest any) (reflect.Value, error) {
	if dest == nil {
		return reflect.Value{}, errorsf("destination must not be nil")
	}

	value := reflect.ValueOf(dest)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		return reflect.Value{}, errorsf("destination must be a non-nil pointer")
	}

	return value.Elem(), nil
}

func bindTypedValue[T any](b *ValueBinder, name string, dest *T, required bool) *ValueBinder {
	if required {
		return b.MustBind(name, dest)
	}
	return b.Bind(name, dest)
}

func bindInterfaceValue(b *ValueBinder, name string, dest any, required bool) *ValueBinder {
	if required {
		return b.MustBind(name, dest)
	}
	return b.Bind(name, dest)
}

func isUnsupportedBindingDestination(err error) bool {
	return errors.Is(err, errUnknownParamType)
}

func (b *ValueBinder) bindParsedTime(name string, dest *time.Time, required bool, parse func(string) (time.Time, error)) *ValueBinder {
	if b == nil {
		return nil
	}
	if b.shouldSkip() {
		return b
	}
	if !b.ensureConfigured() {
		return b
	}
	if b.r == nil {
		b.usageErr = errorsf("request must not be nil")
		return b
	}
	if _, err := binderDestinationValue(dest); err != nil {
		b.usageErr = err
		return b
	}

	values, exists := b.lookup(b.r, name)
	if !exists || len(values) == 0 {
		if required {
			b.addViolation(name, ViolationCodeRequired)
		}
		return b
	}

	parsed, err := parse(values[0])
	if err != nil {
		b.addViolation(name, ViolationCodeInvalid)
		return b
	}
	*dest = parsed
	return b
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
		return 0, fmt.Errorf("timestamp must be %d digits", digits)
	}
	return strconv.ParseInt(value, 10, 64)
}
