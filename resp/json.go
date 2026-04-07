package resp

import (
	"fmt"
	"net/http"
)

// JSON 按 Echo Response 的核心能力写出 JSON 响应。
// 当请求 URL 带有 ?pretty 时，会自动输出 pretty JSON。
func JSON(w http.ResponseWriter, r *http.Request, status int, data any) error {
	return writeJSON(w, status, data, jsonIndent(r))
}

// JSONPretty 使用指定缩进写出 pretty JSON。
func JSONPretty(w http.ResponseWriter, _ *http.Request, status int, data any, indent string) error {
	return writeJSON(w, status, data, indent)
}

// JSONBlob 直接写出原始 JSON 字节。
// 调用方需要自行保证 body 是合法 JSON。
func JSONBlob(w http.ResponseWriter, _ *http.Request, status int, body []byte) error {
	return writeJSONBytes(w, status, body)
}

// OK 写出 200 JSON 成功响应。
func OK(w http.ResponseWriter, r *http.Request, data any) error {
	return writeSuccess(w, r, http.StatusOK, data)
}

// Created 写出 201 JSON 成功响应。
func Created(w http.ResponseWriter, r *http.Request, data any) error {
	return writeSuccess(w, r, http.StatusCreated, data)
}

// NoContent 写出 204 响应且不包含响应体。
func NoContent(w http.ResponseWriter, _ *http.Request) error {
	return writeStatus(w, http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, data any, indent string) error {
	if w == nil {
		return errNilResponseWriter
	}
	if err := validateHTTPStatus(status); err != nil {
		return err
	}
	if err := validateStatusAllowsBody(status, "JSON body writers"); err != nil {
		return err
	}

	body, err := encodeJSON(data, indent)
	if err != nil {
		return err
	}
	return writeJSONBytes(w, status, body)
}

func writeSuccess(w http.ResponseWriter, r *http.Request, status int, data any) error {
	if err := validateHTTPStatus(status); err != nil {
		return err
	}
	if status > 399 {
		return fmt.Errorf("resp: invalid success status %d", status)
	}
	if err := validateStatusAllowsBody(status, "success writers with a body"); err != nil {
		return err
	}
	if w == nil {
		return errNilResponseWriter
	}

	dataJSON, err := encodeJSON(data, jsonIndent(r))
	if err != nil {
		return err
	}
	if isJSONNullBytes(dataJSON) {
		return fmt.Errorf("resp: data must exist and must not encode to null")
	}

	return writeJSONBytes(w, status, dataJSON)
}

func jsonIndent(r *http.Request) string {
	if r == nil || r.URL == nil {
		return ""
	}
	_, pretty := r.URL.Query()["pretty"]
	if pretty {
		return defaultJSONIndent
	}
	return ""
}
