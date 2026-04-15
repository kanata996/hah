package reqx

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kanata996/hah/errx"
)

type numericBuilderOps[T comparable] struct {
	requiredMinMaxCheckGet func(min, max T) (T, error)
	defaultGet             func(def T) (T, error)
	requiredGet            func() (T, error)
	defaultMinGet          func(def, min T) error
	defaultCheckGet        func(def T, detail string) error
	minGet                 func(min T) error
	maxGet                 func(max T) error
	maxThenMinGet          func(max, min T) error
	minThenMaxGet          func(min, max T) error
	parseGet               func() error
}

type numericContract[T comparable] struct {
	field              string
	boundaryTarget     string
	emptyTarget        string
	minInvalidTarget   string
	maxInvalidTarget   string
	parseInvalidTarget string
	boundaryValue      T
	defaultValue       T
	minValue           T
	maxValue           T
	conflictLow        T
	conflictHigh       T
	defaultDetail      string
	newBuilder         func(req *http.Request, field string) numericBuilderOps[T]
}

func TestIntParam_ValidationAndRangeErrors(t *testing.T) {
	runNumericContract(t, numericContract[int]{
		field:              "page",
		boundaryTarget:     "/items?page=0",
		emptyTarget:        "/items?page=",
		minInvalidTarget:   "/items?page=1",
		maxInvalidTarget:   "/items?page=3",
		parseInvalidTarget: "/items?page=oops",
		boundaryValue:      0,
		defaultValue:       9,
		minValue:           2,
		maxValue:           2,
		conflictLow:        1,
		conflictHigh:       2,
		defaultDetail:      "default int must be rejected",
		newBuilder: func(req *http.Request, field string) numericBuilderOps[int] {
			return numericBuilderOps[int]{
				requiredMinMaxCheckGet: func(min, max int) (int, error) {
					return Query(req, field).Int().Required().Min(min).Max(max).Check(func(value int) error { return nil }).Get()
				},
				defaultGet: func(def int) (int, error) {
					return Query(req, field).Int().Default(def).Get()
				},
				requiredGet: func() (int, error) {
					return Query(req, field).Int().Required().Get()
				},
				defaultMinGet: func(def, min int) error {
					_, err := Query(req, field).Int().Default(def).Min(min).Get()
					return err
				},
				defaultCheckGet: func(def int, detail string) error {
					_, err := Query(req, field).Int().Default(def).Check(func(value int) error {
						return errors.New(detail)
					}).Get()
					return err
				},
				minGet: func(min int) error {
					_, err := Query(req, field).Int().Min(min).Get()
					return err
				},
				maxGet: func(max int) error {
					_, err := Query(req, field).Int().Max(max).Get()
					return err
				},
				maxThenMinGet: func(max, min int) error {
					_, err := Query(req, field).Int().Max(max).Min(min).Get()
					return err
				},
				minThenMaxGet: func(min, max int) error {
					_, err := Query(req, field).Int().Min(min).Max(max).Get()
					return err
				},
				parseGet: func() error {
					_, err := Query(req, field).Int().Get()
					return err
				},
			}
		},
	})
}

func TestInt64Param_ValidationAndRangeErrors(t *testing.T) {
	runNumericContract(t, numericContract[int64]{
		field:              "v",
		boundaryTarget:     "/items?v=0",
		emptyTarget:        "/items?v=",
		minInvalidTarget:   "/items?v=1",
		maxInvalidTarget:   "/items?v=3",
		parseInvalidTarget: "/items?v=oops",
		boundaryValue:      0,
		defaultValue:       9,
		minValue:           2,
		maxValue:           2,
		conflictLow:        1,
		conflictHigh:       2,
		defaultDetail:      "default int64 must be rejected",
		newBuilder: func(req *http.Request, field string) numericBuilderOps[int64] {
			return numericBuilderOps[int64]{
				requiredMinMaxCheckGet: func(min, max int64) (int64, error) {
					return Query(req, field).Int64().Required().Min(min).Max(max).Check(func(value int64) error { return nil }).Get()
				},
				defaultGet: func(def int64) (int64, error) {
					return Query(req, field).Int64().Default(def).Get()
				},
				requiredGet: func() (int64, error) {
					return Query(req, field).Int64().Required().Get()
				},
				defaultMinGet: func(def, min int64) error {
					_, err := Query(req, field).Int64().Default(def).Min(min).Get()
					return err
				},
				defaultCheckGet: func(def int64, detail string) error {
					_, err := Query(req, field).Int64().Default(def).Check(func(value int64) error {
						return errors.New(detail)
					}).Get()
					return err
				},
				minGet: func(min int64) error {
					_, err := Query(req, field).Int64().Min(min).Get()
					return err
				},
				maxGet: func(max int64) error {
					_, err := Query(req, field).Int64().Max(max).Get()
					return err
				},
				maxThenMinGet: func(max, min int64) error {
					_, err := Query(req, field).Int64().Max(max).Min(min).Get()
					return err
				},
				minThenMaxGet: func(min, max int64) error {
					_, err := Query(req, field).Int64().Min(min).Max(max).Get()
					return err
				},
				parseGet: func() error {
					_, err := Query(req, field).Int64().Get()
					return err
				},
			}
		},
	})
}

func TestUintParam_ValidationAndRangeErrors(t *testing.T) {
	runNumericContract(t, numericContract[uint]{
		field:              "v",
		boundaryTarget:     "/items?v=0",
		emptyTarget:        "/items?v=",
		minInvalidTarget:   "/items?v=1",
		maxInvalidTarget:   "/items?v=3",
		parseInvalidTarget: "/items?v=-1",
		boundaryValue:      0,
		defaultValue:       7,
		minValue:           2,
		maxValue:           2,
		conflictLow:        1,
		conflictHigh:       2,
		defaultDetail:      "default uint must be rejected",
		newBuilder: func(req *http.Request, field string) numericBuilderOps[uint] {
			return numericBuilderOps[uint]{
				requiredMinMaxCheckGet: func(min, max uint) (uint, error) {
					return Query(req, field).Uint().Required().Min(min).Max(max).Check(func(value uint) error { return nil }).Get()
				},
				defaultGet: func(def uint) (uint, error) {
					return Query(req, field).Uint().Default(def).Get()
				},
				requiredGet: func() (uint, error) {
					return Query(req, field).Uint().Required().Get()
				},
				defaultMinGet: func(def, min uint) error {
					_, err := Query(req, field).Uint().Default(def).Min(min).Get()
					return err
				},
				defaultCheckGet: func(def uint, detail string) error {
					_, err := Query(req, field).Uint().Default(def).Check(func(value uint) error {
						return errors.New(detail)
					}).Get()
					return err
				},
				minGet: func(min uint) error {
					_, err := Query(req, field).Uint().Min(min).Get()
					return err
				},
				maxGet: func(max uint) error {
					_, err := Query(req, field).Uint().Max(max).Get()
					return err
				},
				maxThenMinGet: func(max, min uint) error {
					_, err := Query(req, field).Uint().Max(max).Min(min).Get()
					return err
				},
				minThenMaxGet: func(min, max uint) error {
					_, err := Query(req, field).Uint().Min(min).Max(max).Get()
					return err
				},
				parseGet: func() error {
					_, err := Query(req, field).Uint().Get()
					return err
				},
			}
		},
	})
}

func TestUint64Param_ValidationAndRangeErrors(t *testing.T) {
	runNumericContract(t, numericContract[uint64]{
		field:              "v",
		boundaryTarget:     "/items?v=0",
		emptyTarget:        "/items?v=",
		minInvalidTarget:   "/items?v=1",
		maxInvalidTarget:   "/items?v=3",
		parseInvalidTarget: "/items?v=-1",
		boundaryValue:      0,
		defaultValue:       11,
		minValue:           2,
		maxValue:           2,
		conflictLow:        1,
		conflictHigh:       2,
		defaultDetail:      "default uint64 must be rejected",
		newBuilder: func(req *http.Request, field string) numericBuilderOps[uint64] {
			return numericBuilderOps[uint64]{
				requiredMinMaxCheckGet: func(min, max uint64) (uint64, error) {
					return Query(req, field).Uint64().Required().Min(min).Max(max).Check(func(value uint64) error { return nil }).Get()
				},
				defaultGet: func(def uint64) (uint64, error) {
					return Query(req, field).Uint64().Default(def).Get()
				},
				requiredGet: func() (uint64, error) {
					return Query(req, field).Uint64().Required().Get()
				},
				defaultMinGet: func(def, min uint64) error {
					_, err := Query(req, field).Uint64().Default(def).Min(min).Get()
					return err
				},
				defaultCheckGet: func(def uint64, detail string) error {
					_, err := Query(req, field).Uint64().Default(def).Check(func(value uint64) error {
						return errors.New(detail)
					}).Get()
					return err
				},
				minGet: func(min uint64) error {
					_, err := Query(req, field).Uint64().Min(min).Get()
					return err
				},
				maxGet: func(max uint64) error {
					_, err := Query(req, field).Uint64().Max(max).Get()
					return err
				},
				maxThenMinGet: func(max, min uint64) error {
					_, err := Query(req, field).Uint64().Max(max).Min(min).Get()
					return err
				},
				minThenMaxGet: func(min, max uint64) error {
					_, err := Query(req, field).Uint64().Min(min).Max(max).Get()
					return err
				},
				parseGet: func() error {
					_, err := Query(req, field).Uint64().Get()
					return err
				},
			}
		},
	})
}

func TestFloat64Param_ValidationAndRangeErrors(t *testing.T) {
	runNumericContract(t, numericContract[float64]{
		field:              "price",
		boundaryTarget:     "/items?price=0",
		emptyTarget:        "/items?price=",
		minInvalidTarget:   "/items?price=1",
		maxInvalidTarget:   "/items?price=3",
		parseInvalidTarget: "/items?price=oops",
		boundaryValue:      0,
		defaultValue:       1.25,
		minValue:           2,
		maxValue:           2,
		conflictLow:        1,
		conflictHigh:       2,
		defaultDetail:      "default float64 must be rejected",
		newBuilder: func(req *http.Request, field string) numericBuilderOps[float64] {
			return numericBuilderOps[float64]{
				requiredMinMaxCheckGet: func(min, max float64) (float64, error) {
					return Query(req, field).Float64().Required().Min(min).Max(max).Check(func(value float64) error { return nil }).Get()
				},
				defaultGet: func(def float64) (float64, error) {
					return Query(req, field).Float64().Default(def).Get()
				},
				requiredGet: func() (float64, error) {
					return Query(req, field).Float64().Required().Get()
				},
				defaultMinGet: func(def, min float64) error {
					_, err := Query(req, field).Float64().Default(def).Min(min).Get()
					return err
				},
				defaultCheckGet: func(def float64, detail string) error {
					_, err := Query(req, field).Float64().Default(def).Check(func(value float64) error {
						return errors.New(detail)
					}).Get()
					return err
				},
				minGet: func(min float64) error {
					_, err := Query(req, field).Float64().Min(min).Get()
					return err
				},
				maxGet: func(max float64) error {
					_, err := Query(req, field).Float64().Max(max).Get()
					return err
				},
				maxThenMinGet: func(max, min float64) error {
					_, err := Query(req, field).Float64().Max(max).Min(min).Get()
					return err
				},
				minThenMaxGet: func(min, max float64) error {
					_, err := Query(req, field).Float64().Min(min).Max(max).Get()
					return err
				},
				parseGet: func() error {
					_, err := Query(req, field).Float64().Get()
					return err
				},
			}
		},
	})
}

func TestOrderedRangeParam_RepeatedBoundsUseLatestConfiguration(t *testing.T) {
	t.Run("latest min overrides earlier min check", func(t *testing.T) {
		got, err := Query(httptest.NewRequest(http.MethodGet, "/items?page=7", nil), "page").Int().
			Min(10).
			Min(5).
			Max(7).
			Get()
		if err != nil {
			t.Fatalf("Int().Min().Min().Max().Get() error = %v", err)
		}
		if got != 7 {
			t.Fatalf("page = %d, want 7", got)
		}
	})

	t.Run("later bounds can recover from earlier conflict", func(t *testing.T) {
		got, err := Query(httptest.NewRequest(http.MethodGet, "/items?page=7", nil), "page").Int().
			Min(10).
			Max(7).
			Min(5).
			Get()
		if err != nil {
			t.Fatalf("Int().Min().Max().Min().Get() error = %v", err)
		}
		if got != 7 {
			t.Fatalf("page = %d, want 7", got)
		}
	})

	t.Run("repeated min after check preserves custom detail precedence", func(t *testing.T) {
		_, err := Query(httptest.NewRequest(http.MethodGet, "/items?page=3", nil), "page").Int().
			Min(1).
			Check(func(value int) error {
				if value == 3 {
					return errors.New("custom numeric detail")
				}
				return nil
			}).
			Min(5).
			Get()
		assertViolation(t, err, errx.Violation{
			Field:  "page",
			In:     errx.ViolationInQuery,
			Code:   errx.ViolationCodeInvalid,
			Detail: "custom numeric detail",
		})
	})
}

func runNumericContract[T comparable](t *testing.T, contract numericContract[T]) {
	t.Helper()

	t.Run("boundary success", func(t *testing.T) {
		got, err := contract.newBuilder(newNumericRequest(contract.boundaryTarget), contract.field).
			requiredMinMaxCheckGet(contract.boundaryValue, contract.boundaryValue)
		if err != nil {
			t.Fatalf("Required().Min().Max().Check().Get() error = %v", err)
		}
		if got != contract.boundaryValue {
			t.Fatalf("%s = %v, want %v", contract.field, got, contract.boundaryValue)
		}
	})

	t.Run("default success", func(t *testing.T) {
		got, err := contract.newBuilder(newNumericRequest("/items"), contract.field).defaultGet(contract.defaultValue)
		if err != nil || got != contract.defaultValue {
			t.Fatalf("Default().Get() = (%v, %v), want (%v, nil)", got, err, contract.defaultValue)
		}
	})

	t.Run("empty string parses zero when required", func(t *testing.T) {
		got, err := contract.newBuilder(newNumericRequest(contract.emptyTarget), contract.field).requiredGet()
		var zero T
		if err != nil || got != zero {
			t.Fatalf("Required().Get() = (%v, %v), want (%v, nil)", got, err, zero)
		}
	})

	t.Run("default min invalid", func(t *testing.T) {
		err := contract.newBuilder(newNumericRequest("/items"), contract.field).defaultMinGet(contract.conflictLow, contract.minValue)
		assertInvalidViolationAt(t, err, contract.field, errx.ViolationInQuery)
	})

	t.Run("default check invalid uses custom detail", func(t *testing.T) {
		err := contract.newBuilder(newNumericRequest("/items"), contract.field).defaultCheckGet(contract.defaultValue, contract.defaultDetail)
		assertViolation(t, err, errx.Violation{
			Field:  contract.field,
			In:     errx.ViolationInQuery,
			Code:   errx.ViolationCodeInvalid,
			Detail: contract.defaultDetail,
		})
	})

	t.Run("min invalid", func(t *testing.T) {
		err := contract.newBuilder(newNumericRequest(contract.minInvalidTarget), contract.field).minGet(contract.minValue)
		assertInvalidViolationAt(t, err, contract.field, errx.ViolationInQuery)
	})

	t.Run("max invalid", func(t *testing.T) {
		err := contract.newBuilder(newNumericRequest(contract.maxInvalidTarget), contract.field).maxGet(contract.maxValue)
		assertInvalidViolationAt(t, err, contract.field, errx.ViolationInQuery)
	})

	t.Run("max then min conflict", func(t *testing.T) {
		err := contract.newBuilder(newNumericRequest(contract.maxInvalidTarget), contract.field).
			maxThenMinGet(contract.conflictLow, contract.conflictHigh)
		assertUsageErrorContains(t, err, "minimum must be less than or equal to maximum")
	})

	t.Run("min then max conflict", func(t *testing.T) {
		err := contract.newBuilder(newNumericRequest(contract.maxInvalidTarget), contract.field).
			minThenMaxGet(contract.conflictHigh, contract.conflictLow)
		assertUsageErrorContains(t, err, "maximum must be greater than or equal to minimum")
	})

	t.Run("parse invalid", func(t *testing.T) {
		err := contract.newBuilder(newNumericRequest(contract.parseInvalidTarget), contract.field).parseGet()
		assertInvalidViolationAt(t, err, contract.field, errx.ViolationInQuery)
	})
}

func newNumericRequest(target string) *http.Request {
	return httptest.NewRequest(http.MethodGet, target, nil)
}
