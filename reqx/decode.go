package reqx

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"reflect"
	"strings"
)

const DefaultMaxBodyBytes int64 = 1 << 20

type decodeConfig struct {
	maxBodyBytes       int64
	allowUnknownFields bool
	allowEmptyBody     bool
}

// DecodeOption customizes JSON decoding behavior.
type DecodeOption func(*decodeConfig)

// WithMaxBodyBytes limits the number of bytes read from the request body.
func WithMaxBodyBytes(limit int64) DecodeOption {
	return func(cfg *decodeConfig) {
		if limit > 0 {
			cfg.maxBodyBytes = limit
		}
	}
}

// AllowUnknownFields disables strict unknown-field rejection.
func AllowUnknownFields() DecodeOption {
	return func(cfg *decodeConfig) {
		cfg.allowUnknownFields = true
	}
}

// AllowEmptyBody permits an empty request body.
func AllowEmptyBody() DecodeOption {
	return func(cfg *decodeConfig) {
		cfg.allowEmptyBody = true
	}
}

// DecodeJSON decodes a JSON request body into dst and returns reqx Problems for
// request-shape failures.
func DecodeJSON[T any](r *http.Request, dst *T, opts ...DecodeOption) error {
	if r == nil {
		return fmt.Errorf("hah/reqx: request must not be nil")
	}
	if dst == nil {
		return fmt.Errorf("hah/reqx: destination must not be nil")
	}

	cfg := decodeConfig{
		maxBodyBytes: DefaultMaxBodyBytes,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	if err := validateJSONContentType(r.Header.Get("Content-Type")); err != nil {
		return err
	}

	body, err := readBody(r.Body, cfg.maxBodyBytes)
	if err != nil {
		if errors.Is(err, errRequestTooLarge) {
			return requestTooLargeError()
		}
		return err
	}
	if len(bytes.TrimSpace(body)) == 0 {
		if cfg.allowEmptyBody {
			return nil
		}
		return emptyBodyError()
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

// DecodeAndValidateJSON decodes a JSON request body, then runs validation.
func DecodeAndValidateJSON[T any](r *http.Request, dst *T, fn ValidateFunc[T], opts ...DecodeOption) error {
	if err := DecodeJSON(r, dst, opts...); err != nil {
		return err
	}
	return Validate(dst, fn)
}

var errRequestTooLarge = errors.New("hah/reqx: request body too large")

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
	if mediaType == "application/json" || strings.HasSuffix(mediaType, "+json") {
		return nil
	}

	return unsupportedMediaTypeError()
}

func mapDecodeError(err error) error {
	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		return invalidJSONError("request body must be valid JSON")
	}

	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		return invalidFieldError(Violation{
			Field:   typeErr.Field,
			Code:    ViolationCodeType,
			Message: "must be " + describeJSONType(typeErr.Type),
		})
	}

	var invalidUnmarshalErr *json.InvalidUnmarshalError
	if errors.As(err, &invalidUnmarshalErr) {
		return err
	}

	if errors.Is(err, io.EOF) {
		return emptyBodyError()
	}

	if field, ok := parseUnknownField(err); ok {
		return invalidFieldError(Violation{
			Field:   field,
			Code:    ViolationCodeUnknown,
			Message: "unknown field",
		})
	}

	return invalidJSONError("request body must be valid JSON")
}

func parseUnknownField(err error) (string, bool) {
	const prefix = `json: unknown field "`

	if err == nil {
		return "", false
	}

	message := err.Error()
	if !strings.HasPrefix(message, prefix) || !strings.HasSuffix(message, `"`) {
		return "", false
	}

	return strings.TrimSuffix(strings.TrimPrefix(message, prefix), `"`), true
}

func describeJSONType(t reflect.Type) string {
	if t == nil {
		return "valid value"
	}

	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	switch t.Kind() {
	case reflect.Bool:
		return "boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return "number"
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return "number"
	case reflect.Float32, reflect.Float64:
		return "number"
	case reflect.String:
		return "string"
	case reflect.Array, reflect.Slice:
		return "array"
	case reflect.Map, reflect.Struct:
		return "object"
	default:
		return t.String()
	}
}

func invalidJSONError(message string) error {
	return NewProblem(http.StatusBadRequest, CodeInvalidJSON, message)
}

func emptyBodyError() error {
	return invalidJSONError("request body must not be empty")
}

func unsupportedMediaTypeError() error {
	return NewProblem(
		http.StatusUnsupportedMediaType,
		CodeUnsupportedMediaType,
		"Content-Type must be application/json",
	)
}

func requestTooLargeError() error {
	return NewProblem(
		http.StatusRequestEntityTooLarge,
		CodeRequestTooLarge,
		"request body is too large",
	)
}
