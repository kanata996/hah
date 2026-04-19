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
const maxConsecutiveEmptyBodyReads = 100

const (
	CodeInvalidJSON          = "invalid_json"
	CodeUnsupportedMediaType = "unsupported_media_type"
	CodeRequestTooLarge      = "request_too_large"
)

const mimeApplicationJSON = "application/json"

type replayReadCloser struct {
	io.Reader
	io.Closer
}

func BindBody(r *http.Request, target any) error {
	if err := validateBindInputs(r, target); err != nil {
		return err
	}
	if err := validateBindBodyTarget(reflect.TypeOf(target)); err != nil {
		return err
	}

	hasBody, err := requestHasBody(r)
	if err != nil {
		if errors.Is(err, errRequestTooLarge) {
			return requestTooLargeError()
		}
		return err
	}
	if !hasBody {
		return nil
	}

	if mediaType, err := bodyMediaType(r); err != nil || mediaType != mimeApplicationJSON {
		return unsupportedMediaTypeError()
	}

	body, err := readRequestBody(r)
	if err != nil {
		if errors.Is(err, errRequestTooLarge) {
			return requestTooLargeError()
		}
		return err
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

func requestHasBody(r *http.Request) (bool, error) {
	if r == nil || r.Body == nil {
		return false, nil
	}

	var prefix [1]byte
	n, err := readWithProgress(r.Body, prefix[:])
	if err != nil && err != io.EOF {
		if n > 0 {
			r.Body = &replayReadCloser{
				Reader: io.MultiReader(bytes.NewReader(prefix[:n]), r.Body),
				Closer: r.Body,
			}
		}
		return false, err
	}

	if n == 0 {
		return false, nil
	}

	r.Body = &replayReadCloser{
		Reader: io.MultiReader(bytes.NewReader(prefix[:n]), r.Body),
		Closer: r.Body,
	}
	return true, nil
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
	data, err := io.ReadAll(io.LimitReader(newProgressReader(body), defaultMaxBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > defaultMaxBodyBytes {
		return nil, errRequestTooLarge
	}
	return data, nil
}

func readWithProgress(r io.Reader, p []byte) (int, error) {
	progressReader := newProgressReader(r)
	for {
		n, err := progressReader.Read(p)
		if n != 0 || err != nil {
			return n, err
		}
	}
}

type progressReader struct {
	reader     io.Reader
	emptyReads int
}

func newProgressReader(r io.Reader) *progressReader {
	return &progressReader{reader: r}
}

func (r *progressReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n != 0 || err != nil {
		r.emptyReads = 0
		return n, err
	}

	r.emptyReads++
	if r.emptyReads >= maxConsecutiveEmptyBodyReads {
		return 0, io.ErrNoProgress
	}
	return 0, nil
}
