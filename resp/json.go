package resp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
)

const (
	jsonContentType        = "application/json"
	problemJSONContentType = "application/problem+json"
)

var errNilResponseWriter = errors.New("resp: response writer is nil")

type responseWriteError struct {
	cause error
}

func (e *responseWriteError) Error() string {
	if cause := normalizeWriteErrorCause(e.cause); cause != nil {
		return "resp: write response failed: " + cause.Error()
	}
	return "resp: write response failed"
}

func (e *responseWriteError) Unwrap() error {
	return normalizeWriteErrorCause(e.cause)
}

// JSON 写出 JSON 响应。
func JSON(w http.ResponseWriter, status int, data any) error {
	return writeJSON(w, status, data)
}

// writeJSON 是通用 JSON 成功响应的核心路径。
// 它先校验响应边界，再编码 payload，最后写出已准备好的 JSON 字节，
// 避免无效状态码或空 writer 触发多余编码，也避免底层写回重复做同一轮校验。
func writeJSON(w http.ResponseWriter, status int, data any) error {
	if err := validateJSONBodyWriter(w, status); err != nil {
		return err
	}

	body, err := encodeJSON(data)
	if err != nil {
		return err
	}
	return writePreparedJSONBytes(w, status, jsonContentType, body)
}

// encodeJSON 使用标准库编码 JSON。
// 标准库会保留尾部换行。
func encodeJSON(data any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	if err := enc.Encode(data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// validateJSONBodyWriter 统一校验 JSON body writer 的响应边界。
// 它在编码前提前失败，避免无意义编码与重复校验。
func validateJSONBodyWriter(w http.ResponseWriter, status int) error {
	if w == nil {
		return errNilResponseWriter
	}
	switch {
	case status < 100 || status > 999:
		return fmt.Errorf("resp: invalid HTTP status %d", status)
	}

	switch status {
	case http.StatusOK, http.StatusCreated, http.StatusAccepted:
		return nil
	default:
		return fmt.Errorf("resp: JSON only supports status 200, 201, or 202")
	}
}

func normalizeWriteErrorCause(err error) error {
	if err == nil {
		return nil
	}

	value := reflect.ValueOf(err)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if value.IsNil() {
			return nil
		}
	}
	return err
}

// writePreparedJSONBytes 假定 writer 与 status 已完成校验，直接执行头和 body 的实际写回。
func writePreparedJSONBytes(w http.ResponseWriter, status int, contentType string, body []byte) error {
	header := w.Header()
	// 清掉外部预设的旧长度，让 net/http 按本次实际 body 重新决定最终 Content-Length。
	header.Del("Content-Length")
	header.Set("Content-Type", contentType)
	w.WriteHeader(status)
	if _, err := w.Write(body); err != nil {
		return &responseWriteError{cause: err}
	}
	return nil
}
