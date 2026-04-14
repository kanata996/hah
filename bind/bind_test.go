package bind

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBind_PublicEntryPointsRejectInvalidInputs(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	type destination struct{}

	var typedNil *destination

	entryPoints := []struct {
		name string
		call func(*http.Request, any) error
	}{
		{name: "Bind", call: Bind},
		{name: "BindBody", call: BindBody},
		{name: "BindQueryParams", call: BindQueryParams},
		{name: "BindPathValues", call: BindPathValues},
		{name: "BindHeaders", call: BindHeaders},
	}

	invalidInputs := []struct {
		name   string
		req    *http.Request
		target any
		want   string
	}{
		{name: "rejects nil request", req: nil, target: &destination{}, want: wantNilRequestErr},
		{name: "rejects nil destination", req: req, target: nil, want: wantNilDestinationErr},
		{name: "rejects non-pointer destination", req: req, target: destination{}, want: wantNilDestinationErr},
		{name: "rejects typed nil destination", req: req, target: typedNil, want: wantNilDestinationErr},
	}

	for _, entryPoint := range entryPoints {
		t.Run(entryPoint.name, func(t *testing.T) {
			for _, tc := range invalidInputs {
				t.Run(tc.name, func(t *testing.T) {
					assertUsageError(t, entryPoint.call(tc.req, tc.target), tc.want)
				})
			}
		})
	}
}

func TestBind_SingleSourcePublicAPIsMatchBindNoopSemanticsForUnsupportedTargets(t *testing.T) {
	queryReq := httptest.NewRequest(http.MethodGet, "/?page=1", nil)
	pathReq := requestWithPathParams(map[string][]string{"id": {"1"}})
	headerReq := httptest.NewRequest(http.MethodGet, "/", nil)
	headerReq.Header.Set("X-Request-Id", "req-1")

	scalar := 1
	if err := BindQueryParams(queryReq, &scalar); err != nil {
		t.Fatalf("BindQueryParams(scalar) error = %v", err)
	}
	if err := BindPathValues(pathReq, &scalar); err != nil {
		t.Fatalf("BindPathValues(scalar) error = %v", err)
	}
	if err := BindHeaders(headerReq, &scalar); err != nil {
		t.Fatalf("BindHeaders(scalar) error = %v", err)
	}
	if err := Bind(queryReq, &scalar); err != nil {
		t.Fatalf("Bind(scalar) error = %v", err)
	}

	unsupportedMap := map[string]int(nil)
	if err := BindQueryParams(queryReq, &unsupportedMap); err != nil {
		t.Fatalf("BindQueryParams(map[string]int) error = %v", err)
	}
	if err := Bind(queryReq, &unsupportedMap); err != nil {
		t.Fatalf("Bind(map[string]int) error = %v", err)
	}
	if unsupportedMap != nil {
		t.Fatalf("unsupportedMap = %#v, want nil no-op", unsupportedMap)
	}
}

func TestBind_StageOrderAndMethodRules(t *testing.T) {
	t.Run("get applies path query then body", func(t *testing.T) {
		type request struct {
			ID   string `param:"id" query:"id" json:"id"`
			Name string `query:"name" json:"name"`
		}

		req := requestWithPathParams(map[string][]string{
			"id": {"route-id"},
		})
		req.Method = http.MethodGet
		req.URL.RawQuery = "id=query-id&name=query-name"
		setRequestBody(req, mimeApplicationJSON, `{"id":"body-id","name":"body-name"}`)

		var bound request
		if err := Bind(req, &bound); err != nil {
			t.Fatalf("Bind() error = %v", err)
		}
		if bound.ID != "body-id" || bound.Name != "body-name" {
			t.Fatalf("bound = %#v, want body values to win", bound)
		}
	})

	t.Run("delete binds query over path when body is absent", func(t *testing.T) {
		type request struct {
			ID string `param:"id" query:"id"`
		}

		req := requestWithPathParams(map[string][]string{
			"id": {"route-id"},
		})
		req.Method = http.MethodDelete
		req.URL.RawQuery = "id=query-id"

		var bound request
		if err := Bind(req, &bound); err != nil {
			t.Fatalf("Bind() error = %v", err)
		}
		if bound.ID != "query-id" {
			t.Fatalf("Bind() id = %q, want query-id", bound.ID)
		}
	})

	t.Run("head also binds query", func(t *testing.T) {
		type request struct {
			ID string `param:"id" query:"id"`
		}

		req := requestWithPathParams(map[string][]string{
			"id": {"route-id"},
		})
		req.Method = http.MethodHead
		req.URL.RawQuery = "id=query-id"

		var bound request
		if err := Bind(req, &bound); err != nil {
			t.Fatalf("Bind() error = %v", err)
		}
		if bound.ID != "query-id" {
			t.Fatalf("Bind() id = %q, want query-id", bound.ID)
		}
	})

	t.Run("post skips query but still binds body", func(t *testing.T) {
		type request struct {
			ID    string `param:"id" json:"id"`
			Scope string `query:"scope"`
		}

		req := requestWithPathParams(map[string][]string{
			"id": {"route-id"},
		})
		req.Method = http.MethodPost
		req.URL.RawQuery = "scope=query-scope"
		setRequestBody(req, mimeApplicationJSON, `{"id":"body-id"}`)

		var bound request
		if err := Bind(req, &bound); err != nil {
			t.Fatalf("Bind() error = %v", err)
		}
		if bound.ID != "body-id" || bound.Scope != "" {
			t.Fatalf("bound = %#v, want body id and skipped query scope", bound)
		}
	})
}

func TestBind_DoesNotUseHeadersByDefault(t *testing.T) {
	type request struct {
		RequestID string `header:"x-request-id"`
	}

	req := newJSONRequest(http.MethodGet, "/", "")
	req.Header.Set("X-Request-Id", "req-123")
	req.ContentLength = 0

	var bound request
	if err := Bind(req, &bound); err != nil {
		t.Fatalf("Bind() error = %v", err)
	}
	if bound.RequestID != "" {
		t.Fatalf("request_id = %q, want empty", bound.RequestID)
	}
}

func TestBind_EmptyBodyNoopPreservesEarlierStageWrites(t *testing.T) {
	type request struct {
		ID   string `param:"id"`
		Page int    `query:"page"`
		Name string `json:"name"`
	}

	req := requestWithPathParams(map[string][]string{
		"id": {"route-id"},
	})
	req.Method = http.MethodGet
	req.URL.RawQuery = "page=2"
	req.Header.Set("Content-Type", "text/plain")
	setRequestBody(req, "text/plain", "")

	dst := request{Name: "existing-name"}
	if err := Bind(req, &dst); err != nil {
		t.Fatalf("Bind() error = %v", err)
	}
	if dst.ID != "route-id" || dst.Page != 2 || dst.Name != "existing-name" {
		t.Fatalf("dst = %#v, want path/query updates and body no-op", dst)
	}

	reqUnknownLength := requestWithPathParams(map[string][]string{
		"id": {"route-id"},
	})
	reqUnknownLength.Method = http.MethodGet
	reqUnknownLength.URL.RawQuery = "page=2"
	setRequestBody(reqUnknownLength, "text/plain", "")
	reqUnknownLength.ContentLength = -1

	dst = request{Name: "existing-name"}
	if err := Bind(reqUnknownLength, &dst); err != nil {
		t.Fatalf("Bind(unknown-length empty body) error = %v", err)
	}
	if dst.ID != "route-id" || dst.Page != 2 || dst.Name != "existing-name" {
		t.Fatalf("dst = %#v, want path/query updates and empty body no-op", dst)
	}
}

func TestBind_PartialUpdatesPersistAcrossStageFailure(t *testing.T) {
	t.Run("path update remains when query fails", func(t *testing.T) {
		type request struct {
			ID   string `param:"id"`
			Page int    `query:"page"`
		}

		req := requestWithPathParams(map[string][]string{
			"id": {"route-id"},
		})
		req.Method = http.MethodGet
		req.URL.RawQuery = "page=oops"

		dst := request{ID: "existing-id", Page: 3}
		err := Bind(req, &dst)
		_ = assertBadRequest(t, err)
		if dst.ID != "route-id" || dst.Page != 3 {
			t.Fatalf("dst = %#v, want path update preserved before query failure", dst)
		}
	})

	t.Run("query update remains when body fails", func(t *testing.T) {
		type request struct {
			ID   string `param:"id"`
			Page int    `query:"page"`
			Age  int    `json:"age"`
		}

		req := requestWithPathParams(map[string][]string{
			"id": {"route-id"},
		})
		req.Method = http.MethodGet
		req.URL.RawQuery = "page=7"
		setRequestBody(req, mimeApplicationJSON, `{"age":"oops"}`)

		dst := request{ID: "existing-id", Page: 3, Age: 1}
		err := Bind(req, &dst)
		_ = assertHTTPError(t, err, http.StatusBadRequest, CodeInvalidJSON, "request body must be valid JSON")
		if dst.ID != "route-id" || dst.Page != 7 || dst.Age != 1 {
			t.Fatalf("dst = %#v, want path/query updates preserved before body failure", dst)
		}
	})
}
