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

var bindBodyJSONUnmarshalerType = reflect.TypeFor[json.Unmarshaler]()

// 零字节 body 直接 no-op；非空 body 则按 JSON object 输入模型处理，
// 全部成功后再一次性提交到零值临时对象对应的 target。
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

// 这里只接受非 nil 的 *struct，其他形状都属于 usage error。
func validateBindBodyTarget(targetType reflect.Type) error {
	if targetType.Kind() != reflect.Pointer || targetType.Elem().Kind() != reflect.Struct {
		return usageErrorf("destination must point to struct")
	}
	if targetType.Implements(bindBodyJSONUnmarshalerType) {
		return usageErrorf("destination must point to struct without custom UnmarshalJSON")
	}
	return nil
}

// 对 nil body 走空输入语义，其余情况统一交给带大小限制的读取逻辑。
func readRequestBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	return readBody(r.Body)
}

// 在进入 JSON 解码前先拦掉明显不符合“顶层必须是 object”的输入。
func looksLikeJSONObject(body []byte) bool {
	trimmed := bytes.TrimSpace(body)
	return len(trimmed) > 0 && trimmed[0] == '{'
}

// 非空 body 只接受一个 Content-Type，且主媒体类型必须是 application/json。
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

// 统一构造非 JSON body 的稳定客户端输入错误。
func unsupportedMediaTypeError() error {
	return errx.NewHTTPError(
		http.StatusUnsupportedMediaType,
		CodeUnsupportedMediaType,
		"Content-Type must be application/json",
	)
}

// 统一构造超过默认 body 大小上限的稳定客户端输入错误。
func requestTooLargeError() error {
	return errx.NewHTTPError(
		http.StatusRequestEntityTooLarge,
		CodeRequestTooLarge,
		"request body is too large",
	)
}

// 统一收敛 JSON 语法、结构和字段层面的客户端输入错误。
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

// 只多读 1 个字节来判断是否超限，避免为大小检查引入更复杂的状态管理。
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
