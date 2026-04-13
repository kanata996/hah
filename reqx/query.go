package reqx

import (
	"net/http"
	"strings"
)

// QueryParam 表示一个待解析的 query 单参数。
type QueryParam struct {
	spec paramSpec
}

// Query 创建 query 单参数读取与校验 builder。
func Query(r *http.Request, name string) *QueryParam {
	return &QueryParam{
		spec: paramSpec{
			r:      r,
			name:   strings.TrimSpace(name),
			input:  ViolationInQuery,
			lookup: queryParamValues,
		},
	}
}

func (p *QueryParam) specOrZero() paramSpec {
	if p == nil {
		return paramSpec{}
	}
	return p.spec
}

// String 读取 string 参数。
func (p *QueryParam) String() *StringParam {
	return newStringParam(p.specOrZero(), p == nil)
}

// Int 读取 int 参数。
func (p *QueryParam) Int() *IntParam {
	return newIntParam(p.specOrZero(), p == nil)
}

// Int64 读取 int64 参数。
func (p *QueryParam) Int64() *Int64Param {
	return newInt64Param(p.specOrZero(), p == nil)
}

// Uint64 读取 uint64 参数。
func (p *QueryParam) Uint64() *Uint64Param {
	return newUint64Param(p.specOrZero(), p == nil)
}

// Uint 读取 uint 参数。
func (p *QueryParam) Uint() *UintParam {
	return newUintParam(p.specOrZero(), p == nil)
}

// Bool 读取 bool 参数。
func (p *QueryParam) Bool() *BoolParam {
	return newBoolParam(p.specOrZero(), p == nil)
}

// Float64 读取 float64 参数。
func (p *QueryParam) Float64() *Float64Param {
	return newFloat64Param(p.specOrZero(), p == nil)
}

// Duration 读取 time.Duration 参数。
func (p *QueryParam) Duration() *DurationParam {
	return newDurationParam(p.specOrZero(), p == nil)
}

// UUID 读取 uuid.UUID 参数。
func (p *QueryParam) UUID() *UUIDParam {
	return newUUIDParam(p.specOrZero(), p == nil)
}

// Time 按 RFC3339 读取 time.Time 参数。
func (p *QueryParam) Time() *TimeParam {
	return newTimeParam(p.specOrZero(), p == nil, parseRFC3339Time)
}

// UnixTime 按 10 位秒级 Unix 时间戳读取 time.Time 参数。
func (p *QueryParam) UnixTime() *TimeParam {
	return newTimeParam(p.specOrZero(), p == nil, parseUnixTime)
}

// UnixMilliTime 按 13 位毫秒级 Unix 时间戳读取 time.Time 参数。
func (p *QueryParam) UnixMilliTime() *TimeParam {
	return newTimeParam(p.specOrZero(), p == nil, parseUnixMilliTime)
}

// Values 读取 query 参数的全部原始值。
func (p *QueryParam) Values() *ValuesParam {
	return newValuesParam(p.specOrZero(), p == nil)
}

// Strings 是 Values 的别名。
func (p *QueryParam) Strings() *ValuesParam {
	return newValuesParam(p.specOrZero(), p == nil)
}

func queryParamValues(r *http.Request, name string) ([]string, bool) {
	if r == nil || r.URL == nil {
		return nil, false
	}
	values, exists := r.URL.Query()[name]
	return values, exists
}
