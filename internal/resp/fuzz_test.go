package resp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kanata996/hah/internal/errx"
)

func FuzzJSONWritersPublicContracts(f *testing.F) {
	f.Add(uint8(0), "u_1")
	f.Add(uint8(1), "\t")
	f.Add(uint8(2), "")
	f.Add(uint8(3), "kanata")

	f.Fuzz(func(t *testing.T, variant uint8, value string) {
		fuzzJSONWriterContracts(t, variant, value)
	})
}

func FuzzSuccessWritersPublicContracts(f *testing.F) {
	f.Add(uint8(0), "u_1")
	f.Add(uint8(1), "\t")
	f.Add(uint8(2), "")
	f.Add(uint8(3), "kanata")

	f.Fuzz(func(t *testing.T, variant uint8, value string) {
		fuzzSuccessWriterContracts(t, variant, value)
	})
}

func FuzzNoContentPublicContracts(f *testing.F) {
	f.Add("trace-1", "application/json", "999")
	f.Add("", "text/plain", "0")
	f.Add("kanata", "", "")
	f.Add("trace/with space", "application/problem+json", "100")

	f.Fuzz(func(t *testing.T, traceID, staleContentType, staleContentLength string) {
		fuzzNoContentContracts(t, traceID, staleContentType, staleContentLength)
	})
}

func FuzzWriteErrorWrappedHTTPErrorPublicContracts(f *testing.F) {
	f.Add(http.StatusBadRequest, "payload invalid", "name")
	f.Add(http.StatusUnprocessableEntity, "", "")
	f.Add(99, "payload invalid", "name")
	f.Add(1000, "\xff", "\xff")

	f.Fuzz(func(t *testing.T, status int, detail, field string) {
		fuzzWriteErrorWrappedHTTPErrorContracts(t, status, detail, field)
	})
}

func fuzzJSONWriterContracts(t *testing.T, variant uint8, value string) {
	t.Helper()

	payload := map[string]string{"value": value}
	rr := httptest.NewRecorder()

	switch variant % 4 {
	case 0:
		err := JSON(rr, http.StatusAccepted, payload)
		assertRecorderRawJSONSuccess(t, rr, err, http.StatusAccepted, payload)
	case 1:
		err := JSON(rr, http.StatusOK, nil)
		assertJSONNullWriterResult(t, rr, err, http.StatusOK)
	case 2:
		err := OK(rr, payload)
		assertRecorderEnvelopeSuccess(t, rr, err, http.StatusOK, payload)
	default:
		err := Created(rr, payload)
		assertRecorderEnvelopeSuccess(t, rr, err, http.StatusCreated, payload)
	}
}

func fuzzSuccessWriterContracts(t *testing.T, variant uint8, value string) {
	t.Helper()

	payload := map[string]string{"value": value}
	rr := httptest.NewRecorder()

	switch variant % 3 {
	case 0:
		err := OK(rr, payload)
		assertRecorderEnvelopeSuccess(t, rr, err, http.StatusOK, payload)
	case 1:
		err := Accepted(rr, payload)
		assertRecorderEnvelopeSuccess(t, rr, err, http.StatusAccepted, payload)
	default:
		err := Created(rr, payload)
		assertRecorderEnvelopeSuccess(t, rr, err, http.StatusCreated, payload)
	}
}

func fuzzNoContentContracts(t *testing.T, traceID, staleContentType, staleContentLength string) {
	t.Helper()

	rr := httptest.NewRecorder()
	rr.Header().Set("X-Trace-ID", traceID)
	rr.Header().Set("Content-Type", staleContentType)
	rr.Header().Set("Content-Length", staleContentLength)

	err := NoContent(rr)
	assertNoContentWriterResult(t, rr, err, traceID)
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

func assertRecorderRawJSONSuccess(t *testing.T, rr *httptest.ResponseRecorder, err error, status int, payload map[string]string) {
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

func assertRecorderEnvelopeSuccess(t *testing.T, rr *httptest.ResponseRecorder, err error, status int, payload map[string]string) {
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

	body := decodePayload(t, rr.Body.Bytes())
	if got := body["code"]; got != float64(0) {
		t.Fatalf("code = %#v, want 0", got)
	}
	if got := body["message"]; got != "success" {
		t.Fatalf("message = %#v, want success", got)
	}

	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("data = %#v, want object", body["data"])
	}
	if got := data["value"]; got != jsonSafeString(payload["value"]) {
		t.Fatalf("data.value = %#v, want %#v", got, jsonSafeString(payload["value"]))
	}
}

func assertNoContentWriterResult(t *testing.T, rr *httptest.ResponseRecorder, err error, wantTraceID string) {
	t.Helper()

	if err != nil {
		t.Fatalf("writer error = %v", err)
	}
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNoContent)
	}
	if rr.Body.Len() != 0 {
		t.Fatalf("body = %q, want empty", rr.Body.String())
	}
	if got := rr.Header().Get("X-Trace-ID"); got != wantTraceID {
		t.Fatalf("X-Trace-ID = %q, want %q", got, wantTraceID)
	}
	if got := rr.Header().Get("Content-Type"); got != "" {
		t.Fatalf("Content-Type = %q, want empty", got)
	}
	if got := rr.Header().Get("Content-Length"); got != "" {
		t.Fatalf("Content-Length = %q, want empty", got)
	}
}

func fuzzWriteErrorWrappedHTTPErrorContracts(t *testing.T, status int, detail, field string) {
	t.Helper()

	rr := httptest.NewRecorder()

	hiddenCause := "internal cause sentinel"
	wantField := map[string]any{
		"code":   string(errx.CodeInvalid),
		"detail": "is invalid",
	}
	if field != "" {
		wantField["field"] = jsonSafeString(field)
	}
	httpErr := errx.NewHTTPErrorWithCause(status, "", detail, errors.New(hiddenCause)).WithFieldErrors([]errx.FieldError{
		{Field: field, Code: errx.CodeInvalid, Detail: "is invalid"},
	})
	input := fmt.Errorf("wrapped: %w", httpErr)
	wantStatus := httpErr.Status()
	wantTopCode := wantStatus * 100
	wantReason := httpErr.Code()
	wantMessage := jsonSafeString(httpErr.Detail())

	if err := WriteError(rr, input); err != nil {
		t.Fatalf("WriteError() error = %v", err)
	}
	if rr.Code != wantStatus {
		t.Fatalf("status = %d, want %d", rr.Code, wantStatus)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}

	body := decodePayload(t, rr.Body.Bytes())
	if got := body["code"]; got != float64(wantTopCode) {
		t.Fatalf("code = %#v, want %d", got, wantTopCode)
	}
	if got := body["message"]; got != wantMessage {
		t.Fatalf("message = %#v, want %q", got, wantMessage)
	}
	errorValue, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("error = %#v, want object", body["error"])
	}
	if got := errorValue["reason"]; got != wantReason {
		t.Fatalf("error.reason = %#v, want %q", got, wantReason)
	}
	if _, exists := errorValue["status"]; exists {
		t.Fatalf("error.status unexpectedly present: %#v", errorValue["status"])
	}
	if _, exists := errorValue["code"]; exists {
		t.Fatalf("error.code unexpectedly present: %#v", errorValue["code"])
	}
	if _, exists := errorValue["title"]; exists {
		t.Fatalf("error.title unexpectedly present: %#v", errorValue["title"])
	}
	if _, exists := errorValue["detail"]; exists {
		t.Fatalf("error.detail unexpectedly present: %#v", errorValue["detail"])
	}

	details, ok := errorValue["details"].([]any)
	if !ok || len(details) != 1 {
		t.Fatalf("details = %#v, want 1 item", errorValue["details"])
	}
	assertPublicErrorObject(t, details[0], wantField)

	if bytes.Contains(rr.Body.Bytes(), []byte(hiddenCause)) {
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
