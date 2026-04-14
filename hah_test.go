package hah

import (
	"context"
	"encoding/json"
	"errors"
	"io"
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
// [✓] 根包 facade 公开常用绑定入口：Bind、BindBody、BindQueryParams、BindPathValues、BindHeaders
// [✓] 根包 facade 继续暴露 body-required helper 与统一错误响应写回

type rootPayloadMap map[string]any

type partialReadErrorCloser struct {
	done bool
	err  error
}

func (r *partialReadErrorCloser) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if r.done {
		return 0, io.EOF
	}
	r.done = true
	p[0] = '{'
	return 1, r.err
}

func (r *partialReadErrorCloser) Close() error {
	return nil
}

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

// BindQueryParams 只从 query 参数绑定数据。
func TestBindQueryParams_DelegatesToBind(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/accounts?cursor=next", nil)

	var dst struct {
		Cursor string `query:"cursor"`
	}

	if err := BindQueryParams(req, &dst); err != nil {
		t.Fatalf("BindQueryParams() error = %v", err)
	}
	if dst.Cursor != "next" {
		t.Fatalf("cursor = %q, want next", dst.Cursor)
	}
}

// BindPathValues 只从 path 参数绑定数据。
func TestBindPathValues_DelegatesToBind(t *testing.T) {
	req := newRouteRequest(http.MethodGet, "/accounts/acct_123", "account_id", "acct_123")

	var dst struct {
		AccountID string `param:"account_id"`
	}

	if err := BindPathValues(req, &dst); err != nil {
		t.Fatalf("BindPathValues() error = %v", err)
	}
	if dst.AccountID != "acct_123" {
		t.Fatalf("account_id = %q, want acct_123", dst.AccountID)
	}
}

// BindHeaders 只从 header 绑定数据。
func TestBindHeaders_DelegatesToBind(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/accounts", nil)
	req.Header.Set("X-Actor", "kanata")

	var dst struct {
		Actor string `header:"X-Actor"`
	}

	if err := BindHeaders(req, &dst); err != nil {
		t.Fatalf("BindHeaders() error = %v", err)
	}
	if dst.Actor != "kanata" {
		t.Fatalf("actor = %q, want kanata", dst.Actor)
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

// Query 也会通过根包 facade 暴露 query 专用的多值读取 builder。
func TestQuery_ValuesDelegatesToReqx(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/accounts?tag=a&tag=b", nil)

	tags, err := Query(req, "tag").Values().Required().Get()
	if err != nil {
		t.Fatalf("Query().Values().Required().Get() error = %v", err)
	}
	if got := strings.Join(tags, ","); got != "a,b" {
		t.Fatalf("tags = %q, want a,b", got)
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

// 同一请求先经过 body-required 探测，再进入 body 绑定时，底层短读错误不能被后续探测掩盖。
func TestRequireBodyThenBindBody_PreservesShortReadError(t *testing.T) {
	wantErr := errors.New("short read")
	req := newJSONRequest(http.MethodPost, "/accounts", "")
	req.Body = &partialReadErrorCloser{err: wantErr}
	req.ContentLength = -1

	if err := RequireBody(req); !errors.Is(err, wantErr) {
		t.Fatalf("RequireBody() error = %v, want %v", err, wantErr)
	}

	dst := struct {
		Name string `json:"name"`
	}{Name: "existing"}
	if err := BindBody(req, &dst); !errors.Is(err, wantErr) {
		t.Fatalf("BindBody() error = %v, want %v", err, wantErr)
	}
	if dst.Name != "existing" {
		t.Fatalf("name = %q, want existing value preserved on read error", dst.Name)
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
