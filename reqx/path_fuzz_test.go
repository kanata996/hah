package reqx

// 测试清单：
// - 标记说明：[✓] 已核对且已有真实覆盖；[x] 尚未完成，不得作为验收依据。
// - [✓] path wildcard 解析与存在性判断在任意 pattern/name 输入下保持稳定且不 panic。
// - [✓] `BindPathValues` 与 `ParamInt` 在任意 path pattern/value 输入下维持稳定的公开契约。
// - [✓] `BindQueryParams` 在任意 query 输入下维持稳定的公开契约。
// - [✓] `BindBody` 在任意 body 与 Content-Type 输入下维持稳定的公开契约。
// - [✓] `BindHeaders` 在任意 header key/value 输入下维持稳定的公开契约。
// - [✓] 本文件提供单一 `FuzzReqxPublicContracts` 入口，可直接配合仓库规范中的 `-fuzz=Fuzz` 执行。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func FuzzReqxPublicContracts(f *testing.F) {
	f.Add(uint8(0), uint8(0), "/users/{id}", "42", "id")
	f.Add(uint8(0), uint8(0), "GET /files/{path...}/{$}", " ", "path")
	f.Add(uint8(0), uint8(0), "/users/{id", "oops", "id")
	f.Add(uint8(1), uint8(0), "page=2", "", "")
	f.Add(uint8(1), uint8(0), "page=oops", "", "")
	f.Add(uint8(1), uint8(0), "page=1&page=2", "", "")
	f.Add(uint8(1), uint8(0), "name=ignored&extra=1", "", "")
	f.Add(uint8(2), uint8(0), "kanata", "17", "application/json")
	f.Add(uint8(2), uint8(1), "kanata", "oops", "application/json")
	f.Add(uint8(2), uint8(2), "kanata", "17", "")
	f.Add(uint8(2), uint8(3), "kanata", "17", "text/plain")
	f.Add(uint8(2), uint8(4), "kanata", "17", "application/problem+json")
	f.Add(uint8(3), uint8(0), " x-request-id ", "req-1", "2")
	f.Add(uint8(3), uint8(1), "x-request-id", "req-1", "2")
	f.Add(uint8(3), uint8(0), "x-request-id", "req-1", "oops")
	f.Add(uint8(3), uint8(0), "x-extra", "req-1", "2")

	f.Fuzz(func(t *testing.T, kind uint8, variant uint8, a, b, c string) {
		switch kind % 4 {
		case 0:
			fuzzPathContracts(t, a, b, c)
		case 1:
			fuzzQueryContracts(t, a)
		case 2:
			fuzzBodyContracts(t, variant, a, b, c)
		default:
			fuzzHeaderContracts(t, variant, a, b, c)
		}
	})
}

func fuzzPathContracts(t *testing.T, pattern, rawValue, name string) {
	t.Helper()

	names := pathWildcardNames(pattern)
	for i, wildcard := range names {
		if strings.TrimSpace(wildcard) == "" {
			t.Fatalf("names[%d] = %q, want non-blank wildcard", i, wildcard)
		}
		if wildcard == "$" {
			t.Fatalf("names[%d] = %q, want anonymous wildcard filtered out", i, wildcard)
		}
	}

	wantExists := false
	trimmedName := strings.TrimSpace(name)
	for _, wildcard := range names {
		if wildcard == trimmedName {
			wantExists = true
			break
		}
	}
	if got := pathWildcardExists(pattern, name); got != wantExists {
		t.Fatalf("pathWildcardExists(%q, %q) = %v, want %v (names = %#v)", pattern, name, got, wantExists, names)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Pattern = pattern
	req.SetPathValue("id", rawValue)

	var dst struct {
		ID string `param:"id"`
	}
	dst.ID = "existing"

	if err := BindPathValues(req, &dst); err != nil {
		t.Fatalf("BindPathValues() error = %v", err)
	}

	trimmedValue := strings.TrimSpace(rawValue)
	if trimmedValue != "" {
		if dst.ID != trimmedValue {
			t.Fatalf("BindPathValues() id = %q, want %q", dst.ID, trimmedValue)
		}
	} else if pathWildcardExists(pattern, "id") {
		if dst.ID != "" {
			t.Fatalf("BindPathValues() id = %q, want empty string", dst.ID)
		}
	} else if dst.ID != "existing" {
		t.Fatalf("BindPathValues() id = %q, want preserved existing value", dst.ID)
	}

	got, err := ParamInt(req, "id")
	if trimmedValue == "" {
		violation := assertSingleViolation(t, err)
		if violation.Field != "id" || violation.In != ViolationInPath || violation.Code != ViolationCodeRequired {
			t.Fatalf("violation = %#v, want required path violation", violation)
		}
		return
	}

	want, parseErr := strconv.Atoi(trimmedValue)
	if parseErr != nil {
		violation := assertSingleViolation(t, err)
		if violation.Field != "id" || violation.In != ViolationInPath || violation.Code != ViolationCodeType {
			t.Fatalf("violation = %#v, want type path violation", violation)
		}
		return
	}

	if err != nil {
		t.Fatalf("ParamInt() error = %v", err)
	}
	if got != want {
		t.Fatalf("ParamInt() = %d, want %d", got, want)
	}
}

func fuzzQueryContracts(t *testing.T, rawQuery string) {
	t.Helper()

	req := &http.Request{
		Method: http.MethodGet,
		URL:    &url.URL{RawQuery: rawQuery},
		Header: make(http.Header),
	}

	type request struct {
		Page int `query:"page"`
	}

	dst := request{Page: 7}
	err := BindQueryParams(req, &dst)

	values := req.URL.Query()["page"]
	switch len(values) {
	case 0:
		if err != nil {
			t.Fatalf("BindQueryParams() error = %v", err)
		}
		if dst.Page != 7 {
			t.Fatalf("page = %d, want preserved existing value 7", dst.Page)
		}
	default:
		if len(values) > 1 {
			violation := assertSingleViolation(t, err)
			if violation.Field != "page" || violation.In != ViolationInQuery || violation.Code != ViolationCodeMultiple {
				t.Fatalf("violation = %#v, want repeated query violation", violation)
			}
			if dst.Page != 7 {
				t.Fatalf("page = %d, want preserved existing value 7", dst.Page)
			}
			return
		}

		want, parseErr := strconv.Atoi(values[0])
		if parseErr != nil {
			violation := assertSingleViolation(t, err)
			if violation.Field != "page" || violation.In != ViolationInQuery || violation.Code != ViolationCodeType {
				t.Fatalf("violation = %#v, want type query violation", violation)
			}
			if dst.Page != 7 {
				t.Fatalf("page = %d, want preserved existing value 7", dst.Page)
			}
			return
		}

		if err != nil {
			t.Fatalf("BindQueryParams() error = %v", err)
		}
		if dst.Page != want {
			t.Fatalf("page = %d, want %d", dst.Page, want)
		}
	}
}

func fuzzBodyContracts(t *testing.T, variant uint8, name, ageText, contentType string) {
	t.Helper()

	type request struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	payload, wantName, wantAge, expectBodyPreserved := fuzzBodyPayload(variant, name, ageText)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(payload))
	if strings.TrimSpace(contentType) != "" {
		req.Header.Set("Content-Type", contentType)
	}

	dst := request{
		Name: "existing",
		Age:  7,
	}
	err := BindBody(req, &dst)

	if strings.TrimSpace(payload) == "" {
		if !emptyBodyAllowsContentType(contentType) {
			_ = assertHTTPError(t, err, http.StatusUnsupportedMediaType, CodeUnsupportedMediaType, "Content-Type must be application/json or application/*+json")
			if dst.Name != "existing" || dst.Age != 7 {
				t.Fatalf("dst = %#v, want preserved destination on empty-body content-type failure", dst)
			}
			return
		}
		if err != nil {
			t.Fatalf("BindBody() error = %v", err)
		}
		if dst.Name != "existing" || dst.Age != 7 {
			t.Fatalf("dst = %#v, want preserved destination on empty body", dst)
		}
		return
	}

	if !nonEmptyBodyAllowsContentType(contentType) {
		_ = assertHTTPError(t, err, http.StatusUnsupportedMediaType, CodeUnsupportedMediaType, "Content-Type must be application/json or application/*+json")
		if dst.Name != "existing" || dst.Age != 7 {
			t.Fatalf("dst = %#v, want preserved destination on unsupported content type", dst)
		}
		return
	}

	switch variant % 7 {
	case 1:
		violation := assertSingleViolation(t, err)
		if violation.Field != "age" || violation.In != ViolationInBody || violation.Code != ViolationCodeType {
			t.Fatalf("violation = %#v, want type body violation", violation)
		}
		if dst.Name != "existing" || dst.Age != 7 {
			t.Fatalf("dst = %#v, want preserved destination on body type failure", dst)
		}
	case 2:
		_ = assertHTTPError(t, err, http.StatusBadRequest, CodeInvalidJSON, "request body must be valid JSON")
		if dst.Name != "existing" || dst.Age != 7 {
			t.Fatalf("dst = %#v, want preserved destination on invalid JSON", dst)
		}
	case 3:
		_ = assertHTTPError(t, err, http.StatusBadRequest, CodeInvalidJSON, "request body must contain a single JSON value")
		if dst.Name != "existing" || dst.Age != 7 {
			t.Fatalf("dst = %#v, want preserved destination on multiple JSON values", dst)
		}
	default:
		if err != nil {
			t.Fatalf("BindBody() error = %v", err)
		}
		if dst.Name != wantName || dst.Age != wantAge {
			t.Fatalf("dst = %#v, want {Name:%q Age:%d}", dst, wantName, wantAge)
		}
		if expectBodyPreserved && dst.Age != 7 {
			t.Fatalf("dst.Age = %d, want preserved age 7", dst.Age)
		}
	}
}

func fuzzHeaderContracts(t *testing.T, variant uint8, headerKey, requestID, retryRaw string) {
	t.Helper()

	type request struct {
		RequestID string `header:"x-request-id"`
		Retry     int    `header:"x-retry"`
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header = http.Header{
		headerKey: {requestID},
		"x-retry": {retryRaw},
	}
	if variant%2 == 1 {
		req.Header.Add(headerKey, requestID+"-2")
	}

	dst := request{
		RequestID: "existing",
		Retry:     7,
	}
	err := BindHeaders(req, &dst)

	normalizedKey := textproto.CanonicalMIMEHeaderKey(strings.TrimSpace(headerKey))
	wantRetry, parseErr := strconv.Atoi(retryRaw)
	if variant%2 == 1 && normalizedKey == "X-Request-Id" {
		violations := assertViolations(t, err)
		if !containsViolation(violations, Violation{Field: "X-Request-Id", In: ViolationInHeader, Code: ViolationCodeMultiple}) {
			t.Fatalf("violations = %#v, want repeated header violation", violations)
		}
		if parseErr == nil && len(violations) != 1 {
			t.Fatalf("violations = %#v, want only repeated header violation", violations)
		}
		if parseErr != nil && !containsViolation(violations, Violation{Field: "X-Retry", In: ViolationInHeader, Code: ViolationCodeType}) {
			t.Fatalf("violations = %#v, want retry type violation when retry is invalid", violations)
		}
		if dst.RequestID != "existing" || dst.Retry != 7 {
			t.Fatalf("dst = %#v, want preserved destination on repeated header failure", dst)
		}
		return
	}

	if parseErr != nil {
		violation := assertSingleViolation(t, err)
		if violation.Field != "X-Retry" || violation.In != ViolationInHeader || violation.Code != ViolationCodeType {
			t.Fatalf("violation = %#v, want type header violation", violation)
		}
		if dst.RequestID != "existing" || dst.Retry != 7 {
			t.Fatalf("dst = %#v, want preserved destination on header type failure", dst)
		}
		return
	}

	if err != nil {
		t.Fatalf("BindHeaders() error = %v", err)
	}
	if normalizedKey == "X-Request-Id" {
		if dst.RequestID != requestID {
			t.Fatalf("request id = %q, want %q", dst.RequestID, requestID)
		}
	} else if dst.RequestID != "existing" {
		t.Fatalf("request id = %q, want preserved existing value", dst.RequestID)
	}
	if dst.Retry != wantRetry {
		t.Fatalf("retry = %d, want %d", dst.Retry, wantRetry)
	}
}

func fuzzBodyPayload(variant uint8, name, ageText string) (payload, wantName string, wantAge int, expectAgePreserved bool) {
	quotedName := fuzzJSONString(name)
	quotedAge := fuzzJSONString(ageText)
	normalizedName := decodeFuzzJSONString(quotedName)
	validAge := len(ageText)

	switch variant % 7 {
	case 0:
		return "", "existing", 7, true
	case 1:
		return fmt.Sprintf(`{"name":%s,"age":%s}`, quotedName, quotedAge), "existing", 7, true
	case 2:
		return `{"name":`, "existing", 7, true
	case 3:
		return fmt.Sprintf(`{"name":%s}{"age":1}`, quotedName), "existing", 7, true
	case 4:
		return fmt.Sprintf(`{"name":%s,"age":%d}`, quotedName, validAge), normalizedName, validAge, false
	case 5:
		return fmt.Sprintf("{\"name\":%s} \n\t ", quotedName), normalizedName, 7, true
	default:
		return fmt.Sprintf(`{"name":%s,"extra":1}`, quotedName), normalizedName, 7, true
	}
}

func fuzzJSONString(value string) string {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(data)
}

func decodeFuzzJSONString(value string) string {
	var decoded string
	if err := json.Unmarshal([]byte(value), &decoded); err != nil {
		panic(err)
	}
	return decoded
}

func emptyBodyAllowsContentType(contentType string) bool {
	return validateJSONContentType(contentType) == nil
}

func nonEmptyBodyAllowsContentType(contentType string) bool {
	contentType = strings.TrimSpace(contentType)
	if contentType == "" {
		return false
	}
	return validateJSONContentType(contentType) == nil
}

func containsViolation(violations []Violation, want Violation) bool {
	for _, violation := range violations {
		if violation.Field == want.Field && violation.In == want.In && violation.Code == want.Code {
			return true
		}
	}
	return false
}
