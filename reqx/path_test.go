package reqx

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kanata996/hah/errx"
)

func TestPathBuilder_Contracts(t *testing.T) {
	t.Run("required missing path returns required violation", func(t *testing.T) {
		_, err := Path(requestWithPathParams(nil), "id").String().Required().Get()
		assertRequiredViolationAt(t, err, "id", errx.InPath)
	})

	t.Run("declared empty wildcard counts as present", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Pattern = "/accounts/{id}"
		req.SetPathValue("id", "")

		got, err := Path(req, "id").String().Required().Get()
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if got != "" {
			t.Fatalf("id = %q, want empty string", got)
		}
	})

	t.Run("malformed pattern does not make empty value present", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Pattern = "/accounts/{id"
		req.SetPathValue("id", "")

		_, err := Path(req, "id").String().Required().Get()
		assertRequiredViolationAt(t, err, "id", errx.InPath)
	})

	t.Run("empty string only string accepts", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Pattern = "/accounts/{id}"
		req.SetPathValue("id", "")

		_, err := Path(req, "id").Int().Required().Get()
		assertInvalidViolationAt(t, err, "id", errx.InPath)
	})

	t.Run("default validation failure is usage error", func(t *testing.T) {
		_, err := Path(requestWithPathParams(nil), "since").String().
			Default(time.Now().Format(time.RFC3339)).
			MinLen(100).
			Get()
		assertNotHTTPError(t, err)
	})

	t.Run("uuid parse failure is invalid violation", func(t *testing.T) {
		req := requestWithPathParams(map[string][]string{"id": {"not-a-uuid"}})

		_, err := Path(req, "id").UUID().Get()
		assertInvalidViolationAt(t, err, "id", errx.InPath)
	})

	t.Run("uuid parse success", func(t *testing.T) {
		want := uuid.New()
		req := requestWithPathParams(map[string][]string{"id": {want.String()}})

		got, err := Path(req, "id").UUID().Required().Get()
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if got != want {
			t.Fatalf("uuid = %v, want %v", got, want)
		}
	})
}
