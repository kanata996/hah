package reqx

import (
	"testing"

	"github.com/kanata996/hah/internal/errx"
)

func TestInvalidRequest_UsesViolationEnvelope(t *testing.T) {
	testCases := []struct {
		name string
		in   errx.Violation
		want errx.Violation
	}{
		{
			name: "default invalid",
			in:   errx.Violation{Field: "name"},
			want: errx.Violation{Field: "name", Code: errx.CodeInvalid, Detail: "is invalid"},
		},
		{
			name: "required",
			in:   errx.Violation{Field: "body", In: errx.InBody, Code: errx.CodeRequired},
			want: errx.Violation{Field: "body", In: errx.InBody, Code: errx.CodeRequired, Detail: "is required"},
		},
		{
			name: "unknown",
			in:   errx.Violation{Field: "extra", In: errx.InQuery, Code: errx.CodeUnknown},
			want: errx.Violation{Field: "extra", In: errx.InQuery, Code: errx.CodeUnknown, Detail: "unknown field"},
		},
		{
			name: "type",
			in:   errx.Violation{Field: "limit", In: errx.InBody, Code: errx.CodeType},
			want: errx.Violation{Field: "limit", In: errx.InBody, Code: errx.CodeType, Detail: "has invalid type"},
		},
		{
			name: "multiple",
			in:   errx.Violation{Field: "X-Trace-Id", In: errx.InHeader, Code: errx.CodeMultiple},
			want: errx.Violation{Field: "X-Trace-Id", In: errx.InHeader, Code: errx.CodeMultiple, Detail: "must appear only once"},
		},
		{
			name: "custom detail is preserved",
			in:   errx.Violation{Field: "name", Detail: "must be unique"},
			want: errx.Violation{Field: "name", Code: errx.CodeInvalid, Detail: "must be unique"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			violation := assertSingleViolation(t, InvalidRequest(tc.in))
			if violation != tc.want {
				t.Fatalf("violation = %#v, want %#v", violation, tc.want)
			}
		})
	}

	t.Run("multiple violations are preserved in order", func(t *testing.T) {
		got := assertViolations(t, InvalidRequest(
			errx.Violation{Field: "page", In: errx.InQuery},
			errx.Violation{Field: "body", In: errx.InBody, Code: errx.CodeRequired},
		))

		want := []errx.Violation{
			{Field: "page", In: errx.InQuery, Code: errx.CodeInvalid, Detail: "is invalid"},
			{Field: "body", In: errx.InBody, Code: errx.CodeRequired, Detail: "is required"},
		}
		if len(got) != len(want) {
			t.Fatalf("violations len = %d, want %d", len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("violations[%d] = %#v, want %#v", i, got[i], want[i])
			}
		}
	})
}
