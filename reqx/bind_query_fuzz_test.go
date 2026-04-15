package reqx

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

type bindQueryFuzzRequest struct {
	Name      string   `query:"name"`
	Page      int      `query:"page"`
	Tags      []string `query:"tag"`
	AccountID string   `param:"account_id"`
	Actor     string   `header:"x-actor"`
	BodyName  string   `json:"name"`
}

func FuzzBindQueryPublicContracts(f *testing.F) {
	f.Add("/items?page=1")
	f.Add("/items?name=kanata&tag=a&tag=b")
	f.Add("/items?when=2026-04-09")
	f.Add("/items?page=oops")
	f.Add("/items?PAGE=7&tag=&tag=b")

	f.Fuzz(func(t *testing.T, target string) {
		req, err := http.NewRequest(http.MethodGet, strings.TrimSpace(target), nil)
		if err != nil {
			req = httptest.NewRequest(http.MethodGet, "/", nil)
		}

		req.Header.Set("X-Actor", "header-actor")
		req.SetPathValue("account_id", "acct_from_path")
		req.Pattern = "/accounts/{account_id}"
		setRequestBody(req, mimeApplicationJSON, `{"name":"body-name"}`)

		got := bindQueryFuzzRequest{
			Name:      "existing-name",
			Page:      7,
			Tags:      []string{"existing-tag"},
			AccountID: "existing-account",
			Actor:     "existing-actor",
			BodyName:  "existing-body",
		}
		want, wantBadRequest := expectedBindQueryOutcome(req, got)

		gotErr := BindQuery(req, &got)
		if wantBadRequest {
			_ = assertBadRequest(t, gotErr)
		} else if gotErr != nil {
			t.Fatalf("BindQuery() error = %v, want nil", gotErr)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("BindQuery() result = %#v, want %#v", got, want)
		}
	})
}

func expectedBindQueryOutcome(req *http.Request, seed bindQueryFuzzRequest) (bindQueryFuzzRequest, bool) {
	want := seed
	want.Tags = append([]string(nil), seed.Tags...)

	if req.URL == nil {
		return want, false
	}

	values := req.URL.Query()
	if fieldValues, ok := values["name"]; ok && len(fieldValues) > 0 {
		want.Name = fieldValues[0]
	}

	if fieldValues, ok := values["page"]; ok && len(fieldValues) > 0 {
		page := fieldValues[0]
		if page == "" {
			page = "0"
		}
		parsed, err := strconv.ParseInt(page, 10, 0)
		if err != nil {
			return want, true
		}
		want.Page = int(parsed)
	}

	if fieldValues, ok := values["tag"]; ok && len(fieldValues) > 0 {
		want.Tags = append([]string(nil), fieldValues...)
	}

	return want, false
}
