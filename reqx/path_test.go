package reqx

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func TestPathBuilder_Contracts(t *testing.T) {
	t.Run("required missing path returns required field error", func(t *testing.T) {
		_, err := Path(requestWithPathParams(nil), "id").String().Required().Get()
		assertRequiredFieldErrorAt(t, err, "id", InPath)
	})

	t.Run("query values do not satisfy path lookup", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/?id=42", nil)

		_, err := Path(req, "id").String().Required().Get()
		assertRequiredFieldErrorAt(t, err, "id", InPath)
	})

	t.Run("empty path value counts as missing", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.SetPathValue("id", "")

		_, err := Path(req, "id").String().Required().Get()
		assertRequiredFieldErrorAt(t, err, "id", InPath)
	})

	t.Run("request check failure keeps stable invalid detail", func(t *testing.T) {
		req := requestWithPathParams(map[string][]string{"id": {"kanata"}})

		_, err := Path(req, "id").String().
			Check(func(string) error { return errors.New("must be numeric") }).
			Get()
		assertInvalidFieldErrorAt(t, err, "id", InPath)
	})

	t.Run("uuid parse failure is invalid field error", func(t *testing.T) {
		req := requestWithPathParams(map[string][]string{"id": {"not-a-uuid"}})

		_, err := Path(req, "id").UUID().Get()
		assertInvalidFieldErrorAt(t, err, "id", InPath)
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

func TestPathBuilder_BaselineContracts(t *testing.T) {
	t.Run("nil request and empty name are usage errors", func(t *testing.T) {
		_, err := Path(nil, "id").String().Get()
		assertNotHTTPError(t, err)

		_, err = Path(httptest.NewRequest(http.MethodGet, "/", nil), " ").String().Get()
		assertNotHTTPError(t, err)
	})

	t.Run("optional missing returns zero and required is idempotent", func(t *testing.T) {
		got, err := Path(requestWithPathParams(nil), "count").Int().Get()
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if got != 0 {
			t.Fatalf("count = %d, want 0", got)
		}
	})

	t.Run("bridge path values are consumed as set without unescape", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.SetPathValue("id", "a%2Fb")

		got, err := Path(req, " id ").String().Required().Get()
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if got != "a%2Fb" {
			t.Fatalf("id = %q, want a%%2Fb", got)
		}
	})
}

func TestPathBuilder_ServeMuxPathValueContracts(t *testing.T) {
	t.Run("non empty wildcard matches request path value", func(t *testing.T) {
		mux := http.NewServeMux()

		var got string
		var pathValue string
		var handlerErr error

		mux.HandleFunc("GET /accounts/{id}", func(w http.ResponseWriter, r *http.Request) {
			pathValue = r.PathValue("id")
			got, handlerErr = Path(r, "id").String().Required().Get()
			w.WriteHeader(http.StatusNoContent)
		})

		req := httptest.NewRequest(http.MethodGet, "/accounts/kanata", nil)
		mux.ServeHTTP(httptest.NewRecorder(), req)

		if handlerErr != nil {
			t.Fatalf("Get() error = %v", handlerErr)
		}
		if pathValue != "kanata" {
			t.Fatalf("request.PathValue(id) = %q, want kanata", pathValue)
		}
		if got != pathValue {
			t.Fatalf("Path().String().Get() = %q, want %q", got, pathValue)
		}
	})

	t.Run("empty catch all path value counts as missing", func(t *testing.T) {
		mux := http.NewServeMux()

		var handlerErr error

		mux.HandleFunc("GET /files/{rest...}", func(w http.ResponseWriter, r *http.Request) {
			_, handlerErr = Path(r, "rest").String().Required().Get()
			w.WriteHeader(http.StatusNoContent)
		})

		req := httptest.NewRequest(http.MethodGet, "/files/", nil)
		mux.ServeHTTP(httptest.NewRecorder(), req)

		assertRequiredFieldErrorAt(t, handlerErr, "rest", InPath)
	})
}

func TestPathBuilder_InternalLookupContracts(t *testing.T) {
	t.Run("empty path value is missing", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.SetPathValue("id", "")

		got, ok := pathParamValues(req, "id")
		if ok {
			t.Fatal("pathParamValues(req, id) reported present for empty value")
		}
		if got != nil {
			t.Fatalf("pathParamValues(req, id) = %v, want nil", got)
		}
	})
}
