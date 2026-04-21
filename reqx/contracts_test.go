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

func TestInvalidRequest_UsesFieldErrorEnvelope(t *testing.T) {
	testCases := []struct {
		name string
		in   errx.FieldError
		want errx.FieldError
	}{
		{
			name: "default invalid",
			in:   errx.FieldError{Field: "name"},
			want: errx.FieldError{Field: "name", Code: errx.CodeInvalid, Detail: "is invalid"},
		},
		{
			name: "required",
			in:   errx.FieldError{Field: "body", In: errx.InBody, Code: errx.CodeRequired},
			want: errx.FieldError{Field: "body", In: errx.InBody, Code: errx.CodeRequired, Detail: "is required"},
		},
		{
			name: "unknown",
			in:   errx.FieldError{Field: "extra", In: errx.InQuery, Code: errx.CodeUnknown},
			want: errx.FieldError{Field: "extra", In: errx.InQuery, Code: errx.CodeUnknown, Detail: "unknown field"},
		},
		{
			name: "type",
			in:   errx.FieldError{Field: "limit", In: errx.InBody, Code: errx.CodeType},
			want: errx.FieldError{Field: "limit", In: errx.InBody, Code: errx.CodeType, Detail: "has invalid type"},
		},
		{
			name: "multiple",
			in:   errx.FieldError{Field: "X-Trace-Id", In: errx.InHeader, Code: errx.CodeMultiple},
			want: errx.FieldError{Field: "X-Trace-Id", In: errx.InHeader, Code: errx.CodeMultiple, Detail: "must appear only once"},
		},
		{
			name: "custom detail is preserved",
			in:   errx.FieldError{Field: "name", Detail: "must be unique"},
			want: errx.FieldError{Field: "name", Code: errx.CodeInvalid, Detail: "must be unique"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fieldError := assertSingleFieldError(t, InvalidRequest(tc.in))
			if fieldError != tc.want {
				t.Fatalf("field error = %#v, want %#v", fieldError, tc.want)
			}
		})
	}

	t.Run("multiple field errors are preserved in order", func(t *testing.T) {
		got := assertFieldErrors(t, InvalidRequest(
			errx.FieldError{Field: "page", In: errx.InQuery},
			errx.FieldError{Field: "body", In: errx.InBody, Code: errx.CodeRequired},
		))

		want := []errx.FieldError{
			{Field: "page", In: errx.InQuery, Code: errx.CodeInvalid, Detail: "is invalid"},
			{Field: "body", In: errx.InBody, Code: errx.CodeRequired, Detail: "is required"},
		}
		if len(got) != len(want) {
			t.Fatalf("field errors len = %d, want %d", len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("fieldErrors[%d] = %#v, want %#v", i, got[i], want[i])
			}
		}
	})

	t.Run("does not mutate caller owned field error slice", func(t *testing.T) {
		fieldErrors := []errx.FieldError{
			{Field: "page", In: errx.InQuery},
			{Field: "body", In: errx.InBody, Code: errx.CodeRequired},
		}
		wantOriginal := append([]errx.FieldError(nil), fieldErrors...)

		got := assertFieldErrors(t, InvalidRequest(fieldErrors...))

		want := []errx.FieldError{
			{Field: "page", In: errx.InQuery, Code: errx.CodeInvalid, Detail: "is invalid"},
			{Field: "body", In: errx.InBody, Code: errx.CodeRequired, Detail: "is required"},
		}
		if len(got) != len(want) {
			t.Fatalf("field errors len = %d, want %d", len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("fieldErrors[%d] = %#v, want %#v", i, got[i], want[i])
			}
		}
		for i := range wantOriginal {
			if fieldErrors[i] != wantOriginal[i] {
				t.Fatalf("caller fieldErrors[%d] = %#v, want %#v", i, fieldErrors[i], wantOriginal[i])
			}
		}
	})
}
