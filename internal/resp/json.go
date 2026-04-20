package resp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

const (
	jsonContentType = "application/json"
)

var errNilResponseWriter = errors.New("resp: response writer is nil")

type responseEnvelope struct {
	Code    int        `json:"code"`
	Message string     `json:"message"`
	Data    any        `json:"data,omitempty"`
	Error   *errorBody `json:"error,omitempty"`
}

// JSON 写出 JSON 响应。
func JSON(w http.ResponseWriter, status int, data any) error {
	if w == nil {
		return errNilResponseWriter
	}
	if status < 100 || status > 999 {
		return fmt.Errorf("resp: invalid HTTP status %d", status)
	}
	if status < 200 || status == http.StatusNoContent || status == http.StatusResetContent || status == http.StatusNotModified {
		return fmt.Errorf("resp: JSON does not support status %d without a response body", status)
	}

	return writeJSONResponse(w, status, data)
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

// writePreparedJSONBytes 假定 writer 与 status 已校验完成，直接写出头和 body。
func writePreparedJSONBytes(w http.ResponseWriter, status int, contentType string, body []byte) error {
	header := w.Header()
	// 清掉外部预设的旧长度，让 net/http 按本次实际 body 重新决定最终 Content-Length。
	header.Del("Content-Length")
	header.Set("Content-Type", contentType)
	w.WriteHeader(status)
	if _, err := w.Write(body); err != nil {
		return fmt.Errorf("resp: write response failed: %w", err)
	}
	return nil
}

func writeJSONResponse(w http.ResponseWriter, status int, payload any) error {
	if w == nil {
		return errNilResponseWriter
	}

	body, err := encodeJSON(payload)
	if err != nil {
		return err
	}
	return writePreparedJSONBytes(w, status, jsonContentType, body)
}
