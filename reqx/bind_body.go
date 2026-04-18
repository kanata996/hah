package reqx

import (
	"bytes"
	"context"
	"encoding"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/kanata996/hah/errx"
)

const defaultMaxBodyBytes int64 = 1 << 20

const (
	CodeInvalidJSON          = "invalid_json"
	CodeUnsupportedMediaType = "unsupported_media_type"
	CodeRequestTooLarge      = "request_too_large"
)

const mimeApplicationJSON = "application/json"

var (
	bodyTimeType            = reflect.TypeOf(time.Time{})
	bodyRawMessageType      = reflect.TypeOf(json.RawMessage{})
	bodyJSONUnmarshalerType = reflect.TypeOf((*json.Unmarshaler)(nil)).Elem()
	bodyTextUnmarshalType   = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()
)

func BindBody(r *http.Request, target any) error {
	if err := validateBindInputs(r, target); err != nil {
		return err
	}
	if err := validateBindBodyTarget(reflect.TypeOf(target)); err != nil {
		return err
	}

	return bindBody(r, reflect.ValueOf(target).Elem())
}

func bindBody(r *http.Request, dst reflect.Value) error {
	hasBody, err := hasRequestBody(r)
	if err != nil {
		return err
	}
	if !hasBody {
		return nil
	}

	if mediaType, err := bodyMediaType(r); err != nil || mediaType != mimeApplicationJSON {
		return unsupportedMediaTypeError()
	}

	body, err := readBody(r.Body)
	if err != nil {
		if errors.Is(err, errRequestTooLarge) {
			return requestTooLargeError()
		}
		return err
	}

	if err := validateJSONDocument(body); err != nil {
		return mapJSONBodyDecodeError(err)
	}

	temp := reflect.New(dst.Type()).Elem()
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(temp.Addr().Interface()); err != nil {
		return mapJSONBodyDecodeError(err)
	}

	dst.Set(temp)
	return nil
}

func validateBindBodyTarget(targetType reflect.Type) error {
	if targetType.Kind() != reflect.Pointer || targetType.Elem().Kind() != reflect.Struct {
		return usageErrorf("destination must point to struct")
	}
	if disallowedBindBodyTargetDecoder(targetType.Elem()) {
		return usageErrorf("unsupported body field type")
	}
	return validateBindBodyStructType(targetType.Elem())
}

func validateBindBodyStructType(t reflect.Type) error {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		if tag := field.Tag.Get("json"); tag == "-" {
			continue
		}
		if err := validateBindBodyFieldType(field.Type); err != nil {
			return err
		}
	}
	return nil
}

func validateBindBodyFieldType(t reflect.Type) error {
	base := t
	if base.Kind() == reflect.Pointer {
		base = base.Elem()
		if base.Kind() == reflect.Pointer {
			return usageErrorf("unsupported body field type")
		}
	}

	return validateBindBodyNonPointerType(base, true)
}

func validateBindBodyNonPointerType(base reflect.Type, allowSlice bool) error {
	if base == bodyTimeType {
		return nil
	}
	if base == bodyRawMessageType {
		return usageErrorf("unsupported body field type")
	}
	if disallowedBindBodyDecoder(base) {
		return usageErrorf("unsupported body field type")
	}

	switch base.Kind() {
	case reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64:
		return nil
	case reflect.Struct:
		return validateBindBodyStructType(base)
	case reflect.Slice:
		if !allowSlice {
			return usageErrorf("unsupported body field type")
		}
		return validateBindBodySliceElementType(base.Elem())
	default:
		return usageErrorf("unsupported body field type")
	}
}

func validateBindBodySliceElementType(t reflect.Type) error {
	if t.Kind() == reflect.Pointer {
		return usageErrorf("unsupported body field type")
	}
	return validateBindBodyNonPointerType(t, false)
}

func disallowedBindBodyDecoder(t reflect.Type) bool {
	ptr := reflect.PointerTo(t)
	if t != bodyTimeType {
		if ptr.Implements(bodyJSONUnmarshalerType) || ptr.Implements(bodyTextUnmarshalType) {
			return true
		}
	}
	return false
}

func disallowedBindBodyTargetDecoder(t reflect.Type) bool {
	ptr := reflect.PointerTo(t)
	return ptr.Implements(bodyJSONUnmarshalerType) || ptr.Implements(bodyTextUnmarshalType)
}

func bodyMediaType(r *http.Request) (string, error) {
	contentType := strings.TrimSpace(r.Header.Get("Content-Type"))
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

func mapJSONBodyDecodeError(err error) error {
	var invalidUnmarshalErr *json.InvalidUnmarshalError
	if errors.As(err, &invalidUnmarshalErr) {
		return err
	}

	return errx.NewHTTPErrorWithCause(
		http.StatusBadRequest,
		CodeInvalidJSON,
		"request body must be valid JSON",
		err,
	)
}

var errRequestTooLarge = errors.New("reqx: request body too large")
var errDuplicateJSONObjectKey = errors.New("reqx: duplicate json object key")
var errExpectedJSONObject = errors.New("reqx: top-level JSON value must be object")

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

func validateJSONDocument(body []byte) error {
	dec := json.NewDecoder(bytes.NewReader(body))

	tok, err := dec.Token()
	if err != nil {
		return err
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '{' {
		return errExpectedJSONObject
	}
	if err := consumeJSONObject(dec); err != nil {
		return err
	}

	if _, err := dec.Token(); err != io.EOF {
		if err == nil {
			return errors.New("request body must contain exactly one JSON value")
		}
		return err
	}
	return nil
}

func consumeJSONObject(dec *json.Decoder) error {
	seen := map[string]struct{}{}
	for dec.More() {
		keyToken, err := dec.Token()
		if err != nil {
			return err
		}
		key := keyToken.(string)
		if _, exists := seen[key]; exists {
			return errDuplicateJSONObjectKey
		}
		seen[key] = struct{}{}
		if err := consumeJSONValue(dec); err != nil {
			return err
		}
	}
	_, err := dec.Token()
	return err
}

func consumeJSONArray(dec *json.Decoder) error {
	for dec.More() {
		if err := consumeJSONValue(dec); err != nil {
			return err
		}
	}
	_, err := dec.Token()
	return err
}

func consumeJSONValue(dec *json.Decoder) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		return consumeJSONObject(dec)
	case '[':
		return consumeJSONArray(dec)
	default:
		return errors.New("invalid JSON delimiter")
	}
}

type requestBodyProbeKey struct{}

type requestBodyProbeState struct {
	has bool
	err error
}

type replayReadCloser struct {
	io.Reader
	io.Closer
}

func hasRequestBody(r *http.Request) (bool, error) {
	if r == nil || r.Body == nil {
		return false, nil
	}

	if cached, ok := r.Context().Value(requestBodyProbeKey{}).(requestBodyProbeState); ok {
		return cached.has, cached.err
	}

	has, err := detectRequestBody(r)
	*r = *r.WithContext(context.WithValue(r.Context(), requestBodyProbeKey{}, requestBodyProbeState{
		has: has,
		err: err,
	}))
	return has, err
}

func detectRequestBody(r *http.Request) (bool, error) {
	body := r.Body
	var prefix [1]byte
	n, err := body.Read(prefix[:])
	if err != nil && err != io.EOF {
		if n > 0 {
			r.Body = &replayReadCloser{
				Reader: io.MultiReader(bytes.NewReader(prefix[:n]), body),
				Closer: body,
			}
		}
		return false, err
	}

	if n == 0 {
		return false, nil
	}

	r.Body = &replayReadCloser{
		Reader: io.MultiReader(bytes.NewReader(prefix[:n]), body),
		Closer: body,
	}
	return true, nil
}
