package resp

import (
	"bytes"
	"fmt"
	"net/http"
)

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
	if w == nil {
		return errNilResponseWriter
	}
	header := w.Header()
	header.Del("Content-Type")
	header.Del("Content-Length")
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// writeSuccess 是 OK / Created 这类显式成功响应的核心路径。
// 相比通用 JSON 写回，它额外要求 payload 不能编码为 JSON null。
func writeSuccess(w http.ResponseWriter, status int, data any) error {
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

	return writePreparedJSONBytes(w, status, jsonContentType, dataJSON)
}

// isJSONNullBytes 判断一段 JSON 字节在去掉首尾空白后是否等于 null。
func isJSONNullBytes(body []byte) bool {
	return bytes.Equal(bytes.TrimSpace(body), []byte("null"))
}
