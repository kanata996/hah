package reqx

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
)

type bindQueryNamedString string
type bindQueryNamedSlice []string

type bindQueryTextValue string

func (*bindQueryTextValue) UnmarshalText([]byte) error { return nil }

func TestBindQuery_Contracts(t *testing.T) {
	t.Run("binds supported leaf types and inline structs", func(t *testing.T) {
		type inlineFields struct {
			When  time.Time     `query:"when"`
			Wait  time.Duration `query:"wait"`
			Token uuid.UUID     `query:"token"`
		}
		type request struct {
			Name    bindQueryNamedString `query:"name"`
			Enabled bool                 `query:"enabled"`
			Count   *int                 `query:"count"`
			Inline  inlineFields         `query:",inline"`
		}

		token := uuid.New()
		req := httptest.NewRequest(http.MethodGet, "/?name=kanata&enabled=true&count=7&when=2026-04-13T10:00:00Z&wait=5s&token="+token.String(), nil)

		var dst request
		if err := BindQuery(req, &dst); err != nil {
			t.Fatalf("BindQuery() error = %v", err)
		}
		if dst.Name != "kanata" || !dst.Enabled || dst.Count == nil || *dst.Count != 7 || dst.Inline.Wait != 5*time.Second || dst.Inline.Token != token {
			t.Fatalf("dst = %#v, want bound supported fields", dst)
		}
		if got := dst.Inline.When.UTC().Format(time.RFC3339); got != "2026-04-13T10:00:00Z" {
			t.Fatalf("when = %q, want 2026-04-13T10:00:00Z", got)
		}
	})

	t.Run("missing keys do not inherit existing values", func(t *testing.T) {
		type request struct {
			Name    string `query:"name"`
			Enabled bool   `query:"enabled"`
		}

		dst := request{Name: "existing", Enabled: true}
		if err := BindQuery(httptest.NewRequest(http.MethodGet, "/", nil), &dst); err != nil {
			t.Fatalf("BindQuery() error = %v", err)
		}
		if dst != (request{}) {
			t.Fatalf("dst = %#v, want zero value struct", dst)
		}
	})

	t.Run("duplicate key returns bad request and preserves target", func(t *testing.T) {
		type request struct {
			Page int `query:"page"`
		}

		dst := request{Page: 9}
		err := BindQuery(httptest.NewRequest(http.MethodGet, "/?page=1&page=2", nil), &dst)
		_ = assertBadRequest(t, err)
		if dst != (request{Page: 9}) {
			t.Fatalf("dst = %#v, want unchanged", dst)
		}
	})

	t.Run("unknown repeated key also returns bad request", func(t *testing.T) {
		type request struct {
			Name string `query:"name"`
		}

		err := BindQuery(httptest.NewRequest(http.MethodGet, "/?extra=1&extra=2", nil), &request{})
		_ = assertBadRequest(t, err)
	})

	t.Run("empty string only string family accepts", func(t *testing.T) {
		type request struct {
			Name    bindQueryNamedString `query:"name"`
			Enabled bool                 `query:"enabled"`
		}

		var dst request
		if err := BindQuery(httptest.NewRequest(http.MethodGet, "/?name=", nil), &dst); err != nil {
			t.Fatalf("BindQuery() string family error = %v", err)
		}
		if dst.Name != "" {
			t.Fatalf("name = %q, want empty string", dst.Name)
		}

		err := BindQuery(httptest.NewRequest(http.MethodGet, "/?enabled=", nil), &dst)
		_ = assertBadRequest(t, err)
	})

	t.Run("map string string target becomes single value snapshot", func(t *testing.T) {
		dst := map[string]string{"stale": "value"}

		if err := BindQuery(httptest.NewRequest(http.MethodGet, "/?name=kanata&page=2", nil), &dst); err != nil {
			t.Fatalf("BindQuery() error = %v", err)
		}
		if !reflect.DeepEqual(dst, map[string]string{"name": "kanata", "page": "2"}) {
			t.Fatalf("dst = %#v, want current single-value snapshot", dst)
		}
	})

	t.Run("empty query binds map target to usable empty map", func(t *testing.T) {
		var dst map[string]string

		if err := BindQuery(httptest.NewRequest(http.MethodGet, "/", nil), &dst); err != nil {
			t.Fatalf("BindQuery() error = %v", err)
		}
		if dst == nil || len(dst) != 0 {
			t.Fatalf("dst = %#v, want usable empty map", dst)
		}
	})

	t.Run("raw query parse failure is bad request and preserves target", func(t *testing.T) {
		type request struct {
			Name string `query:"name"`
		}

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.URL.RawQuery = "%"
		dst := request{Name: "existing"}

		err := BindQuery(req, &dst)
		_ = assertBadRequest(t, err)
		if dst != (request{Name: "existing"}) {
			t.Fatalf("dst = %#v, want unchanged", dst)
		}
	})

	t.Run("unsupported field families are usage errors", func(t *testing.T) {
		type unsupportedSlice struct {
			Tags bindQueryNamedSlice `query:"tag"`
		}
		type unsupportedTextDecoder struct {
			Value bindQueryTextValue `query:"value"`
		}

		assertNotHTTPError(t, BindQuery(httptest.NewRequest(http.MethodGet, "/?tag=a", nil), &unsupportedSlice{}))
		assertNotHTTPError(t, BindQuery(httptest.NewRequest(http.MethodGet, "/?value=x", nil), &unsupportedTextDecoder{}))
	})
}
