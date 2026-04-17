package reqx

import (
	"encoding/json"
	"errors"
	"hash/fnv"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fuzzBindBodyRequest struct {
	Name string `json:"name"`
}

func FuzzBindPublicContracts(f *testing.F) {
	f.Add("0", "existing", "", "")
	f.Add("1", "kanata", "", "application/json")
	f.Add("2", "kanata", "", "application/json")
	f.Add("3", "kanata", "", "text/plain")
	f.Add("4", "kanata", "", "application/json")
	f.Add("5", "kanata", "", "text/plain")
	f.Add("6", "kanata", "", "text/plain")

	f.Fuzz(func(t *testing.T, variantKey, name, extra, contentType string) {
		switch fuzzBindVariant(variantKey, name, extra, contentType) {
		case 0:
			fuzzBindZeroByteBodyNoop(t, name, contentType)
		case 1:
			fuzzBindValidJSONObject(t, name)
		case 2:
			fuzzBindInvalidJSONKeepsTarget(t, name, extra)
		case 3:
			fuzzBindUnsupportedMediaTypePriority(t)
		case 4:
			fuzzBindRequestTooLargePriority(t)
		case 5:
			fuzzBindUsageErrorPriority(t)
		default:
			fuzzBindProbeReadErrorPriority(t)
		}
	})
}

func fuzzBindZeroByteBodyNoop(t *testing.T, name, contentType string) {
	t.Helper()

	dst := fuzzBindBodyRequest{Name: fuzzBindJSONSafeString(name)}
	req := httptest.NewRequest(http.MethodPost, "/", http.NoBody)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	if err := BindBody(req, &dst); err != nil {
		t.Fatalf("BindBody() error = %v", err)
	}
	if dst != (fuzzBindBodyRequest{Name: fuzzBindJSONSafeString(name)}) {
		t.Fatalf("dst = %#v, want unchanged zero-byte no-op", dst)
	}
}

func fuzzBindValidJSONObject(t *testing.T, name string) {
	t.Helper()

	payload := `{"name":` + fuzzBindJSONString(name) + `}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	dst := fuzzBindBodyRequest{Name: "existing"}
	if err := BindBody(req, &dst); err != nil {
		t.Fatalf("BindBody() error = %v", err)
	}
	if dst != (fuzzBindBodyRequest{Name: fuzzBindJSONSafeString(name)}) {
		t.Fatalf("dst = %#v, want decoded JSON object", dst)
	}
}

func fuzzBindInvalidJSONKeepsTarget(t *testing.T, name, extra string) {
	t.Helper()

	payload := `{"name":` + fuzzBindJSONString(name) + `}` + fuzzBindNonWhitespaceTail(extra)
	dst := fuzzBindBodyRequest{Name: "existing"}

	err := BindBody(newJSONRequest(http.MethodPost, "/", payload), &dst)
	_ = assertHTTPStatusCode(t, err, http.StatusBadRequest, CodeInvalidJSON)
	if dst != (fuzzBindBodyRequest{Name: "existing"}) {
		t.Fatalf("dst = %#v, want unchanged after invalid JSON", dst)
	}
}

func fuzzBindUnsupportedMediaTypePriority(t *testing.T) {
	t.Helper()

	payload := "{" + strings.Repeat("a", int(defaultMaxBodyBytes)+1)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(payload))
	req.Header.Set("Content-Type", "text/plain")

	dst := fuzzBindBodyRequest{Name: "existing"}
	err := BindBody(req, &dst)
	_ = assertHTTPStatusCode(t, err, http.StatusUnsupportedMediaType, CodeUnsupportedMediaType)
	if dst != (fuzzBindBodyRequest{Name: "existing"}) {
		t.Fatalf("dst = %#v, want unchanged after unsupported media type", dst)
	}
}

func fuzzBindRequestTooLargePriority(t *testing.T) {
	t.Helper()

	payload := "{" + strings.Repeat("a", int(defaultMaxBodyBytes)+1)
	dst := fuzzBindBodyRequest{Name: "existing"}

	err := BindBody(newJSONRequest(http.MethodPost, "/", payload), &dst)
	_ = assertHTTPStatusCode(t, err, http.StatusRequestEntityTooLarge, CodeRequestTooLarge)
	if dst != (fuzzBindBodyRequest{Name: "existing"}) {
		t.Fatalf("dst = %#v, want unchanged after oversized body", dst)
	}
}

func fuzzBindUsageErrorPriority(t *testing.T) {
	t.Helper()

	wantErr := errors.New("read failed")
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Content-Type", "text/plain")
	req.Body = bindBodyReadErrorCloser{err: wantErr}
	req.ContentLength = -1

	var unsupported map[string]string
	err := BindBody(req, &unsupported)
	assertNotHTTPError(t, err)
	if errors.Is(err, wantErr) {
		t.Fatalf("BindBody() error = %v, want usage error before body inspection", err)
	}
}

func fuzzBindProbeReadErrorPriority(t *testing.T) {
	t.Helper()

	wantErr := errors.New("read failed")
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Content-Type", "text/plain")
	req.Body = bindBodyReadErrorCloser{err: wantErr}
	req.ContentLength = -1

	dst := fuzzBindBodyRequest{Name: "existing"}
	err := BindBody(req, &dst)
	if !errors.Is(err, wantErr) {
		t.Fatalf("BindBody() error = %v, want %v", err, wantErr)
	}
	if dst != (fuzzBindBodyRequest{Name: "existing"}) {
		t.Fatalf("dst = %#v, want unchanged after read error", dst)
	}
}

func fuzzBindVariant(variantKey string, inputs ...string) int {
	if len(variantKey) == 1 && variantKey[0] >= '0' && variantKey[0] <= '6' {
		return int(variantKey[0] - '0')
	}

	h := fnv.New32a()
	_, _ = h.Write([]byte(variantKey))
	for _, input := range inputs {
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(input))
	}
	return int(h.Sum32() % 7)
}

func fuzzBindJSONString(value string) string {
	body, err := json.Marshal(value)
	if err != nil {
		return `""`
	}
	return string(body)
}

func fuzzBindJSONSafeString(value string) string {
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

func fuzzBindNonWhitespaceTail(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "x"
	}
	return trimmed
}
