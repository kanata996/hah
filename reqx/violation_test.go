package reqx

// 测试清单：
// - 标记说明：[✓] 已核对且已有真实覆盖；[x] 尚未完成，不得作为验收依据。
// - [✓] `invalidFieldsError` 会把 violation 规范化后写成稳定的 invalid_request 错误。
// - [✓] `normalizeViolation` 会按 violation code 补齐默认 detail 与默认 code。

import "testing"

// invalidFieldsError 会把 violation 默认值补齐后写成统一错误。
func TestInvalidFieldsError_NormalizesViolation(t *testing.T) {
	err := invalidFieldsError([]Violation{{Field: "name"}})
	violation := assertSingleViolation(t, err)
	if violation.Field != "name" || violation.Code != ViolationCodeInvalid || violation.Detail != "is invalid" {
		t.Fatalf("violation = %#v", violation)
	}
}

// normalizeViolation 会按错误码补齐默认错误信息。
func TestNormalizeViolationBranches(t *testing.T) {
	testCases := []struct {
		name string
		in   Violation
		want Violation
	}{
		{
			name: "required",
			in:   Violation{Field: "name", Code: ViolationCodeRequired},
			want: Violation{Field: "name", Code: ViolationCodeRequired, Detail: "is required"},
		},
		{
			name: "unknown",
			in:   Violation{Field: "name", Code: ViolationCodeUnknown},
			want: Violation{Field: "name", Code: ViolationCodeUnknown, Detail: "unknown field"},
		},
		{
			name: "type",
			in:   Violation{Field: "name", Code: ViolationCodeType},
			want: Violation{Field: "name", Code: ViolationCodeType, Detail: "has invalid type"},
		},
		{
			name: "multiple",
			in:   Violation{Field: "name", Code: ViolationCodeMultiple},
			want: Violation{Field: "name", Code: ViolationCodeMultiple, Detail: "must not be repeated"},
		},
		{
			name: "default",
			in:   Violation{Field: "name"},
			want: Violation{Field: "name", Code: ViolationCodeInvalid, Detail: "is invalid"},
		},
		{
			name: "explicit detail",
			in:   Violation{Field: "name", Code: ViolationCodeInvalid, Detail: "custom"},
			want: Violation{Field: "name", Code: ViolationCodeInvalid, Detail: "custom"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeViolation(tc.in); got != tc.want {
				t.Fatalf("normalizeViolation() = %#v, want %#v", got, tc.want)
			}
		})
	}
}
