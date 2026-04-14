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
		{name: "BindBody", call: BindBody},
		{name: "BindQuery", call: BindQuery},
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

func TestBindQuery_NoopsOnUnsupportedTargets(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?page=1", nil)

	scalar := 1
	if err := BindQuery(req, &scalar); err != nil {
		t.Fatalf("BindQuery(scalar) error = %v", err)
	}

	unsupportedMap := map[string]int(nil)
	if err := BindQuery(req, &unsupportedMap); err != nil {
		t.Fatalf("BindQuery(map[string]int) error = %v", err)
	}
	if unsupportedMap != nil {
		t.Fatalf("unsupportedMap = %#v, want nil no-op", unsupportedMap)
	}
}

func TestBindQuery_UsesOnlyQuerySource(t *testing.T) {
	type request struct {
		AccountID string `param:"account_id"`
		Actor     string `header:"x-actor"`
		Name      string `json:"name"`
		Cursor    string `query:"cursor"`
	}

	req := httptest.NewRequest(http.MethodPost, "/?cursor=next", nil)
	req.Header.Set("X-Actor", "kanata")
	req.SetPathValue("account_id", "acct_123")
	req.Pattern = "/accounts/{account_id}"
	setRequestBody(req, mimeApplicationJSON, `{"name":"body-name"}`)

	dst := request{
		AccountID: "existing-account",
		Actor:     "existing-actor",
		Name:      "existing-name",
	}
	if err := BindQuery(req, &dst); err != nil {
		t.Fatalf("BindQuery() error = %v", err)
	}
	if dst.AccountID != "existing-account" || dst.Actor != "existing-actor" || dst.Name != "existing-name" || dst.Cursor != "next" {
		t.Fatalf("dst = %#v, want query field only", dst)
	}
}
