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

	body, err := encodeJSON(data)
	if err != nil {
		return err
	}
	return writeJSONBytes(w, status, body)
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

// writeJSONBytes 假定 writer、status 与 body 已校验完成，直接写出标准 JSON 响应。
func writeJSONBytes(w http.ResponseWriter, status int, body []byte) error {
	header := w.Header()
	// 清掉外部预设的旧长度，让 net/http 按本次实际 body 重新决定最终 Content-Length。
	header.Del("Content-Length")
	header.Set("Content-Type", jsonContentType)
	w.WriteHeader(status)
	if _, err := w.Write(body); err != nil {
		return fmt.Errorf("resp: write response failed: %w", err)
	}
	return nil
}
