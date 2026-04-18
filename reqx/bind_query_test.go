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
	t.Run("binds supported leaf types", func(t *testing.T) {
		type request struct {
			Name    bindQueryNamedString `query:"name"`
			Enabled bool                 `query:"enabled"`
			Count   *int                 `query:"count"`
			When    time.Time            `query:"when"`
			Wait    time.Duration        `query:"wait"`
			Token   uuid.UUID            `query:"token"`
		}

		token := uuid.New()
		req := httptest.NewRequest(http.MethodGet, "/?name=kanata&enabled=true&count=7&when=2026-04-13T10:00:00Z&wait=5s&token="+token.String(), nil)

		var dst request
		if err := BindQuery(req, &dst); err != nil {
			t.Fatalf("BindQuery() error = %v", err)
		}
		if dst.Name != "kanata" || !dst.Enabled || dst.Count == nil || *dst.Count != 7 || dst.Wait != 5*time.Second || dst.Token != token {
			t.Fatalf("dst = %#v, want bound supported fields", dst)
		}
		if got := dst.When.UTC().Format(time.RFC3339); got != "2026-04-13T10:00:00Z" {
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
		_ = assertHTTPStatusCode(t, err, http.StatusBadRequest, "bad_request")
		if dst != (request{Page: 9}) {
			t.Fatalf("dst = %#v, want unchanged", dst)
		}
	})

	t.Run("unknown repeated key also returns bad request", func(t *testing.T) {
		type request struct {
			Name string `query:"name"`
		}

		dst := request{Name: "stale"}
		err := BindQuery(httptest.NewRequest(http.MethodGet, "/?extra=1&extra=2", nil), &dst)
		_ = assertHTTPStatusCode(t, err, http.StatusBadRequest, "bad_request")
		if dst != (request{Name: "stale"}) {
			t.Fatalf("dst = %#v, want unchanged", dst)
		}
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
		_ = assertHTTPStatusCode(t, err, http.StatusBadRequest, "bad_request")
	})

	t.Run("query values follow net url decoding", func(t *testing.T) {
		type request struct {
			Name string `query:"name"`
			Plus string `query:"plus"`
		}

		var dst request
		if err := BindQuery(httptest.NewRequest(http.MethodGet, "/?name=a+b&plus=%2B", nil), &dst); err != nil {
			t.Fatalf("BindQuery() error = %v", err)
		}
		if dst.Name != "a b" || dst.Plus != "+" {
			t.Fatalf("dst = %#v, want decoded query values", dst)
		}
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

	t.Run("map target rejects repeated key and preserves target", func(t *testing.T) {
		dst := map[string]string{"stale": "value"}

		err := BindQuery(httptest.NewRequest(http.MethodGet, "/?name=kanata&name=dup", nil), &dst)
		_ = assertHTTPStatusCode(t, err, http.StatusBadRequest, "bad_request")
		if !reflect.DeepEqual(dst, map[string]string{"stale": "value"}) {
			t.Fatalf("dst = %#v, want unchanged", dst)
		}
	})

	t.Run("map target raw query parse failure is bad request and preserves target", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.URL.RawQuery = "%"
		dst := map[string]string{"stale": "value"}

		err := BindQuery(req, &dst)
		_ = assertHTTPStatusCode(t, err, http.StatusBadRequest, "bad_request")
		if !reflect.DeepEqual(dst, map[string]string{"stale": "value"}) {
			t.Fatalf("dst = %#v, want unchanged", dst)
		}
	})

	t.Run("unsupported map target is usage error and preserves target", func(t *testing.T) {
		dst := map[string]int{"stale": 9}

		err := BindQuery(httptest.NewRequest(http.MethodGet, "/?page=2", nil), &dst)
		assertNotHTTPError(t, err)
		if !reflect.DeepEqual(dst, map[string]int{"stale": 9}) {
			t.Fatalf("dst = %#v, want unchanged", dst)
		}
	})

	t.Run("pointer leaf decode failure is bad request and preserves target", func(t *testing.T) {
		type request struct {
			Count *int `query:"count"`
		}

		existing := 9
		dst := request{Count: &existing}

		err := BindQuery(httptest.NewRequest(http.MethodGet, "/?count=bad", nil), &dst)
		_ = assertHTTPStatusCode(t, err, http.StatusBadRequest, "bad_request")
		if dst.Count == nil || *dst.Count != 9 {
			t.Fatalf("dst = %#v, want unchanged pointer field", dst)
		}
	})

	t.Run("pointer leaf success overwrites existing value", func(t *testing.T) {
		type request struct {
			Count *int `query:"count"`
		}

		existing := 1
		dst := request{Count: &existing}

		if err := BindQuery(httptest.NewRequest(http.MethodGet, "/?count=2", nil), &dst); err != nil {
			t.Fatalf("BindQuery() error = %v", err)
		}
		if dst.Count == nil || *dst.Count != 2 {
			t.Fatalf("dst = %#v, want overwritten pointer field", dst)
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
		_ = assertHTTPStatusCode(t, err, http.StatusBadRequest, "bad_request")
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

	t.Run("special query scalar formats reject invalid input and preserve target", func(t *testing.T) {
		t.Run("duration", func(t *testing.T) {
			type request struct {
				Wait time.Duration `query:"wait"`
			}

			dst := request{Wait: 3 * time.Second}
			err := BindQuery(httptest.NewRequest(http.MethodGet, "/?wait=forever", nil), &dst)
			_ = assertHTTPStatusCode(t, err, http.StatusBadRequest, "bad_request")
			if dst.Wait != 3*time.Second {
				t.Fatalf("dst = %#v, want unchanged", dst)
			}
		})

		t.Run("time", func(t *testing.T) {
			type request struct {
				When time.Time `query:"when"`
			}

			existing := time.Date(2026, time.April, 13, 10, 0, 0, 0, time.UTC)
			dst := request{When: existing}
			err := BindQuery(httptest.NewRequest(http.MethodGet, "/?when=not-rfc3339", nil), &dst)
			_ = assertHTTPStatusCode(t, err, http.StatusBadRequest, "bad_request")
			if !dst.When.Equal(existing) {
				t.Fatalf("dst = %#v, want unchanged", dst)
			}
		})

		t.Run("uuid", func(t *testing.T) {
			type request struct {
				Token uuid.UUID `query:"token"`
			}

			existing := uuid.New()
			dst := request{Token: existing}
			err := BindQuery(httptest.NewRequest(http.MethodGet, "/?token=not-a-uuid", nil), &dst)
			_ = assertHTTPStatusCode(t, err, http.StatusBadRequest, "bad_request")
			if dst.Token != existing {
				t.Fatalf("dst = %#v, want unchanged", dst)
			}
		})
	})

	t.Run("special pointer scalar formats bind successfully", func(t *testing.T) {
		type request struct {
			Wait  *time.Duration `query:"wait"`
			When  *time.Time     `query:"when"`
			Token *uuid.UUID     `query:"token"`
		}

		token := uuid.New()
		var dst request

		if err := BindQuery(httptest.NewRequest(http.MethodGet, "/?wait=5s&when=2026-04-13T10:00:00Z&token="+token.String(), nil), &dst); err != nil {
			t.Fatalf("BindQuery() error = %v", err)
		}
		if dst.Wait == nil || *dst.Wait != 5*time.Second {
			t.Fatalf("wait = %#v, want 5s", dst.Wait)
		}
		if dst.When == nil || dst.When.UTC().Format(time.RFC3339) != "2026-04-13T10:00:00Z" {
			t.Fatalf("when = %#v, want 2026-04-13T10:00:00Z", dst.When)
		}
		if dst.Token == nil || *dst.Token != token {
			t.Fatalf("token = %#v, want %v", dst.Token, token)
		}
	})

	t.Run("duration pointer uses parse duration semantics", func(t *testing.T) {
		type request struct {
			Wait *time.Duration `query:"wait"`
		}

		existing := 3 * time.Second
		dst := request{Wait: &existing}

		err := BindQuery(httptest.NewRequest(http.MethodGet, "/?wait=5", nil), &dst)
		_ = assertHTTPStatusCode(t, err, http.StatusBadRequest, "bad_request")
		if dst.Wait == nil || *dst.Wait != 3*time.Second {
			t.Fatalf("dst = %#v, want unchanged duration pointer", dst)
		}
	})

	t.Run("unsigned and float leaf types bind and reject invalid input", func(t *testing.T) {
		type request struct {
			Limit uint16  `query:"limit"`
			Ratio float32 `query:"ratio"`
		}

		t.Run("success", func(t *testing.T) {
			var dst request
			if err := BindQuery(httptest.NewRequest(http.MethodGet, "/?limit=7&ratio=1.25", nil), &dst); err != nil {
				t.Fatalf("BindQuery() error = %v", err)
			}
			if dst.Limit != 7 || dst.Ratio != 1.25 {
				t.Fatalf("dst = %#v, want bound unsigned and float values", dst)
			}
		})

		t.Run("invalid unsigned", func(t *testing.T) {
			dst := request{Limit: 9, Ratio: 1.25}
			err := BindQuery(httptest.NewRequest(http.MethodGet, "/?limit=-1", nil), &dst)
			_ = assertHTTPStatusCode(t, err, http.StatusBadRequest, "bad_request")
			if dst != (request{Limit: 9, Ratio: 1.25}) {
				t.Fatalf("dst = %#v, want unchanged", dst)
			}
		})

		t.Run("invalid float", func(t *testing.T) {
			dst := request{Limit: 9, Ratio: 1.25}
			err := BindQuery(httptest.NewRequest(http.MethodGet, "/?ratio=nan-ish", nil), &dst)
			_ = assertHTTPStatusCode(t, err, http.StatusBadRequest, "bad_request")
			if dst != (request{Limit: 9, Ratio: 1.25}) {
				t.Fatalf("dst = %#v, want unchanged", dst)
			}
		})
	})
}

func TestBindQuery_UsageAndPlanningContracts(t *testing.T) {
	t.Run("rejects invalid request and target shapes", func(t *testing.T) {
		type request struct {
			Name string `query:"name"`
		}

		assertNotHTTPError(t, BindQuery(nil, &request{}))
		assertNotHTTPError(t, BindQuery(httptest.NewRequest(http.MethodGet, "/", nil), nil))
		assertNotHTTPError(t, BindQuery(httptest.NewRequest(http.MethodGet, "/", nil), request{}))

		var typedNil *request
		assertNotHTTPError(t, BindQuery(httptest.NewRequest(http.MethodGet, "/", nil), typedNil))

		var unsupported []string
		assertNotHTTPError(t, BindQuery(httptest.NewRequest(http.MethodGet, "/", nil), &unsupported))
	})

	t.Run("usage errors win before raw query parsing", func(t *testing.T) {
		t.Run("unsupported target shape", func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.URL.RawQuery = "%"

			dst := map[string]int{"stale": 9}
			err := BindQuery(req, &dst)
			assertNotHTTPError(t, err)
			if !reflect.DeepEqual(dst, map[string]int{"stale": 9}) {
				t.Fatalf("dst = %#v, want unchanged", dst)
			}
		})

		t.Run("invalid tag", func(t *testing.T) {
			type request struct {
				Name string `query:""`
			}

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.URL.RawQuery = "%"

			dst := request{Name: "existing"}
			err := BindQuery(req, &dst)
			assertNotHTTPError(t, err)
			if dst != (request{Name: "existing"}) {
				t.Fatalf("dst = %#v, want unchanged", dst)
			}
		})

		t.Run("duplicate planned fields", func(t *testing.T) {
			type request struct {
				Name  string `query:"name"`
				Alias string `query:"name"`
			}

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.URL.RawQuery = "%"

			dst := request{Name: "existing", Alias: "keep"}
			err := BindQuery(req, &dst)
			assertNotHTTPError(t, err)
			if dst != (request{Name: "existing", Alias: "keep"}) {
				t.Fatalf("dst = %#v, want unchanged", dst)
			}
		})
	})

	t.Run("untagged and tagged unexported fields are ignored before validation", func(t *testing.T) {
		type request struct {
			Name     string              `query:"name"`
			hidden   bindQueryNamedSlice `query:"hidden"`
			badTag   string              `query:""`
			Untagged bindQueryNamedSlice
		}

		var dst request
		if err := BindQuery(httptest.NewRequest(http.MethodGet, "/?name=kanata&hidden=x&Untagged=y", nil), &dst); err != nil {
			t.Fatalf("BindQuery() error = %v", err)
		}
		if dst.Name != "kanata" || dst.hidden != nil || dst.badTag != "" || dst.Untagged != nil {
			t.Fatalf("dst = %#v, want only tagged exported fields to participate", dst)
		}
	})

	t.Run("nil url acts like empty query source", func(t *testing.T) {
		type request struct {
			Name string `query:"name"`
		}

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.URL = nil
		dst := request{Name: "stale"}

		if err := BindQuery(req, &dst); err != nil {
			t.Fatalf("BindQuery() error = %v", err)
		}
		if dst != (request{}) {
			t.Fatalf("dst = %#v, want zero value struct", dst)
		}
	})

	t.Run("unknown query keys are ignored", func(t *testing.T) {
		type request struct {
			Name string `query:"name"`
		}

		dst := request{Name: "stale"}
		if err := BindQuery(httptest.NewRequest(http.MethodGet, "/?extra=1", nil), &dst); err != nil {
			t.Fatalf("BindQuery() error = %v", err)
		}
		if dst != (request{}) {
			t.Fatalf("dst = %#v, want zero value struct", dst)
		}
	})

	t.Run("invalid tags are usage errors", func(t *testing.T) {
		type emptyTag struct {
			Name string `query:""`
		}
		type spacedTag struct {
			Name string `query:" name "`
		}
		type inlineTag struct {
			Filters struct{} `query:",inline"`
		}
		type recursiveInline struct {
			Self *recursiveInline `query:",inline"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?name=kanata", nil)

		assertNotHTTPError(t, BindQuery(req, &emptyTag{}))
		assertNotHTTPError(t, BindQuery(req, &spacedTag{}))
		assertNotHTTPError(t, BindQuery(req, &inlineTag{}))
		assertNotHTTPError(t, BindQuery(req, &recursiveInline{}))
	})

	t.Run("later client input errors do not commit earlier successful field writes", func(t *testing.T) {
		type request struct {
			Name  string `query:"name"`
			Count int    `query:"count"`
		}

		dst := request{Name: "existing", Count: 9}
		err := BindQuery(httptest.NewRequest(http.MethodGet, "/?name=kanata&count=bad", nil), &dst)
		_ = assertHTTPStatusCode(t, err, http.StatusBadRequest, "bad_request")
		if dst != (request{Name: "existing", Count: 9}) {
			t.Fatalf("dst = %#v, want unchanged", dst)
		}
	})

	t.Run("double pointer leaf is usage error", func(t *testing.T) {
		type request struct {
			Count **int `query:"count"`
		}

		assertNotHTTPError(t, BindQuery(httptest.NewRequest(http.MethodGet, "/?count=7", nil), &request{}))
	})

	t.Run("query dash fields are ignored before type validation", func(t *testing.T) {
		type request struct {
			Name    string              `query:"name"`
			Ignored bindQueryNamedSlice `query:"-"`
		}

		var dst request
		if err := BindQuery(httptest.NewRequest(http.MethodGet, "/?name=kanata", nil), &dst); err != nil {
			t.Fatalf("BindQuery() error = %v", err)
		}
		if dst.Name != "kanata" || dst.Ignored != nil {
			t.Fatalf("dst = %#v, want ignored field skipped", dst)
		}
	})

	t.Run("query dash fields do not participate in duplicate detection", func(t *testing.T) {
		type request struct {
			Name    string `query:"name"`
			Ignored string `query:"-"`
			Alias   string `query:"alias"`
		}

		var dst request
		if err := BindQuery(httptest.NewRequest(http.MethodGet, "/?name=kanata&alias=friend", nil), &dst); err != nil {
			t.Fatalf("BindQuery() error = %v", err)
		}
		if dst != (request{Name: "kanata", Alias: "friend"}) {
			t.Fatalf("dst = %#v, want bound non-dash fields only", dst)
		}
	})

	t.Run("query keys match parsed names exactly", func(t *testing.T) {
		type request struct {
			Upper string `query:"Name"`
			Lower string `query:"name"`
			Plus  string `query:"x+y"`
		}

		req := httptest.NewRequest(http.MethodGet, "/?Name=upper&name=lower&x+y=space&x%2By=plus", nil)

		var dst request
		if err := BindQuery(req, &dst); err != nil {
			t.Fatalf("BindQuery() error = %v", err)
		}
		if dst.Upper != "upper" || dst.Lower != "lower" || dst.Plus != "plus" {
			t.Fatalf("dst = %#v, want exact parsed-key matches only", dst)
		}
	})

	t.Run("duplicate planned fields fail before any write", func(t *testing.T) {
		type request struct {
			Name  string `query:"name"`
			Alias string `query:"name"`
		}

		dst := request{Name: "existing", Alias: "keep"}
		err := BindQuery(httptest.NewRequest(http.MethodGet, "/?name=kanata", nil), &dst)
		assertNotHTTPError(t, err)
		if dst != (request{Name: "existing", Alias: "keep"}) {
			t.Fatalf("dst = %#v, want unchanged", dst)
		}
	})

}
