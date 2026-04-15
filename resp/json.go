package resp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
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
	if e == nil || e.cause == nil {
		return "resp: write response failed"
	}
	if cause := safeErrorString(e.cause); cause != "" {
		return "resp: write response failed: " + cause
	}
	return "resp: write response failed"
}

func (e *responseWriteError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// JSON 写出 JSON 响应。
func JSON(w http.ResponseWriter, status int, data any) error {
	return writeJSON(w, status, data)
}

// JSONBlob 直接写出原始 JSON 字节。
// 调用方需要自行保证 body 是合法 JSON。
func JSONBlob(w http.ResponseWriter, status int, body []byte) error {
	return writeJSONBytesWithContentType(w, status, jsonContentType, body)
}

// writeJSONBytesWithContentType 以指定 JSON 媒体类型写出原始 JSON 字节切片。
// 调用方需要自行保证 body 已经是合法 JSON。
func writeJSONBytesWithContentType(w http.ResponseWriter, status int, contentType string, body []byte) error {
	if err := validateJSONBodyWrite(w, status); err != nil {
		return err
	}
	return writePreparedJSONBytes(w, status, contentType, body)
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

// encodeJSON 使用标准库编码 JSON。
// 标准库会保留尾部换行。
// 某些自定义 MarshalJSON 实现可能 panic，这里统一恢复为 error，
// 避免成功响应路径反向把 handler 打崩。
func encodeJSON(data any) (body []byte, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			body = nil
			err = fmt.Errorf("resp: encode JSON panicked: %v", recovered)
		}
	}()

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	if err := enc.Encode(data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
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

func validateHTTPStatus(status int) error {
	if status < 100 || status > 999 {
		return fmt.Errorf("resp: invalid HTTP status %d", status)
	}
	return nil
}

func validateStatusAllowsBody(status int, writerName string) error {
	if status < http.StatusOK {
		return fmt.Errorf("resp: %s cannot use informational status %d", writerName, status)
	}
	switch status {
	case http.StatusNoContent, http.StatusResetContent, http.StatusNotModified:
		return fmt.Errorf("resp: %s cannot use bodyless status %d", writerName, status)
	}
	return nil
}

// writePreparedJSONBytes 假定 writer 与 status 已完成校验，直接执行头和 body 的实际写回。
func writePreparedJSONBytes(w http.ResponseWriter, status int, contentType string, body []byte) error {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	if _, err := w.Write(body); err != nil {
		return &responseWriteError{cause: err}
	}
	return nil
}

// safeErrorString 读取错误文本，并对异常 Error() 实现做恢复。
func safeErrorString(err error) (message string) {
	if err == nil {
		return ""
	}
	defer func() {
		if recover() != nil {
			message = "panic calling Error()"
		}
	}()

	return strings.TrimSpace(err.Error())
}
