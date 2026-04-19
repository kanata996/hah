package reqx

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"reflect"
	"strings"

	"github.com/kanata996/hah/internal/errx"
)

const defaultMaxBodyBytes int64 = 1 << 20

const (
	CodeInvalidJSON          = "invalid_json"
	CodeUnsupportedMediaType = "unsupported_media_type"
	CodeRequestTooLarge      = "request_too_large"
)

const mimeApplicationJSON = "application/json"

func BindBody(r *http.Request, target any) error {
	if err := validateBindInputs(r, target); err != nil {
		return err
	}
	if err := validateBindBodyTarget(reflect.TypeOf(target)); err != nil {
		return err
	}

	body, err := readRequestBody(r)
	if err != nil {
		if err == errRequestTooLarge {
			return requestTooLargeError()
		}
		return err
	}
	if len(body) == 0 {
		return nil
	}

	if mediaType, err := bodyMediaType(r); err != nil || mediaType != mimeApplicationJSON {
		return unsupportedMediaTypeError()
	}
	if !looksLikeJSONObject(body) {
		return invalidJSONError(nil)
	}

	temp := reflect.New(reflect.TypeOf(target).Elem())
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(temp.Interface()); err != nil {
		return invalidJSONError(err)
	}

	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return invalidJSONError(errors.New("request body must contain exactly one JSON value"))
		}
		return invalidJSONError(err)
	}

	reflect.ValueOf(target).Elem().Set(temp.Elem())
	return nil
}

func validateBindBodyTarget(targetType reflect.Type) error {
	if targetType.Kind() != reflect.Pointer || targetType.Elem().Kind() != reflect.Struct {
		return usageErrorf("destination must point to struct")
	}
	return nil
}

func readRequestBody(r *http.Request) ([]byte, error) {
	if r == nil || r.Body == nil {
		return nil, nil
	}
	return readBody(r.Body)
}

func looksLikeJSONObject(body []byte) bool {
	trimmed := bytes.TrimSpace(body)
	return len(trimmed) > 0 && trimmed[0] == '{'
}

func bodyMediaType(r *http.Request) (string, error) {
	contentTypes := r.Header.Values("Content-Type")
	if len(contentTypes) > 1 {
		return "", errDuplicateContentType
	}

	contentType := ""
	if len(contentTypes) == 1 {
		contentType = strings.TrimSpace(contentTypes[0])
	}
	if contentType == "" {
		return "", nil
	}

	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(mediaType), nil
}

func unsupportedMediaTypeError() error {
	return errx.NewHTTPError(
		http.StatusUnsupportedMediaType,
		CodeUnsupportedMediaType,
		"Content-Type must be application/json",
	)
}

func requestTooLargeError() error {
	return errx.NewHTTPError(
		http.StatusRequestEntityTooLarge,
		CodeRequestTooLarge,
		"request body is too large",
	)
}

func invalidJSONError(cause error) error {
	return errx.NewHTTPErrorWithCause(
		http.StatusBadRequest,
		CodeInvalidJSON,
		"request body must be valid JSON",
		cause,
	)
}

var errRequestTooLarge = errors.New("reqx: request body too large")
var errDuplicateContentType = errors.New("reqx: multiple Content-Type values")

func readBody(body io.ReadCloser) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, defaultMaxBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > defaultMaxBodyBytes {
		return nil, errRequestTooLarge
	}
	return data, nil
}
