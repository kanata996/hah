package reqx

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
)

var errRequestTooLarge = errors.New("reqx: request body too large")

type bodyBindMode struct {
	ignoreEmptyBody            bool
	validateContentTypeOnEmpty bool
}

func BindBody[T any](r *http.Request, dst *T, opts ...BindBodyOption) error {
	if r == nil {
		return errorsf("request must not be nil")
	}

	cfg := applyBindOptions(opts...)
	return bindIntoClone(dst, func(bound *T) error {
		return bindJSONWithConfig(r, bound, cfg.body, bodyBindMode{
			validateContentTypeOnEmpty: true,
		})
	})
}

func bindJSONWithConfig[T any](r *http.Request, dst *T, cfg bindBodyConfig, mode bodyBindMode) error {
	body, err := readBody(r.Body, cfg.maxBodyBytes)
	if err != nil {
		if errors.Is(err, errRequestTooLarge) {
			return requestTooLargeError()
		}
		return err
	}
	if len(bytes.TrimSpace(body)) == 0 {
		if mode.validateContentTypeOnEmpty {
			if err := validateJSONContentType(r.Header.Get("Content-Type")); err != nil {
				return err
			}
		}
		if cfg.allowEmptyBody || mode.ignoreEmptyBody {
			return nil
		}
		return emptyBodyError()
	}

	contentType := strings.TrimSpace(r.Header.Get("Content-Type"))
	if contentType == "" {
		return unsupportedMediaTypeError()
	}
	if err := validateJSONContentType(contentType); err != nil {
		return err
	}

	dec := json.NewDecoder(bytes.NewReader(body))
	if !cfg.allowUnknownFields {
		dec.DisallowUnknownFields()
	}

	if err := dec.Decode(dst); err != nil {
		return mapDecodeError(err)
	}

	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return invalidJSONError("request body must contain a single JSON value")
	}

	return nil
}

func readBody(body io.ReadCloser, maxBytes int64) ([]byte, error) {
	if body == nil {
		return nil, nil
	}
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBodyBytes
	}

	data, err := io.ReadAll(io.LimitReader(body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, errRequestTooLarge
	}
	return data, nil
}

func validateJSONContentType(contentType string) error {
	contentType = strings.TrimSpace(contentType)
	if contentType == "" {
		return nil
	}

	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return unsupportedMediaTypeError()
	}
	if isSupportedJSONMediaType(mediaType) {
		return nil
	}
	return unsupportedMediaTypeError()
}

func isSupportedJSONMediaType(mediaType string) bool {
	if mediaType == "application/json" {
		return true
	}
	if !strings.HasPrefix(mediaType, "application/") || !strings.HasSuffix(mediaType, "+json") {
		return false
	}

	subtype := strings.TrimPrefix(mediaType, "application/")
	return len(subtype) > len("+json")
}

func mapDecodeError(err error) error {
	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		return invalidJSONError("request body must be valid JSON")
	}

	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		detail := "must be " + describeJSONType(typeErr.Type)
		return invalidFieldError(newViolation(typeErr.Field, ViolationInBody, ViolationCodeType, detail))
	}

	var invalidUnmarshalErr *json.InvalidUnmarshalError
	if errors.As(err, &invalidUnmarshalErr) {
		return err
	}

	if errors.Is(err, io.EOF) {
		return emptyBodyError()
	}

	if field, ok := parseUnknownField(err); ok {
		return invalidFieldError(newViolation(field, ViolationInBody, ViolationCodeUnknown, violationDetailUnknownField))
	}

	return invalidJSONError("request body must be valid JSON")
}

func parseUnknownField(err error) (string, bool) {
	const prefix = `json: unknown field "`

	message := err.Error()
	if !strings.HasPrefix(message, prefix) || !strings.HasSuffix(message, `"`) {
		return "", false
	}
	return strings.TrimSuffix(strings.TrimPrefix(message, prefix), `"`), true
}
