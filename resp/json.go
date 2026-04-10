package resp

import (
	"fmt"
	"net/http"
)

// JSON 写出 JSON 响应。
func JSON(w http.ResponseWriter, r *http.Request, status int, data any) error {
	return writeJSON(w, r, status, data)
}

// JSONBlob 直接写出原始 JSON 字节。
// 调用方需要自行保证 body 是合法 JSON。
func JSONBlob(w http.ResponseWriter, r *http.Request, status int, body []byte) error {
	return writeJSONBytesForRequest(w, r, status, jsonContentType, body)
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

func writeJSON(w http.ResponseWriter, r *http.Request, status int, data any) error {
	if w == nil {
		return errNilResponseWriter
	}
	if err := validateHTTPStatus(status); err != nil {
		return err
	}
	if err := validateStatusAllowsBody(status, "JSON body writers"); err != nil {
		return err
	}

	body, err := encodeJSON(data)
	if err != nil {
		return err
	}
	return writeJSONBytesForRequest(w, r, status, jsonContentType, body)
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

	dataJSON, err := encodeJSON(data)
	if err != nil {
		return err
	}
	if isJSONNullBytes(dataJSON) {
		return fmt.Errorf("resp: data must exist and must not encode to null")
	}

	return writeJSONBytesForRequest(w, r, status, jsonContentType, dataJSON)
}

func writeJSONBytesForRequest(w http.ResponseWriter, r *http.Request, status int, contentType string, body []byte) error {
	if r != nil && r.Method == http.MethodHead {
		return writeHeadJSONBytes(w, status, contentType, body)
	}
	return writeJSONBytesWithContentType(w, status, contentType, body)
}

func writeHeadJSONBytes(w http.ResponseWriter, status int, contentType string, body []byte) error {
	if w == nil {
		return errNilResponseWriter
	}
	if err := validateHTTPStatus(status); err != nil {
		return err
	}
	if err := validateStatusAllowsBody(status, "JSON body writers"); err != nil {
		return err
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
	w.WriteHeader(status)
	return nil
}
