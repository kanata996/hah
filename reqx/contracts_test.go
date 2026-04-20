package reqx

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kanata996/hah/internal/errx"
)

func assertPublicType[T any](_, _ T) {}

func TestPublicBuilderFamilies(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?x=1", nil)

	assertPublicType(Path(req, "id").String(), (*StringParam)(nil))
	assertPublicType(Query(req, "name").String(), (*StringParam)(nil))

	assertPublicType(Path(req, "id").Int(), (*OrderedParam[int])(nil))
	assertPublicType(Path(req, "id").Int64(), (*OrderedParam[int64])(nil))
	assertPublicType(Path(req, "id").Uint(), (*OrderedParam[uint])(nil))
	assertPublicType(Path(req, "id").Uint64(), (*OrderedParam[uint64])(nil))
	assertPublicType(Path(req, "id").UUID(), (*ValueParam[uuid.UUID])(nil))

	assertPublicType(Query(req, "page").Int(), (*OrderedParam[int])(nil))
	assertPublicType(Query(req, "page").Int64(), (*OrderedParam[int64])(nil))
	assertPublicType(Query(req, "page").Uint(), (*OrderedParam[uint])(nil))
	assertPublicType(Query(req, "page").Uint64(), (*OrderedParam[uint64])(nil))
	assertPublicType(Query(req, "enabled").Bool(), (*ValueParam[bool])(nil))
	assertPublicType(Query(req, "score").Float64(), (*OrderedParam[float64])(nil))
	assertPublicType(Query(req, "wait").Duration(), (*OrderedParam[time.Duration])(nil))
	assertPublicType(Query(req, "token").UUID(), (*ValueParam[uuid.UUID])(nil))
	assertPublicType(Query(req, "when").Time(), (*TimeParam)(nil))
	assertPublicType(Query(req, "when").UnixTime(), (*TimeParam)(nil))
	assertPublicType(Query(req, "tag").Values(), (*MultiParam[string])(nil))
}

func TestUsageErrorf_PrefixesAndSupportsUnwrap(t *testing.T) {
	cause := errors.New("boom")

	err := usageErrorf("invalid config: %w", cause)
	if err == nil {
		t.Fatal("usageErrorf() error = nil, want non-nil")
	}
	if got := err.Error(); got != "reqx: invalid config: boom" {
		t.Fatalf("usageErrorf() error = %q, want %q", got, "reqx: invalid config: boom")
	}
	if !errors.Is(err, cause) {
		t.Fatalf("errors.Is(err, cause) = false, want true")
	}
}

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
