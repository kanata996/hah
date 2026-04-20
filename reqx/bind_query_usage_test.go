package reqx

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

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

	t.Run("successful bind preserves non planned fields", func(t *testing.T) {
		type request struct {
			Name     string `query:"name"`
			Ignored  string `query:"-"`
			Untagged string
		}

		dst := request{
			Name:     "stale",
			Ignored:  "keep-ignored",
			Untagged: "keep-untagged",
		}
		if err := BindQuery(httptest.NewRequest(http.MethodGet, "/", nil), &dst); err != nil {
			t.Fatalf("BindQuery() error = %v", err)
		}
		if dst != (request{Ignored: "keep-ignored", Untagged: "keep-untagged"}) {
			t.Fatalf("dst = %#v, want planned fields reset and non planned fields preserved", dst)
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

	t.Run("pointer to unsupported leaf is usage error", func(t *testing.T) {
		type request struct {
			Value *bindQueryTextValue `query:"value"`
		}

		assertNotHTTPError(t, BindQuery(httptest.NewRequest(http.MethodGet, "/?value=x", nil), &request{}))
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
