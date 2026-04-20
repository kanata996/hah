package reqx

import (
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kanata996/hah/internal/errx"
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
			input:  errx.InQuery,
			lookup: queryParamValues,
		},
	}
}

// String 读取 string 参数。
func (p *QueryParam) String() *StringParam {
	return newStringParam(p.spec)
}

// Int 读取 int 参数。
func (p *QueryParam) Int() *OrderedParam[int] {
	return newOrderedParam(p.spec, parseIntValue)
}

// Int64 读取 int64 参数。
func (p *QueryParam) Int64() *OrderedParam[int64] {
	return newOrderedParam(p.spec, parseInt64Value)
}

// Uint64 读取 uint64 参数。
func (p *QueryParam) Uint64() *OrderedParam[uint64] {
	return newOrderedParam(p.spec, parseUint64Value)
}

// Uint 读取 uint 参数。
func (p *QueryParam) Uint() *OrderedParam[uint] {
	return newOrderedParam(p.spec, parseUintValue)
}

// Bool 读取 bool 参数。
func (p *QueryParam) Bool() *ValueParam[bool] {
	return newValueParam(p.spec, parseBoolValue)
}

// Float64 读取 float64 参数。
func (p *QueryParam) Float64() *OrderedParam[float64] {
	param := newOrderedParam(p.spec, parseFloat64Value)
	param.valueValidator = validateFiniteFloat64
	param.boundValidator = validateFiniteFloat64
	return param
}

// Duration 读取 time.Duration 参数。
func (p *QueryParam) Duration() *OrderedParam[time.Duration] {
	return newOrderedParam(p.spec, parseDurationValue)
}

// UUID 读取 uuid.UUID 参数。
func (p *QueryParam) UUID() *ValueParam[uuid.UUID] {
	return newValueParam(p.spec, parseUUIDValue)
}

// Time 按 RFC3339 读取 time.Time 参数。
func (p *QueryParam) Time() *TimeParam {
	return newTimeParam(p.spec, parseRFC3339Time)
}

// UnixTime 按 10 位秒级 Unix 时间戳读取 time.Time 参数。
func (p *QueryParam) UnixTime() *TimeParam {
	return newTimeParam(p.spec, parseUnixTime)
}

// Values 读取 query 参数的全部解析后值。
func (p *QueryParam) Values() *MultiParam[string] {
	return newMultiParam(
		p.spec,
		func(values []string) []string { return values },
		cloneSlice[string],
	)
}

func queryParamValues(r *http.Request, name string) ([]string, bool) {
	if r == nil || r.URL == nil {
		return nil, false
	}
	values, exists := r.URL.Query()[name]
	return values, exists
}
