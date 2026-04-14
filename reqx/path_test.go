package reqx

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func TestPath_SuccessPaths(t *testing.T) {
	t.Run("path uuid required", func(t *testing.T) {
		want := uuid.New()
		req := requestWithPathParams(map[string][]string{"id": {want.String()}})

		got, err := Path(req, "id").UUID().Required().Get()
		if err != nil {
			t.Fatalf("Path().UUID().Required().Get() error = %v", err)
		}
		if got != want {
			t.Fatalf("uuid = %v, want %v", got, want)
		}
	})

	t.Run("path supported identifier types", func(t *testing.T) {
		req := requestWithPathParams(map[string][]string{
			"slug": {"acct_123"},
			"n":    {"7"},
			"n64":  {"9"},
			"u":    {"11"},
			"u64":  {"13"},
		})

		slug, err := Path(req, "slug").String().Required().Get()
		if err != nil || slug != "acct_123" {
			t.Fatalf("Path().String().Get() = (%q, %v), want (acct_123, nil)", slug, err)
		}

		n, err := Path(req, "n").Int().Required().Get()
		if err != nil || n != 7 {
			t.Fatalf("Path().Int().Get() = (%d, %v), want (7, nil)", n, err)
		}

		n64, err := Path(req, "n64").Int64().Required().Get()
		if err != nil || n64 != 9 {
			t.Fatalf("Path().Int64().Get() = (%d, %v), want (9, nil)", n64, err)
		}

		u, err := Path(req, "u").Uint().Required().Get()
		if err != nil || u != 11 {
			t.Fatalf("Path().Uint().Get() = (%d, %v), want (11, nil)", u, err)
		}

		u64, err := Path(req, "u64").Uint64().Required().Get()
		if err != nil || u64 != 13 {
			t.Fatalf("Path().Uint64().Get() = (%d, %v), want (13, nil)", u64, err)
		}
	})

	t.Run("declared empty wildcard counts as present", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/files", nil)
		req.Pattern = "/files/{path...}"
		req.SetPathValue("path", "")

		got, err := Path(req, "path").String().Required().Get()
		if err != nil {
			t.Fatalf("Path().String().Required().Get() error = %v", err)
		}
		if got != "" {
			t.Fatalf("path = %q, want empty string", got)
		}
	})
}

func TestPath_RequiredAndInvalidViolations(t *testing.T) {
	t.Run("required missing path", func(t *testing.T) {
		req := requestWithPathParams(nil)

		_, err := Path(req, "id").String().Required().Get()
		assertRequiredViolationAt(t, err, "id", ViolationInPath)
	})

	t.Run("invalid path uuid", func(t *testing.T) {
		req := requestWithPathParams(map[string][]string{"id": {"not-a-uuid"}})

		_, err := Path(req, "id").UUID().Get()
		assertInvalidViolationAt(t, err, "id", ViolationInPath)
	})

	t.Run("invalid path int", func(t *testing.T) {
		req := requestWithPathParams(map[string][]string{"id": {"oops"}})

		_, err := Path(req, "id").Int().Get()
		assertInvalidViolationAt(t, err, "id", ViolationInPath)
	})
}

func TestPathBuilder_UsageAndOptionalBehavior(t *testing.T) {
	t.Run("nil request", func(t *testing.T) {
		_, err := Path(nil, "id").String().Get()
		assertUsageErrorContains(t, err, "request must not be nil")
	})

	t.Run("empty name", func(t *testing.T) {
		_, err := Path(requestWithPathParams(map[string][]string{"id": {"u_1"}}), " ").String().Get()
		assertUsageErrorContains(t, err, "parameter name must not be empty")
	})

	t.Run("nil path builder", func(t *testing.T) {
		var p *PathParam

		_, err := p.String().Get()
		assertUsageErrorContains(t, err, "param builder must not be nil")
	})

	t.Run("zero path builder", func(t *testing.T) {
		_, err := (&PathParam{}).String().Get()
		assertUsageErrorContains(t, err, "param builder must be created with Path or Query")
	})

	t.Run("missing optional returns zero", func(t *testing.T) {
		got, err := Path(requestWithPathParams(nil), "id").String().Get()
		if err != nil {
			t.Fatalf("Path().String().Get() error = %v", err)
		}
		if got != "" {
			t.Fatalf("id = %q, want empty string", got)
		}
	})
}
