package resp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kanata996/hah/errx"
)

func FuzzRespPublicContracts(f *testing.F) {
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
			fuzzSuccessWriterContracts(t, variant, a)
		case 1:
			fuzzJSONBlobContracts(t, raw)
		default:
			fuzzWriteErrorContracts(t, variant, status, a, b)
		}
	})
}

func fuzzSuccessWriterContracts(t *testing.T, variant uint8, value string) {
	t.Helper()

	payload := map[string]string{"value": value}
	rr := httptest.NewRecorder()

	switch variant % 4 {
	case 0:
		err := JSON(rr, http.StatusAccepted, payload)
		assertRecorderJSONSuccess(t, rr, err, http.StatusAccepted, payload)
	case 1:
		err := JSON(rr, http.StatusOK, nil)
		assertJSONNullWriterResult(t, rr, err, http.StatusOK)
	case 2:
		err := OK(rr, payload)
		assertRecorderJSONSuccess(t, rr, err, http.StatusOK, payload)
	default:
		err := Created(rr, payload)
		assertRecorderJSONSuccess(t, rr, err, http.StatusCreated, payload)
	}
}

func assertJSONNullWriterResult(t *testing.T, rr *httptest.ResponseRecorder, err error, status int) {
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

func fuzzJSONBlobContracts(t *testing.T, body []byte) {
	t.Helper()

	rr := httptest.NewRecorder()
	err := JSONBlob(rr, http.StatusAccepted, body)

	if err != nil {
		t.Fatalf("JSONBlob() error = %v", err)
	}
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusAccepted)
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
		wantErrors = map[string]any{
			"code":   errx.ViolationCodeInvalid,
			"detail": "is invalid",
		}
		if normalizedField := jsonSafeString(field); normalizedField != "" {
			wantErrors["field"] = normalizedField
		}
		httpErr := errx.NewHTTPErrorWithCause(status, "", detail, errors.New(hiddenCause)).WithViolations([]errx.Violation{
			{Field: field, Code: errx.ViolationCodeInvalid, Detail: "is invalid"},
		})
		input = fmt.Errorf("wrapped: %w", httpErr)
		wantStatus = httpErr.Status()
		wantCode = httpErr.Code()
		wantTitle = httpErr.Title()
		wantDetail = jsonSafeString(httpErr.Detail())
	}

	if err := WriteError(rr, input); err != nil {
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

func encodeContractJSON(t *testing.T, payload map[string]string) string {
	t.Helper()

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	if err := enc.Encode(payload); err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	return buf.String()
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
