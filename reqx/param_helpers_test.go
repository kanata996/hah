package reqx

import "testing"

// 测试清单：
// - 标记说明：[✓] 已核对且已有真实覆盖；[x] 尚未完成，不得作为验收依据。
// - [✓] 内部 parse helper 会维持标量空值默认与时间戳格式错误分支；仅作为辅助覆盖，不替代公开 builder 验收。

func TestScalarParseHelpers_EmptyAndInvalidBranches(t *testing.T) {
	t.Run("string passthrough", func(t *testing.T) {
		got, err := parseStringValue("kanata")
		if err != nil || got != "kanata" {
			t.Fatalf("parseStringValue() = (%q, %v), want (kanata, nil)", got, err)
		}
	})

	t.Run("int empty and invalid", func(t *testing.T) {
		if got, err := parseIntValue(""); err != nil || got != 0 {
			t.Fatalf("parseIntValue(empty) = (%d, %v), want (0, nil)", got, err)
		}
		if _, err := parseIntValue("oops"); err == nil {
			t.Fatal("parseIntValue(invalid) = nil, want error")
		}
	})

	t.Run("int64 empty and invalid", func(t *testing.T) {
		if got, err := parseInt64Value(""); err != nil || got != 0 {
			t.Fatalf("parseInt64Value(empty) = (%d, %v), want (0, nil)", got, err)
		}
		if _, err := parseInt64Value("oops"); err == nil {
			t.Fatal("parseInt64Value(invalid) = nil, want error")
		}
	})

	t.Run("uint empty and invalid", func(t *testing.T) {
		if got, err := parseUintValue(""); err != nil || got != 0 {
			t.Fatalf("parseUintValue(empty) = (%d, %v), want (0, nil)", got, err)
		}
		if _, err := parseUintValue("oops"); err == nil {
			t.Fatal("parseUintValue(invalid) = nil, want error")
		}
	})

	t.Run("bool empty and invalid", func(t *testing.T) {
		if got, err := parseBoolValue(""); err != nil || got {
			t.Fatalf("parseBoolValue(empty) = (%v, %v), want (false, nil)", got, err)
		}
		if _, err := parseBoolValue("oops"); err == nil {
			t.Fatal("parseBoolValue(invalid) = nil, want error")
		}
	})

	t.Run("float64 empty and invalid", func(t *testing.T) {
		if got, err := parseFloat64Value(""); err != nil || got != 0 {
			t.Fatalf("parseFloat64Value(empty) = (%v, %v), want (0, nil)", got, err)
		}
		if _, err := parseFloat64Value("oops"); err == nil {
			t.Fatal("parseFloat64Value(invalid) = nil, want error")
		}
	})
}

func TestTimeParseHelpers_InvalidBranches(t *testing.T) {
	t.Run("rfc3339 invalid", func(t *testing.T) {
		if _, err := parseRFC3339Time("not-a-time"); err == nil {
			t.Fatal("parseRFC3339Time(invalid) = nil, want error")
		}
	})

	t.Run("unix time width and numeric errors", func(t *testing.T) {
		if _, err := parseUnixTime("123"); err == nil {
			t.Fatal("parseUnixTime(short) = nil, want error")
		}
		if _, err := parseUnixTime("abcdefghij"); err == nil {
			t.Fatal("parseUnixTime(non-numeric) = nil, want error")
		}
	})

	t.Run("unix milli width and numeric errors", func(t *testing.T) {
		if _, err := parseUnixMilliTime("123"); err == nil {
			t.Fatal("parseUnixMilliTime(short) = nil, want error")
		}
		if _, err := parseUnixMilliTime("abcdefghijklm"); err == nil {
			t.Fatal("parseUnixMilliTime(non-numeric) = nil, want error")
		}
	})
}
