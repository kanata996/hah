package hah

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kanata996/hah/errx"
	"github.com/kanata996/hah/reqx"
)

// 测试清单：
// [✓] 根包 facade 会把 bind / reqx / resp 的核心能力稳定透传出来
// [✓] 根包 facade 会把 resp 的成功响应与错误响应 helper 稳定透传出来
// [✓] 根包 facade 只暴露当前主路径 API：Bind、BindBody、Path、Query、PathParam、QueryParam、BindAndValidate、RequireBody 与响应入口
// [✓] README 中承诺的 create account handler 主路径有根包级端到端测试支撑

type rootPayloadMap map[string]any

// Bind 会通过根包 facade 复用 bind 包的默认绑定顺序。
func TestBind_DelegatesToBind(t *testing.T) {
	type request struct {
		ID   string `param:"id" query:"id" json:"id"`
		Name string `json:"name"`
	}

	req := newRouteRequest(http.MethodGet, "/accounts?id=query-id", "id", "route-id")

	var dst request
	if err := Bind(req, &dst); err != nil {
		t.Fatalf("Bind() error = %v", err)
	}
	if dst.ID != "query-id" {
		t.Fatalf("id = %q, want query-id", dst.ID)
	}
}

// BindBody 只从 JSON body 绑定数据。
func TestBindBody_DelegatesToBind(t *testing.T) {
	req := newJSONRequest(http.MethodPost, "/accounts", `{"name":"kanata"}`)

	var dst struct {
		Name string `json:"name"`
	}

	if err := BindBody(req, &dst); err != nil {
		t.Fatalf("BindBody() error = %v", err)
	}
	if dst.Name != "kanata" {
		t.Fatalf("name = %q, want kanata", dst.Name)
	}
}

// PathParam 会通过根包 facade 暴露 reqx 的 typed path getter。
func TestPathParam_DelegatesToReqx(t *testing.T) {
	req := newRouteRequest(http.MethodGet, "/accounts/42", "id", "42")

	got, err := PathParam[int](req, "id")
	if err != nil {
		t.Fatalf("PathParam() error = %v", err)
	}
	if got != 42 {
		t.Fatalf("PathParam() = %d, want 42", got)
	}
}

// QueryParam 会通过根包 facade 暴露 reqx 的 typed query getter。
func TestQueryParam_DelegatesToReqx(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/accounts?page=42", nil)

	got, err := QueryParam[int](req, "page")
	if err != nil {
		t.Fatalf("QueryParam() error = %v", err)
	}
	if got != 42 {
		t.Fatalf("QueryParam() = %d, want 42", got)
	}
}

// Path 会通过根包 facade 暴露 reqx 的 path 单参数校验 builder。
func TestPath_DelegatesToReqx(t *testing.T) {
	want := uuid.New()
	req := newRouteRequest(http.MethodGet, "/accounts/"+want.String(), "id", want.String())

	id, err := Path(req, "id").UUID().Required().Get()
	if err != nil {
		t.Fatalf("Path().UUID().Required().Get() error = %v", err)
	}
	if id != want {
		t.Fatalf("id = %v, want %v", id, want)
	}
}

// Query 会通过根包 facade 暴露 reqx 的 query 单参数校验 builder。
func TestQuery_DelegatesToReqx(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/accounts?at=2026-04-12T08:30:00Z", nil)

	at, err := Query(req, "at").Time().Required().Get()
	if err != nil {
		t.Fatalf("Query().Time().Required().Get() error = %v", err)
	}
	if got := at.UTC().Format(time.RFC3339); got != "2026-04-12T08:30:00Z" {
		t.Fatalf("at = %q, want 2026-04-12T08:30:00Z", got)
	}
}

// BindAndValidate 会在根包 facade 中同时复用绑定和校验能力。
func TestBindAndValidate_DelegatesToReqx(t *testing.T) {
	type request struct {
		OrgID string `param:"org_id" validate:"required"`
		Name  string `json:"name" validate:"required"`
	}

	req := newRouteJSONRequest(http.MethodPost, "/orgs/o_1/accounts", `{"name":"kanata"}`, "org_id", "o_1")

	var dst request
	if err := BindAndValidate(req, &dst); err != nil {
		t.Fatalf("BindAndValidate() error = %v", err)
	}
	if dst.OrgID != "o_1" {
		t.Fatalf("org_id = %q, want o_1", dst.OrgID)
	}
	if dst.Name != "kanata" {
		t.Fatalf("name = %q, want kanata", dst.Name)
	}
}

// RequireBody 会通过根包 facade 暴露 reqx 的 body-required 规则 helper。
func TestRequireBody_DelegatesToReqx(t *testing.T) {
	req := newJSONRequest(http.MethodPost, "/accounts", "")
	req.ContentLength = -1

	violation := assertSingleRootViolation(t, RequireBody(req))
	if violation.Field != "body" || violation.In != reqx.ViolationInBody || violation.Code != reqx.ViolationCodeRequired || violation.Detail != "is required" {
		t.Fatalf("violation = %#v", violation)
	}
}

// WriteError 会通过根包 facade 写出统一的公开错误包络。
func TestWriteError_DelegatesToResp(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/accounts", nil)
	rr := httptest.NewRecorder()

	if err := WriteError(rr, req, context.DeadlineExceeded); err != nil {
		t.Fatalf("WriteError() error = %v", err)
	}
	if rr.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusGatewayTimeout)
	}

	body := decodeRootPayload(t, rr.Body.Bytes())
	if got := body["code"]; got != "timeout" {
		t.Fatalf("code = %#v, want timeout", got)
	}
	if got := body["title"]; got != http.StatusText(http.StatusGatewayTimeout) {
		t.Fatalf("title = %#v, want %q", got, http.StatusText(http.StatusGatewayTimeout))
	}
}

// NewErrorResponder 会通过根包 facade 公开 resp 的可定制错误响应器。
func TestNewErrorResponder_DelegatesToResp(t *testing.T) {
	responder := NewErrorResponder()
	if responder == nil {
		t.Fatal("NewErrorResponder() = nil")
	}

	req := httptest.NewRequest(http.MethodGet, "/accounts", nil)
	rr := httptest.NewRecorder()

	if err := responder.Respond(rr, req, errx.NewHTTPError(http.StatusBadRequest, "bad_request", "bad request")); err != nil {
		t.Fatalf("Respond() error = %v", err)
	}
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}

	body := decodeRootPayload(t, rr.Body.Bytes())
	if got := body["code"]; got != "bad_request" {
		t.Fatalf("code = %#v, want bad_request", got)
	}
	if got := body["detail"]; got != "bad request" {
		t.Fatalf("detail = %#v, want bad request", got)
	}
}

// OK 会通过根包 facade 写回标准 200 JSON 响应。
func TestOK_DelegatesToResp(t *testing.T) {
	rr := httptest.NewRecorder()

	if err := OK(rr, map[string]any{"id": "u_1"}); err != nil {
		t.Fatalf("OK() error = %v", err)
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	payload := decodeRootPayload(t, rr.Body.Bytes())
	if payload["id"] != "u_1" {
		t.Fatalf("id = %#v, want u_1", payload["id"])
	}
}

// JSON 会通过根包 facade 直接写回紧凑 JSON。
func TestJSON_DelegatesToResp(t *testing.T) {
	rr := httptest.NewRecorder()

	if err := JSON(rr, http.StatusAccepted, map[string]any{"id": "u_1"}); err != nil {
		t.Fatalf("JSON() error = %v", err)
	}
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", rr.Code)
	}
	if body := rr.Body.String(); body != "{\"id\":\"u_1\"}\n" {
		t.Fatalf("body = %q, want compact JSON", body)
	}
}

// JSONBlob 会通过根包 facade 直接写回原始 JSON 字节。
func TestJSONBlob_DelegatesToResp(t *testing.T) {
	rr := httptest.NewRecorder()

	if err := JSONBlob(rr, http.StatusAccepted, []byte(`{"id":"u_1"}`)); err != nil {
		t.Fatalf("JSONBlob() error = %v", err)
	}
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusAccepted)
	}
	if body := rr.Body.String(); body != `{"id":"u_1"}` {
		t.Fatalf("body = %q, want raw JSON", body)
	}
}

// Created 会通过根包 facade 写回标准 201 JSON 响应。
func TestCreated_DelegatesToResp(t *testing.T) {
	rr := httptest.NewRecorder()

	if err := Created(rr, map[string]any{"id": "u_1"}); err != nil {
		t.Fatalf("Created() error = %v", err)
	}
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusCreated)
	}

	payload := decodeRootPayload(t, rr.Body.Bytes())
	if payload["id"] != "u_1" {
		t.Fatalf("id = %#v, want u_1", payload["id"])
	}
}

// NoContent 会通过根包 facade 写回标准 204 响应。
func TestNoContent_DelegatesToResp(t *testing.T) {
	rr := httptest.NewRecorder()

	if err := NoContent(rr); err != nil {
		t.Fatalf("NoContent() error = %v", err)
	}
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNoContent)
	}
	if rr.Body.Len() != 0 {
		t.Fatalf("body = %q, want empty", rr.Body.String())
	}
}

func newJSONRequest(method, target, body string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func newRouteRequest(method, target, name, value string) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	return withRouteParam(req, name, value)
}

func newRouteJSONRequest(method, target, body, name, value string) *http.Request {
	req := newJSONRequest(method, target, body)
	return withRouteParam(req, name, value)
}

func withRouteParam(req *http.Request, name, value string) *http.Request {
	req.Pattern = "/{" + name + "}"
	req.SetPathValue(name, value)
	return req
}

func decodeRootPayload(t *testing.T, body []byte) rootPayloadMap {
	t.Helper()

	var payload rootPayloadMap
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	return payload
}

func assertSingleRootViolation(t *testing.T, err error) reqx.Violation {
	t.Helper()

	payload := decodeRootPayload(t, mustWriteRootError(t, err))
	errorsValue, ok := payload["errors"].([]any)
	if !ok || len(errorsValue) != 1 {
		t.Fatalf("errors = %#v, want single violation", payload["errors"])
	}

	violationMap, ok := errorsValue[0].(map[string]any)
	if !ok {
		t.Fatalf("violation type = %T, want map[string]any", errorsValue[0])
	}

	return reqx.Violation{
		Field:  stringValue(violationMap["field"]),
		In:     stringValue(violationMap["in"]),
		Code:   stringValue(violationMap["code"]),
		Detail: stringValue(violationMap["detail"]),
	}
}

func mustWriteRootError(t *testing.T, err error) []byte {
	t.Helper()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if writeErr := WriteError(rr, req, err); writeErr != nil {
		t.Fatalf("WriteError() error = %v", writeErr)
	}
	return rr.Body.Bytes()
}

func stringValue(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
