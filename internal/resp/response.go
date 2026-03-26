package resp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details []any  `json:"details"`
}

type ErrorPayload struct {
	Status  int
	Code    string
	Message string
	Details []any
}

type ErrorWriteDegraded struct {
	Cause                   error
	PreservedPublicResponse bool
}

func (e *ErrorWriteDegraded) Error() string {
	if e == nil || e.Cause == nil {
		return "hah: error response details were dropped"
	}
	return "hah: error response details were dropped: " + e.Cause.Error()
}

func (e *ErrorWriteDegraded) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func WriteSuccess(w http.ResponseWriter, status int, data any, meta any, includeMeta bool) error {
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
	return WriteJSONBytes(w, status, body)
}

func WriteEmpty(w http.ResponseWriter, status int) error {
	if err := ValidateSuccessStatus(status); err != nil {
		return err
	}

	w.WriteHeader(status)
	return nil
}

func WriteErrorPayload(w http.ResponseWriter, payload ErrorPayload) error {
	body, err := marshalErrorEnvelope(payload.Code, payload.Message, normalizeDetails(payload.Details))
	if err != nil {
		degradedBody, _ := marshalErrorEnvelope(payload.Code, payload.Message, []any{})
		if writeErr := WriteJSONBytes(w, payload.Status, degradedBody); writeErr != nil {
			return errors.Join(&ErrorWriteDegraded{Cause: err}, writeErr)
		}
		return &ErrorWriteDegraded{
			Cause:                   err,
			PreservedPublicResponse: true,
		}
	}

	if writeErr := WriteJSONBytes(w, payload.Status, body); writeErr != nil {
		return writeErr
	}

	return nil
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

func WriteJSONBytes(w http.ResponseWriter, status int, body []byte) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, err := w.Write(body)
	return err
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
