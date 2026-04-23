package resp

import "net/http"

func writeResponse(w http.ResponseWriter, response *Response) error {
	if w == nil {
		return errNilResponseWriter
	}

	body, err := encodeJSON(response)
	if err != nil {
		return err
	}
	return writeJSONBytes(w, response.Status, body)
}

func writeSuccess(w http.ResponseWriter, status int, data any) error {
	response, err := SuccessResponse(status, data)
	if err != nil {
		return err
	}
	return writeResponse(w, response)
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

// WriteError 是 HTTP 错误写回的统一入口。
func WriteError(w http.ResponseWriter, err error, code ...int) error {
	if err == nil {
		return nil
	}

	if w == nil {
		return errNilResponseWriter
	}

	response, buildErr := ErrorResponse(err, code...)
	if buildErr != nil {
		return buildErr
	}

	return writeResponse(w, response)
}
