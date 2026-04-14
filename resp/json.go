package resp

import (
	"fmt"
	"net/http"
)

// JSON 写出 JSON 响应。
func JSON(w http.ResponseWriter, status int, data any) error {
	return writeJSON(w, status, data)
}

// JSONBlob 直接写出原始 JSON 字节。
// 调用方需要自行保证 body 是合法 JSON。
func JSONBlob(w http.ResponseWriter, status int, body []byte) error {
	return writeJSONBytesWithContentType(w, status, jsonContentType, body)
}

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
	return writeStatus(w, http.StatusNoContent)
}

// writeJSON 是通用 JSON 成功响应的核心路径。
// 它先校验响应边界，再编码 payload，最后写出已准备好的 JSON 字节，
// 避免无效状态码或空 writer 触发多余编码，也避免底层写回重复做同一轮校验。
func writeJSON(w http.ResponseWriter, status int, data any) error {
	if err := validateJSONBodyWrite(w, status); err != nil {
		return err
	}

	body, err := encodeJSON(data)
	if err != nil {
		return err
	}
	return writePreparedJSONBytes(w, status, jsonContentType, body)
}

// writeSuccess 是 OK / Created 这类显式成功响应的核心路径。
// 相比通用 JSON 写回，它额外要求状态码必须是非错误状态，且 payload 不能编码为 JSON null。
func writeSuccess(w http.ResponseWriter, status int, data any) error {
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

	return writePreparedJSONBytes(w, status, jsonContentType, dataJSON)
}

// validateJSONBodyWrite 统一校验“会写 JSON body”的响应边界。
// 该校验独立出来后，上层 writer 可以在编码前提前失败，避免无意义编码与重复校验。
func validateJSONBodyWrite(w http.ResponseWriter, status int) error {
	if w == nil {
		return errNilResponseWriter
	}
	if err := validateHTTPStatus(status); err != nil {
		return err
	}
	if err := validateStatusAllowsBody(status, "JSON body writers"); err != nil {
		return err
	}
	return nil
}

// writePreparedJSONBytes 假定 writer 与 status 已完成校验，直接执行头和 body 的实际写回。
func writePreparedJSONBytes(w http.ResponseWriter, status int, contentType string, body []byte) error {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	if _, err := w.Write(body); err != nil {
		return &responseWriteError{
			cause:           err,
			responseStarted: true,
		}
	}
	return nil
}
