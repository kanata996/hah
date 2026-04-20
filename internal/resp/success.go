package resp

import "net/http"

const (
	successTopCode = 0
	successMessage = "success"
)

// OK 写出 200 JSON 成功响应。
func OK(w http.ResponseWriter, data any) error {
	return writeSuccess(w, http.StatusOK, data)
}

// Created 写出 201 JSON 成功响应。
func Created(w http.ResponseWriter, data any) error {
	return writeSuccess(w, http.StatusCreated, data)
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

	return writePreparedJSONBytes(w, status, jsonContentType, body)
}
