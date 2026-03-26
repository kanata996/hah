package reqx

import (
	"encoding"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type listUsersQuery struct {
	ID        string    `query:"id"`
	Page      int       `query:"page"`
	Active    bool      `query:"active"`
	Tags      []string  `query:"tag"`
	Limit     *int      `query:"limit"`
	CreatedAt time.Time `query:"created_at"`
	Mode      queryMode `query:"mode"`
	Ignored   string
}

type queryMode string

var _ encoding.TextUnmarshaler = (*queryMode)(nil)

func (m *queryMode) UnmarshalText(text []byte) error {
	switch value := queryMode(text); value {
	case "basic", "full":
		*m = value
		return nil
	default:
		return simpleError("invalid mode")
	}
}

func TestDecodeQuerySuccess(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/users?id=u_1&page=2&active=true&tag=a&tag=b&limit=50&created_at=2024-01-02T03:04:05Z&mode=full", nil)

	var got listUsersQuery
	if err := DecodeQuery(req, &got); err != nil {
		t.Fatalf("DecodeQuery() error = %v, want nil", err)
	}

	if got.ID != "u_1" {
		t.Fatalf("query.ID = %q, want u_1", got.ID)
	}
	if got.Page != 2 {
		t.Fatalf("query.Page = %d, want 2", got.Page)
	}
	if !got.Active {
		t.Fatal("query.Active = false, want true")
	}
	if len(got.Tags) != 2 || got.Tags[0] != "a" || got.Tags[1] != "b" {
		t.Fatalf("query.Tags = %#v, want [a b]", got.Tags)
	}
	if got.Limit == nil || *got.Limit != 50 {
		t.Fatalf("query.Limit = %#v, want 50", got.Limit)
	}
	if got.CreatedAt.Format(time.RFC3339) != "2024-01-02T03:04:05Z" {
		t.Fatalf("query.CreatedAt = %q, want RFC3339 value", got.CreatedAt.Format(time.RFC3339))
	}
	if got.Mode != "full" {
		t.Fatalf("query.Mode = %q, want full", got.Mode)
	}
	if got.Ignored != "" {
		t.Fatalf("query.Ignored = %q, want empty string", got.Ignored)
	}
}

func TestDecodeQueryLeavesMissingValuesZeroed(t *testing.T) {
	var query struct {
		Page  int  `query:"page"`
		Limit *int `query:"limit"`
	}

	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	if err := DecodeQuery(req, &query); err != nil {
		t.Fatalf("DecodeQuery() error = %v, want nil", err)
	}
	if query.Page != 0 {
		t.Fatalf("query.Page = %d, want 0", query.Page)
	}
	if query.Limit != nil {
		t.Fatalf("query.Limit = %#v, want nil", query.Limit)
	}
}

func TestDecodeAndValidateQueryRejectsViolations(t *testing.T) {
	var query struct {
		Page int `query:"page"`
	}

	req := httptest.NewRequest(http.MethodGet, "/users?page=0", nil)
	err := DecodeAndValidateQuery(req, &query, func(value *struct {
		Page int `query:"page"`
	}) []Violation {
		if value.Page < 1 {
			return []Violation{{Field: "page", Code: "min", Message: "must be at least 1"}}
		}
		return nil
	})

	assertProblem(
		t,
		err,
		http.StatusUnprocessableEntity,
		"invalid_request",
		"request contains invalid fields",
		Violation{Field: "page", Code: "min", Message: "must be at least 1"},
	)
}

func TestDecodeAndValidateQueryReturnsDecodeError(t *testing.T) {
	var query struct {
		Page int `query:"page"`
	}

	req := httptest.NewRequest(http.MethodGet, "/users?page=abc", nil)
	called := false
	err := DecodeAndValidateQuery(req, &query, func(value *struct {
		Page int `query:"page"`
	}) []Violation {
		called = true
		return nil
	})

	assertProblem(
		t,
		err,
		http.StatusUnprocessableEntity,
		"invalid_request",
		"request contains invalid fields",
		Violation{Field: "page", Code: "type", Message: "must be number"},
	)
	if called {
		t.Fatal("validator should not be called when query decode fails")
	}
}

func TestDecodeAndValidateQuerySuccess(t *testing.T) {
	type queryInput struct {
		Page int `query:"page"`
	}

	req := httptest.NewRequest(http.MethodGet, "/users?page=2", nil)

	var query queryInput
	if err := DecodeAndValidateQuery(req, &query, func(value *queryInput) []Violation {
		if value.Page < 1 {
			return []Violation{{Field: "page", Code: "min", Message: "must be at least 1"}}
		}
		return nil
	}); err != nil {
		t.Fatalf("DecodeAndValidateQuery() error = %v, want nil", err)
	}
	if query.Page != 2 {
		t.Fatalf("query.Page = %d, want 2", query.Page)
	}
}

func TestDecodeQueryRejectsUnknownFields(t *testing.T) {
	var query struct {
		ID string `query:"id"`
	}

	req := httptest.NewRequest(http.MethodGet, "/users?id=u_1&extra=yes", nil)
	err := DecodeQuery(req, &query)

	assertProblem(
		t,
		err,
		http.StatusUnprocessableEntity,
		"invalid_request",
		"request contains invalid fields",
		Violation{Field: "extra", Code: "unknown", Message: "unknown field"},
	)
}

func TestDecodeQueryCanAllowUnknownFields(t *testing.T) {
	var query struct {
		ID string `query:"id"`
	}

	req := httptest.NewRequest(http.MethodGet, "/users?id=u_1&extra=yes", nil)
	if err := DecodeQuery(req, &query, AllowUnknownQueryFields()); err != nil {
		t.Fatalf("DecodeQuery() error = %v, want nil", err)
	}
	if query.ID != "u_1" {
		t.Fatalf("query.ID = %q, want u_1", query.ID)
	}
}

func TestDecodeQueryRejectsInvalidScalarType(t *testing.T) {
	var query struct {
		Page int `query:"page"`
	}

	req := httptest.NewRequest(http.MethodGet, "/users?page=abc", nil)
	err := DecodeQuery(req, &query)

	assertProblem(
		t,
		err,
		http.StatusUnprocessableEntity,
		"invalid_request",
		"request contains invalid fields",
		Violation{Field: "page", Code: "type", Message: "must be number"},
	)
}

func TestDecodeQueryRejectsRepeatedScalarField(t *testing.T) {
	var query struct {
		Page int `query:"page"`
	}

	req := httptest.NewRequest(http.MethodGet, "/users?page=1&page=2", nil)
	err := DecodeQuery(req, &query)

	assertProblem(
		t,
		err,
		http.StatusUnprocessableEntity,
		"invalid_request",
		"request contains invalid fields",
		Violation{Field: "page", Code: "multiple", Message: "must not be repeated"},
	)
}

func TestDecodeQueryRejectsInvalidTextUnmarshalerValue(t *testing.T) {
	var query struct {
		Mode queryMode `query:"mode"`
	}

	req := httptest.NewRequest(http.MethodGet, "/users?mode=broken", nil)
	err := DecodeQuery(req, &query)

	assertProblem(
		t,
		err,
		http.StatusUnprocessableEntity,
		"invalid_request",
		"request contains invalid fields",
		Violation{Field: "mode", Code: "invalid", Message: "is invalid"},
	)
}

func TestDecodeQuerySupportsPointerTextUnmarshaler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/users?created_at=2024-01-02T03:04:05Z", nil)

	var query struct {
		CreatedAt *time.Time `query:"created_at"`
	}
	if err := DecodeQuery(req, &query); err != nil {
		t.Fatalf("DecodeQuery() error = %v, want nil", err)
	}
	if query.CreatedAt == nil {
		t.Fatal("query.CreatedAt = nil, want non-nil")
	}
	if got := query.CreatedAt.Format(time.RFC3339); got != "2024-01-02T03:04:05Z" {
		t.Fatalf("query.CreatedAt = %q, want RFC3339 value", got)
	}
}

func TestDecodeQueryRejectsNilRequest(t *testing.T) {
	var query struct {
		ID string `query:"id"`
	}

	err := DecodeQuery(nil, &query)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); got != "hah/reqx: request must not be nil" {
		t.Fatalf("error = %q, want request must not be nil", got)
	}
}

func TestDecodeQueryRejectsNilDestination(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/users?id=u_1", nil)

	err := DecodeQuery[struct {
		ID string `query:"id"`
	}](req, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); got != "hah/reqx: destination must not be nil" {
		t.Fatalf("error = %q, want destination must not be nil", got)
	}
}

func TestDecodeQueryRejectsNonStructDestination(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/users?page=1", nil)

	var page int
	err := DecodeQuery(req, &page)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); got != "hah/reqx: destination must point to a struct" {
		t.Fatalf("error = %q, want destination must point to a struct", got)
	}
}

func TestDecodeQueryRejectsUnsupportedFieldType(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/users", nil)

	var query struct {
		Meta map[string]string `query:"meta"`
	}
	err := DecodeQuery(req, &query)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); got != `hah/reqx: field "Meta" has unsupported query type map[string]string` {
		t.Fatalf("error = %q, want unsupported field type", got)
	}
}

func TestDecodeQueryRejectsDuplicateQueryTags(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/users", nil)

	var query struct {
		ID    string `query:"id"`
		Other string `query:"id"`
	}
	err := DecodeQuery(req, &query)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); got != `hah/reqx: duplicate query field "id" on ID and Other` {
		t.Fatalf("error = %q, want duplicate query field", got)
	}
}

func TestDecodeQueryRejectsUnexportedTaggedField(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/users", nil)

	var query struct {
		id string `query:"id"`
	}
	err := DecodeQuery(req, &query)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); got != `hah/reqx: query field "id" must be exported` {
		t.Fatalf("error = %q, want tagged field must be exported", got)
	}
}

func TestDecodeQueryAggregatesMultipleViolations(t *testing.T) {
	var query struct {
		Page int `query:"page"`
	}

	req := httptest.NewRequest(http.MethodGet, "/users?page=abc&extra=yes", nil)
	err := DecodeQuery(req, &query)

	assertProblem(
		t,
		err,
		http.StatusUnprocessableEntity,
		"invalid_request",
		"request contains invalid fields",
		Violation{Field: "page", Code: "type", Message: "must be number"},
		Violation{Field: "extra", Code: "unknown", Message: "unknown field"},
	)
}

func TestDecodeQueryIgnoresDashTaggedFields(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/users?ignored=value", nil)

	var query struct {
		Ignored string `query:"-"`
	}
	if err := DecodeQuery(req, &query, AllowUnknownQueryFields()); err != nil {
		t.Fatalf("DecodeQuery() error = %v, want nil", err)
	}
	if query.Ignored != "" {
		t.Fatalf("query.Ignored = %q, want empty string", query.Ignored)
	}
}

func TestDecodeQueryTrimsNothingFromStringValues(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/users?name=+alice+smith+", nil)

	var query struct {
		Name string `query:"name"`
	}
	if err := DecodeQuery(req, &query); err != nil {
		t.Fatalf("DecodeQuery() error = %v, want nil", err)
	}
	if got := strings.TrimSpace(query.Name); got != "alice smith" {
		t.Fatalf("trimmed query.Name = %q, want alice smith", got)
	}
	if query.Name != " alice smith " {
		t.Fatalf("query.Name = %q, want spaces preserved", query.Name)
	}
}

func TestDecodeQueryHandlesNilURLAndNilOption(t *testing.T) {
	req := &http.Request{}

	var query struct {
		ID string `query:"id"`
	}
	if err := DecodeQuery(req, &query, nil); err != nil {
		t.Fatalf("DecodeQuery() error = %v, want nil", err)
	}
	if query.ID != "" {
		t.Fatalf("query.ID = %q, want empty string", query.ID)
	}
}

func TestDecodeQueryReturnsDecoderError(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/users?mode=full", nil)

	var query struct {
		Mode encoding.TextUnmarshaler `query:"mode"`
	}
	err := DecodeQuery(req, &query)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); got != "hah/reqx: unsupported text unmarshaler destination type encoding.TextUnmarshaler" {
		t.Fatalf("error = %q, want unsupported text unmarshaler destination type", got)
	}
}
