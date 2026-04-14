package reqx

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

// These tests lock the high-level public contract of reqx.Path / reqx.Query.
// Lower-level typed builders have broader coverage in typed_* tests; this file
// focuses on the core shape that should remain stable across refactors.
func TestPathAndQuery_CoreContracts(t *testing.T) {
	t.Run("path required violation stays path-scoped", func(t *testing.T) {
		req := requestWithPathParams(nil)

		_, err := Path(req, "account_id").String().Required().Get()
		assertRequiredViolationAt(t, err, "account_id", ViolationInPath)
	})

	t.Run("declared empty path wildcard still counts as present", func(t *testing.T) {
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

	t.Run("query scalar uses first repeated value", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?page=5&page=9", nil)

		got, err := Query(req, "page").Int().Required().Get()
		if err != nil {
			t.Fatalf("Query().Int().Required().Get() error = %v", err)
		}
		if got != 5 {
			t.Fatalf("page = %d, want 5", got)
		}
	})

	t.Run("query raw values preserve repeat order", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?tag=a&tag=&tag=b", nil)

		got, err := Query(req, "tag").Values().Required().Get()
		if err != nil {
			t.Fatalf("Query().Values().Required().Get() error = %v", err)
		}
		if !reflect.DeepEqual(got, []string{"a", "", "b"}) {
			t.Fatalf("tag = %#v, want [a \"\" b]", got)
		}
	})

	t.Run("required query string treats explicit empty value as present", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?name=", nil)

		got, err := Query(req, "name").String().Required().Get()
		if err != nil {
			t.Fatalf("Query().String().Required().Get() error = %v", err)
		}
		if got != "" {
			t.Fatalf("name = %q, want empty string", got)
		}
	})
}
