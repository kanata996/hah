package reqx

// 测试清单：
// - 标记说明：[✓] 已核对且已有真实覆盖；[x] 尚未完成，不得作为验收依据。
// - [✓] `Bind` 的阶段顺序、覆盖优先级、按 HTTP 方法启用/跳过规则与阶段错误透传。
// - [✓] `Bind` 在空 body 时会把 body 阶段视为 no-op，并忽略该场景的 Content-Type。
// - [✓] `BindHeaders` 会规范化请求头键名，并拒绝重复的标量 header。
// - [✓] `BindAndValidate` 在请求级校验错误中使用请求标签字段名。
// - [✓] `Bind` 与 path/query/body 单源 API 在代表性成功与阶段失败场景下保持一致。

import (
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// 顶层 Bind 只负责来源顺序和按方法启用/跳过各阶段。
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
		req.Header.Set("Content-Type", "application/json")
		req.Body = io.NopCloser(strings.NewReader(`{"id":"body-id","name":"body-name"}`))

		var bound request
		if err := Bind(req, &bound); err != nil {
			t.Fatalf("Bind() error = %v", err)
		}
		if bound.ID != "body-id" || bound.Name != "body-name" {
			t.Fatalf("bound = %#v, want body values to win", bound)
		}
	})

	t.Run("delete binds query over path", func(t *testing.T) {
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
			t.Fatalf("Bind() id = %q", bound.ID)
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
		req.Header.Set("Content-Type", "application/json")
		req.Body = io.NopCloser(strings.NewReader(`{"id":"body-id"}`))

		var bound request
		if err := Bind(req, &bound); err != nil {
			t.Fatalf("Bind() error = %v", err)
		}
		if bound.ID != "body-id" || bound.Scope != "" {
			t.Fatalf("bound = %#v, want body id and skipped query scope", bound)
		}
	})
}

// 顶层 Bind 对 path/query/body 的公开契约应与对应单源 API 保持一致。
func TestBind_MatchesSingleSourceAPIs(t *testing.T) {
	t.Run("successful get binding matches ordered single-source composition", func(t *testing.T) {
		type request struct {
			ID   string `param:"id" query:"id" json:"id"`
			Name string `query:"name" json:"name"`
			Age  int    `json:"age"`
		}

		newReq := func() *http.Request {
			req := requestWithPathParams(map[string][]string{
				"id": {"route-id"},
			})
			req.Method = http.MethodGet
			req.URL.RawQuery = "id=query-id&name=query-name"
			req.Header.Set("Content-Type", "application/json")
			req.Body = io.NopCloser(strings.NewReader(`{"id":"body-id","name":"body-name","age":17}`))
			return req
		}

		initial := request{
			ID:   "existing-id",
			Name: "existing-name",
			Age:  7,
		}

		got := initial
		if err := Bind(newReq(), &got); err != nil {
			t.Fatalf("Bind() error = %v", err)
		}

		want := initial
		req := newReq()
		if err := BindPathValues(req, &want); err != nil {
			t.Fatalf("BindPathValues() error = %v", err)
		}
		if err := BindQueryParams(req, &want); err != nil {
			t.Fatalf("BindQueryParams() error = %v", err)
		}
		if err := BindBody(req, &want); err != nil {
			t.Fatalf("BindBody() error = %v", err)
		}

		if !reflect.DeepEqual(got, want) {
			t.Fatalf("Bind() result = %#v, want %#v", got, want)
		}
	})

	t.Run("path stage error matches path binding", func(t *testing.T) {
		type request struct {
			ID   int    `param:"id"`
			Name string `query:"name"`
		}

		newReq := func() *http.Request {
			req := requestWithPathParams(map[string][]string{
				"id": {"oops"},
			})
			req.Method = http.MethodGet
			req.URL.RawQuery = "name=kanata"
			return req
		}

		got := request{ID: 7, Name: "existing"}
		gotErr := Bind(newReq(), &got)

		var want request
		wantErr := BindPathValues(newReq(), &want)
		_ = assertSameHTTPError(t, gotErr, wantErr)

		if got.ID != 7 || got.Name != "existing" {
			t.Fatalf("dst = %#v, want destination preserved on path failure", got)
		}
	})

	t.Run("query stage error matches query binding", func(t *testing.T) {
		type request struct {
			ID   string `param:"id"`
			Page int    `query:"page"`
		}

		newReq := func() *http.Request {
			req := requestWithPathParams(map[string][]string{
				"id": {"route-id"},
			})
			req.Method = http.MethodGet
			req.URL.RawQuery = "page=oops"
			return req
		}

		got := request{ID: "existing-id", Page: 3}
		gotErr := Bind(newReq(), &got)

		var want request
		wantErr := BindQueryParams(newReq(), &want)
		_ = assertSameHTTPError(t, gotErr, wantErr)

		if got.ID != "existing-id" || got.Page != 3 {
			t.Fatalf("dst = %#v, want destination preserved on query failure", got)
		}
	})

	t.Run("body stage error matches body binding", func(t *testing.T) {
		type request struct {
			ID   string `param:"id" json:"id"`
			Age  int    `json:"age"`
			Name string `json:"name"`
		}

		newReq := func() *http.Request {
			req := requestWithPathParams(map[string][]string{
				"id": {"route-id"},
			})
			req.Method = http.MethodPost
			req.Header.Set("Content-Type", "application/json")
			req.Body = io.NopCloser(strings.NewReader(`{"name":"kanata","age":"oops"}`))
			return req
		}

		got := request{ID: "existing-id", Age: 3, Name: "existing-name"}
		gotErr := Bind(newReq(), &got)

		var want request
		wantErr := BindBody(newReq(), &want)
		_ = assertSameHTTPError(t, gotErr, wantErr)

		if got.ID != "existing-id" || got.Age != 3 || got.Name != "existing-name" {
			t.Fatalf("dst = %#v, want destination preserved on body failure", got)
		}
	})
}

// 顶层 Bind 在空 body 场景下会把 body 阶段视为 no-op，因此不会校验该阶段的 Content-Type。
func TestBind_IgnoresEmptyBodyContentType(t *testing.T) {
	type request struct {
		Page int `query:"page"`
	}

	req := httptest.NewRequest(http.MethodGet, "/items?page=2", nil)
	req.Header.Set("Content-Type", "text/plain")

	var bound request
	if err := Bind(req, &bound); err != nil {
		t.Fatalf("Bind() error = %v", err)
	}
	if bound.Page != 2 {
		t.Fatalf("Bind() page = %d", bound.Page)
	}
}

// 公开 Bind 入口会直接拒绝空输入参数。
func TestBind_RejectsNilInputs(t *testing.T) {
	t.Run("nil request", func(t *testing.T) {
		var dst struct{}
		err := Bind[struct{}](nil, &dst)
		if err == nil {
			t.Fatal("Bind() error = nil")
		}
		if got := err.Error(); got != "reqx: request must not be nil" {
			t.Fatalf("error = %q", got)
		}
	})

	t.Run("nil destination", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		err := Bind[struct{}](req, nil)
		if err == nil {
			t.Fatal("Bind() error = nil")
		}
		if got := err.Error(); got != "reqx: destination must not be nil" {
			t.Fatalf("error = %q", got)
		}
	})
}

// Header 绑定会兼容规范和非规范请求头键名。
func TestBindHeaders_NormalizesHeaderKeys(t *testing.T) {
	type request struct {
		RequestID string `header:"x-request-id"`
	}

	testCases := []struct {
		name   string
		header http.Header
	}{
		{
			name: "canonical key",
			header: http.Header{
				"X-Request-Id": {"req-123"},
			},
		},
		{
			name: "non canonical key",
			header: http.Header{
				" x-request-id ": {"req-123"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/items", nil)
			req.Header = tc.header

			var bound request
			if err := BindHeaders(req, &bound); err != nil {
				t.Fatalf("BindHeaders() error = %v", err)
			}
			if bound.RequestID != "req-123" {
				t.Fatalf("BindHeaders() request_id = %q", bound.RequestID)
			}
		})
	}
}

// Header 标量字段重复出现时返回重复值错误。
func TestBindHeaders_RejectsRepeatedScalar(t *testing.T) {
	type request struct {
		RequestID string `header:"x-request-id"`
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Add("X-Request-Id", "req-1")
	req.Header.Add("X-Request-Id", "req-2")

	var dst request
	violation := assertSingleViolation(t, BindHeaders(req, &dst))
	if violation.Field != "X-Request-Id" || violation.In != ViolationInHeader || violation.Code != ViolationCodeMultiple || violation.Detail != "must not be repeated" {
		t.Fatalf("violation = %#v", violation)
	}
}

// 请求级校验错误优先使用请求标签中的字段名。
func TestBindAndValidate_UsesRequestTagNames(t *testing.T) {
	type request struct {
		UUID string `param:"uuid" validate:"required"`
	}

	req := httptest.NewRequest(http.MethodGet, "/items", nil)

	var bound request
	violation := assertSingleViolation(t, BindAndValidate(req, &bound))
	if violation.Field != "uuid" || violation.Code != ViolationCodeRequired {
		t.Fatalf("BindAndValidate() violation = %#v", violation)
	}
}

// 请求级校验在嵌套 body 字段上也应继承父级 json 来源，而不是退回 request。
func TestBindAndValidate_InheritsNestedBodyInputSource(t *testing.T) {
	type profile struct {
		Name string `validate:"required"`
	}
	type request struct {
		Profile profile `json:"profile"`
	}

	req := newJSONRequest(http.MethodPost, "/", `{"profile":{}}`)

	var bound request
	violation := assertSingleViolation(t, BindAndValidate(req, &bound))
	if violation.Field != "profile.Name" || violation.In != ViolationInBody || violation.Code != ViolationCodeRequired || violation.Detail != "is required" {
		t.Fatalf("BindAndValidate() violation = %#v", violation)
	}
}

// 顶层 Bind 会直接返回各阶段产生的错误，而不会吞掉或改写来源语义。
func TestBind_PropagatesStageErrors(t *testing.T) {
	t.Run("query binding error on get", func(t *testing.T) {
		type request struct {
			Page int `query:"page"`
		}

		req := httptest.NewRequest(http.MethodGet, "/items?page=oops", nil)

		var bound request
		violation := assertSingleViolation(t, Bind(req, &bound))
		if violation.Field != "page" || violation.Code != ViolationCodeType || violation.Detail != "must be number" {
			t.Fatalf("violation = %#v", violation)
		}
	})

	t.Run("unsupported body content type when body present", func(t *testing.T) {
		type request struct {
			Name string `json:"name"`
		}

		req := httptest.NewRequest(http.MethodPost, "/items", strings.NewReader(`{"name":"kanata"}`))
		req.Header.Set("Content-Type", "text/plain")

		var bound request
		err := Bind(req, &bound)
		_ = assertHTTPError(t, err, http.StatusUnsupportedMediaType, CodeUnsupportedMediaType, "Content-Type must be application/json or application/*+json")
	})
}

// 请求级包装器在绑定前就会拒绝空输入参数。
func TestBindAndValidate_RejectsNilInputs(t *testing.T) {
	t.Run("nil request", func(t *testing.T) {
		var dst struct{}
		err := BindAndValidate[struct{}](nil, &dst)
		if err == nil {
			t.Fatal("BindAndValidate() error = nil")
		}
		if got := err.Error(); got != "reqx: request must not be nil" {
			t.Fatalf("error = %q", got)
		}
	})

	t.Run("nil destination", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		err := BindAndValidate[struct{}](req, nil)
		if err == nil {
			t.Fatal("BindAndValidate() error = nil")
		}
		if got := err.Error(); got != "reqx: destination must not be nil" {
			t.Fatalf("error = %q", got)
		}
	})
}
