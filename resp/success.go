package resp

import "net/http"

// OK 写出 200 JSON 成功响应。
func OK(w http.ResponseWriter, data any) error {
	return writeSuccess(w, http.StatusOK, data)
}

// Created 写出 201 JSON 成功响应。
func Created(w http.ResponseWriter, data any) error {
	return writeSuccess(w, http.StatusCreated, data)
}

// NoContent 写出 204 响应且不包含响应体。
func NoContent(w http.ResponseWriter) error {
	if isNilResponseWriter(w) {
		return errNilResponseWriter
	}
	header := w.Header()
	header.Del("Content-Type")
	header.Del("Content-Length")
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// writeSuccess 是 OK / Created 这类显式成功响应的核心路径。
func writeSuccess(w http.ResponseWriter, status int, data any) error {
	return writeJSON(w, status, data)
}
