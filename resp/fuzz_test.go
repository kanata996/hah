package resp

// 测试清单：
// - 标记说明：[✓] 已核对且已有真实覆盖；[x] 尚未完成，不得作为验收依据。
// - [✓] `JSON` / `OK` / `Created` 在任意字符串 payload 输入下维持稳定写回契约。
// - [✓] `JSONBlob` 在任意原始字节与状态码输入下维持 raw bytes 透传与拒绝契约。
// - [✓] `WriteError` 在任意公开 detail、状态码和常见 error 变体下维持稳定的公共错误契约且不泄漏内部 cause。
// - [✓] 本文件提供单一 `FuzzRespPublicContracts` 入口，可直接配合仓库规范中的 `-fuzz=Fuzz` 执行。

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kanata996/hah/errx"
)

func FuzzRespPublicContracts(f *testing.F) {
	previousDefault := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	f.Cleanup(func() {
		slog.SetDefault(previousDefault)
	})

	f.Add(uint8(0), uint8(0), http.StatusOK, "u_1", "  ", []byte(nil))
	f.Add(uint8(0), uint8(1), http.StatusOK, "u_1", "\t", []byte(nil))
	f.Add(uint8(0), uint8(2), http.StatusNoContent, "u_1", "  ", []byte(nil))
	f.Add(uint8(0), uint8(3), 1000, "u_1", "  ", []byte(nil))
	f.Add(uint8(1), uint8(0), http.StatusAccepted, "", "", []byte(`{"id":"u_1"}`))
	f.Add(uint8(1), uint8(0), http.StatusNoContent, "", "", []byte(`{"id":"u_1"}`))
	f.Add(uint8(1), uint8(0), 1000, "", "", []byte(`{"id":"u_1"}`))
	f.Add(uint8(2), uint8(0), http.StatusBadRequest, "payload invalid", "name", []byte(nil))
	f.Add(uint8(2), uint8(1), 99, "payload invalid", "name", []byte(nil))
	f.Add(uint8(2), uint8(2), http.StatusGatewayTimeout, "", "", []byte(nil))
	f.Add(uint8(2), uint8(3), http.StatusInternalServerError, "", "", []byte(nil))

	f.Fuzz(func(t *testing.T, kind uint8, variant uint8, status int, a, b string, raw []byte) {
		switch kind % 3 {
		case 0:
			fuzzSuccessWriterContracts(t, variant, status, a, b)
		case 1:
			fuzzJSONBlobContracts(t, status, raw)
		default:
			fuzzWriteErrorContracts(t, variant, status, a, b)
		}
	})
}

func fuzzSuccessWriterContracts(t *testing.T, variant uint8, status int, value, _ string) {
	t.Helper()

	payload := map[string]string{"value": value}
	rr := httptest.NewRecorder()

	switch variant % 4 {
	case 0:
		err := JSON(rr, status, payload)
		assertJSONWriterResult(t, rr, err, status, payload)
	case 1:
		err := JSON(rr, status, nil)
		assertJSONNullWriterResult(t, rr, err, status)
	case 2:
		err := OK(rr, payload)
		assertRecorderJSONSuccess(t, rr, err, http.StatusOK, payload)
	default:
		err := Created(rr, payload)
		assertRecorderJSONSuccess(t, rr, err, http.StatusCreated, payload)
	}
}

func assertJSONWriterResult(t *testing.T, rr *httptest.ResponseRecorder, err error, status int, payload map[string]string) {
	t.Helper()

	if wantErr := wantBodyWriterError("JSON body writers", status); wantErr != "" {
		if err == nil || err.Error() != wantErr {
			t.Fatalf("writer error = %v, want %q", err, wantErr)
		}
		assertRecorderHasNoBodyOrContentType(t, rr)
		return
	}

	assertRecorderJSONSuccess(t, rr, err, status, payload)
}

func assertJSONNullWriterResult(t *testing.T, rr *httptest.ResponseRecorder, err error, status int) {
	t.Helper()

	if wantErr := wantBodyWriterError("JSON body writers", status); wantErr != "" {
		if err == nil || err.Error() != wantErr {
			t.Fatalf("writer error = %v, want %q", err, wantErr)
		}
		assertRecorderHasNoBodyOrContentType(t, rr)
		return
	}

	if err != nil {
		t.Fatalf("writer error = %v", err)
	}
	if rr.Code != status {
		t.Fatalf("status = %d, want %d", rr.Code, status)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	if got := rr.Body.String(); got != "null\n" {
		t.Fatalf("body = %q, want %q", got, "null\n")
	}
}

func assertRecorderJSONSuccess(t *testing.T, rr *httptest.ResponseRecorder, err error, status int, payload map[string]string) {
	t.Helper()

	if err != nil {
		t.Fatalf("writer error = %v", err)
	}
	if rr.Code != status {
		t.Fatalf("status = %d, want %d", rr.Code, status)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}

	wantBody := encodeContractJSON(t, payload)
	if got := rr.Body.String(); got != wantBody {
		t.Fatalf("body = %q, want %q", got, wantBody)
	}
}

func fuzzJSONBlobContracts(t *testing.T, status int, body []byte) {
	t.Helper()

	rr := httptest.NewRecorder()
	err := JSONBlob(rr, status, body)

	if wantErr := wantBodyWriterError("JSON body writers", status); wantErr != "" {
		if err == nil || err.Error() != wantErr {
			t.Fatalf("JSONBlob() error = %v, want %q", err, wantErr)
		}
		assertRecorderHasNoBodyOrContentType(t, rr)
		return
	}

	if err != nil {
		t.Fatalf("JSONBlob() error = %v", err)
	}
	if rr.Code != status {
		t.Fatalf("status = %d, want %d", rr.Code, status)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	if got := rr.Body.Bytes(); !bytes.Equal(got, body) {
		t.Fatalf("body = %q, want %q", got, body)
	}
}

func fuzzWriteErrorContracts(t *testing.T, variant uint8, status int, detail, field string) {
	t.Helper()

	req := fuzzWriteErrorRequest(variant)
	rr := httptest.NewRecorder()

	var input error
	var hiddenCause string
	var wantStatus int
	var wantCode string
	var wantTitle string
	var wantDetail string
	var wantErrors map[string]any

	switch variant % 4 {
	case 0:
		hiddenCause = "internal cause sentinel"
		input = errors.New(hiddenCause + ": " + detail)
		wantStatus = http.StatusInternalServerError
		wantCode = "internal_error"
		wantTitle = http.StatusText(http.StatusInternalServerError)
		wantDetail = http.StatusText(http.StatusInternalServerError)
	case 1:
		input = context.Canceled
		wantStatus = 499
		wantCode = "client_closed_request"
		wantTitle = "Client Closed Request"
		wantDetail = "Client Closed Request"
	case 2:
		input = context.DeadlineExceeded
		wantStatus = http.StatusGatewayTimeout
		wantCode = "timeout"
		wantTitle = http.StatusText(http.StatusGatewayTimeout)
		wantDetail = http.StatusText(http.StatusGatewayTimeout)
	default:
		hiddenCause = "internal cause sentinel"
		wantErrors = map[string]any{"field": jsonSafeString(field)}
		httpErr := errx.NewHTTPErrorWithCause(status, "", detail, errors.New(hiddenCause), map[string]any{"field": field})
		input = fmt.Errorf("wrapped: %w", httpErr)
		wantStatus = httpErr.Status()
		wantCode = httpErr.Code()
		wantTitle = httpErr.Title()
		wantDetail = jsonSafeString(httpErr.Detail())
	}

	if err := WriteError(rr, req, input); err != nil {
		t.Fatalf("WriteError() error = %v", err)
	}
	if rr.Code != wantStatus {
		t.Fatalf("status = %d, want %d", rr.Code, wantStatus)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Fatalf("Content-Type = %q, want application/problem+json", got)
	}

	body := decodePayload(t, rr.Body.Bytes())
	if got := body["status"]; got != float64(wantStatus) {
		t.Fatalf("status = %#v, want %d", got, wantStatus)
	}
	if got := body["code"]; got != wantCode {
		t.Fatalf("code = %#v, want %q", got, wantCode)
	}
	if got := body["title"]; got != wantTitle {
		t.Fatalf("title = %#v, want %q", got, wantTitle)
	}
	if got := body["detail"]; got != wantDetail {
		t.Fatalf("detail = %#v, want %q", got, wantDetail)
	}

	if wantErrors == nil {
		if _, exists := body["errors"]; exists {
			t.Fatalf("errors unexpectedly present: %#v", body["errors"])
		}
	} else {
		errorsValue, ok := body["errors"].([]any)
		if !ok || len(errorsValue) != 1 {
			t.Fatalf("errors = %#v, want 1 item", body["errors"])
		}
		assertPublicErrorObject(t, errorsValue[0], wantErrors)
	}

	if hiddenCause != "" && bytes.Contains(rr.Body.Bytes(), []byte(hiddenCause)) {
		t.Fatalf("body leaked internal cause: %q", rr.Body.String())
	}
}

func fuzzWriteErrorRequest(variant uint8) *http.Request {
	switch (variant / 4) % 3 {
	case 0:
		return nil
	case 1:
		return httptest.NewRequest(http.MethodGet, "/users/u_1", nil)
	default:
		return httptest.NewRequest(http.MethodHead, "/users/u_1", nil)
	}
}

func encodeContractJSON(t *testing.T, payload map[string]string) string {
	t.Helper()

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	if err := enc.Encode(payload); err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	return buf.String()
}

func wantBodyWriterError(writerName string, status int) string {
	switch {
	case status < 100 || status > 999:
		return fmt.Sprintf("resp: invalid HTTP status %d", status)
	case status < http.StatusOK:
		return fmt.Sprintf("resp: %s cannot use informational status %d", writerName, status)
	case status == http.StatusNoContent || status == http.StatusResetContent || status == http.StatusNotModified:
		return fmt.Sprintf("resp: %s cannot use bodyless status %d", writerName, status)
	default:
		return ""
	}
}

func jsonSafeString(value string) string {
	body, err := json.Marshal(value)
	if err != nil {
		return strings.ToValidUTF8(value, "\uFFFD")
	}

	var normalized string
	if err := json.Unmarshal(body, &normalized); err != nil {
		return strings.ToValidUTF8(value, "\uFFFD")
	}
	return normalized
}
