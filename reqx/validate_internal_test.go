package reqx

import "testing"

func TestNormalizeViolationDefaults(t *testing.T) {
	tests := []struct {
		name string
		in   Violation
		want Violation
	}{
		{
			name: "required",
			in:   Violation{Field: "name", Code: "required"},
			want: Violation{Field: "name", Code: "required", Message: "is required"},
		},
		{
			name: "unknown",
			in:   Violation{Field: "name", Code: "unknown"},
			want: Violation{Field: "name", Code: "unknown", Message: "unknown field"},
		},
		{
			name: "type",
			in:   Violation{Field: "name", Code: "type"},
			want: Violation{Field: "name", Code: "type", Message: "has invalid type"},
		},
		{
			name: "default",
			in:   Violation{Field: "name"},
			want: Violation{Field: "name", Code: "invalid", Message: "is invalid"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeViolation(tt.in); got != tt.want {
				t.Fatalf("normalizeViolation() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
