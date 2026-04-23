package resp

import "net/http"

const (
	successTopCode = 0
	successMessage = "success"
)

type responseEnvelope struct {
	Code    int        `json:"code"`
	Message string     `json:"message"`
	Data    any        `json:"data,omitempty"`
	Error   *errorBody `json:"error,omitempty"`
}

func writeSuccess(w http.ResponseWriter, status int, data any) error {
	if w == nil {
		return errNilResponseWriter
	}

	body, err := encodeJSON(responseEnvelope{
		Code:    successTopCode,
		Message: successMessage,
		Data:    data,
	})
	if err != nil {
		return err
	}
	return writeJSONBytes(w, status, body)
}

// OK 写出 200 JSON 成功响应。
func OK(w http.ResponseWriter, data any) error {
	return writeSuccess(w, http.StatusOK, data)
}

// Accepted 写出 202 JSON 成功响应。
func Accepted(w http.ResponseWriter, data any) error {
	return writeSuccess(w, http.StatusAccepted, data)
}

// Created 写出 201 JSON 成功响应。
func Created(w http.ResponseWriter, data any) error {
	return writeSuccess(w, http.StatusCreated, data)
}

// NoContent 写出 204 无响应体成功响应。
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
