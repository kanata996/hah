package render

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

type ErrorPayload struct {
	Status  int
	Code    string
	Message string
	Details []any
}

func Render(w http.ResponseWriter, r *http.Request, data any) error {
	return RenderWithMeta(w, r, data, nil)
}

func RenderWithMeta(w http.ResponseWriter, r *http.Request, data any, meta any) error {
	status := statusOrDefault(r, http.StatusOK)
	return WriteSuccess(w, r, status, data, meta, meta != nil)
}

func RenderEmpty(w http.ResponseWriter, r *http.Request, status int) error {
	if status == 0 {
		status = statusOrDefault(r, http.StatusNoContent)
	}
	return WriteEmpty(w, r, status)
}

func RenderErrorPayload(w http.ResponseWriter, r *http.Request, payload ErrorPayload) error {
	body, err := marshalErrorEnvelope(payload.Code, payload.Message, normalizeDetails(payload.Details))
	if err != nil {
		body, _ = marshalErrorEnvelope(payload.Code, payload.Message, []any{})
	}

	return WriteJSONBytes(w, r, payload.Status, body, "application/json")
}

func WriteSuccess(w http.ResponseWriter, r *http.Request, status int, data any, meta any, includeMeta bool) error {
	if err := ValidateSuccessBodyStatus(status); err != nil {
		return err
	}

	dataJSON, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if isJSONNullBytes(dataJSON) {
		return fmt.Errorf("hah: data must exist and must not encode to null")
	}

	var metaJSON []byte

	if includeMeta {
		metaJSON, err = json.Marshal(meta)
		if err != nil {
			return err
		}
		if !isJSONNullBytes(metaJSON) {
			if !isJSONObjectBytes(metaJSON) {
				return fmt.Errorf("hah: meta must encode as a JSON object")
			}
		} else {
			metaJSON = nil
		}
	}

	body := buildSuccessBody(dataJSON, metaJSON)
	return WriteJSONBytes(w, r, status, body, "application/json")
}

func WriteEmpty(w http.ResponseWriter, r *http.Request, status int) error {
	if err := ValidateSuccessStatus(status); err != nil {
		return err
	}

	MarkResponseStarted(r)
	contentType := contentTypeOrDefault(r, "")
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.WriteHeader(status)
	return nil
}

func WriteJSONBytes(w http.ResponseWriter, r *http.Request, status int, body []byte, fallbackContentType string) error {
	MarkResponseStarted(r)
	w.Header().Set("Content-Type", contentTypeOrDefault(r, fallbackContentType))
	w.WriteHeader(status)
	_, err := w.Write(body)
	return err
}

func ValidateSuccessBodyStatus(status int) error {
	if err := ValidateSuccessStatus(status); err != nil {
		return err
	}
	if status < http.StatusOK {
		return fmt.Errorf("hah: success writers with a body cannot use informational status %d", status)
	}
	switch status {
	case http.StatusNoContent, http.StatusResetContent, http.StatusNotModified:
		return fmt.Errorf("hah: success writers with a body cannot use bodyless status %d", status)
	}
	return nil
}

func ValidateSuccessStatus(status int) error {
	if status >= 400 {
		return fmt.Errorf("hah: success writers cannot use error status %d", status)
	}
	if status < 100 {
		return fmt.Errorf("hah: invalid HTTP status %d", status)
	}
	return nil
}

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details []any  `json:"details"`
}

func marshalErrorEnvelope(code, message string, details []any) ([]byte, error) {
	return json.Marshal(errorEnvelope{
		Error: errorBody{
			Code:    code,
			Message: message,
			Details: details,
		},
	})
}

func normalizeDetails(details []any) []any {
	if len(details) == 0 {
		return []any{}
	}
	return details
}

func buildSuccessBody(dataJSON []byte, metaJSON []byte) []byte {
	body := make([]byte, 0, len(dataJSON)+len(metaJSON)+24)
	body = append(body, `{"data":`...)
	body = append(body, dataJSON...)
	if len(metaJSON) > 0 {
		body = append(body, `,"meta":`...)
		body = append(body, metaJSON...)
	}
	body = append(body, '}')
	return body
}

func isJSONNullBytes(body []byte) bool {
	return bytes.Equal(bytes.TrimSpace(body), []byte("null"))
}

func isJSONObjectBytes(body []byte) bool {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return false
	}
	if trimmed[0] != '{' || trimmed[len(trimmed)-1] != '}' {
		return false
	}
	return true
}
