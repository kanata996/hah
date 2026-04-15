package reqx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/kanata996/hah/errx"
)

// 本文件负责 body 绑定相关逻辑，包括 body 读取、Content-Type 判定、JSON 解码和 body 侧错误收敛。
//
// 这里承载的默认 body 契约包括：
//   - 实际读取到零字节 body 时直接 no-op
//   - 非空 body 当前只接受 application/json
//   - body 按默认大小限制读取，超限返回 413 request_too_large
//   - 非法 JSON 返回 400 invalid_json
//   - 不支持的 Content-Type 返回 415 unsupported_media_type

const defaultMaxBodyBytes int64 = 1 << 20

const (
	// CodeInvalidJSON 表示请求 body 不是合法 JSON。
	CodeInvalidJSON = "invalid_json"
	// CodeUnsupportedMediaType 表示请求 body 的 Content-Type 不受支持。
	CodeUnsupportedMediaType = "unsupported_media_type"
	// CodeRequestTooLarge 表示请求 body 超出默认大小限制。
	CodeRequestTooLarge = "request_too_large"
)

const mimeApplicationJSON = "application/json"

// BindBody 只从请求 body 绑定数据。
//
// 解码直接作用在调用方传入的 target 上；JSON 中缺失的字段不会覆盖 target
// 已有值。
func BindBody(r *http.Request, target any) error {
	if err := validateBindInputs(r, target); err != nil {
		return err
	}

	return bindBody(r, target)
}

// bindBody 假定 request 和 target 已完成前置校验，只执行默认 body 绑定本身。
func bindBody(r *http.Request, target any) error {
	// 先探测是否真的存在 body，这样零字节请求可以在 Content-Type 校验前直接 no-op。
	hasBody, err := hasRequestBody(r)
	if err != nil {
		return err
	}
	if !hasBody {
		return nil
	}

	// 读取优先于 media type 语义判断，确保底层 I/O 错误不会被 415 掩盖。
	body, err := readBody(r.Body)
	if err != nil {
		if errors.Is(err, errRequestTooLarge) {
			return requestTooLargeError()
		}
		return err
	}

	mediaType, err := bodyMediaType(r)
	if err != nil {
		return unsupportedMediaTypeError()
	}

	// 默认 body binder 只分发到显式支持的媒体类型实现。
	switch mediaType {
	case mimeApplicationJSON:
		return decodeJSONBody(body, target)
	default:
		return unsupportedMediaTypeError()
	}
}

// bodyMediaType 解析请求头中的主 media type。
// 调用方保证 request 已完成公开入口校验。
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

// decodeJSONBody 负责按默认 JSON 契约解码 body。
func decodeJSONBody(body []byte, target any) error {
	dec := json.NewDecoder(bytes.NewReader(body))
	if err := dec.Decode(target); err != nil {
		return mapJSONBodyDecodeError(err)
	}
	var extra any
	// 第二次 decode 只用于验证“恰好一个 JSON 值”的契约，不消费业务结果。
	if err := dec.Decode(&extra); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return mapJSONBodyDecodeError(err)
	}
	return mapJSONBodyDecodeError(errors.New("request body must contain exactly one JSON value"))
}

// mapJSONBodyDecodeError 把标准库 JSON 解码错误收敛为公开的 HTTP 错误。
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

// errRequestTooLarge 用于在读取阶段标记 body 超限。
var errRequestTooLarge = errors.New("reqx: request body too large")

// readBody 在默认大小限制内完整读取请求 body。
// bindBody 仅在 hasRequestBody 已确认存在非空 body 后调用这里，因此 body 非 nil。
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

// unsupportedMediaTypeError 返回默认 body binder 的 415 错误。
func unsupportedMediaTypeError() error {
	return errx.NewHTTPError(
		http.StatusUnsupportedMediaType,
		CodeUnsupportedMediaType,
		"Content-Type must be application/json",
	)
}

// requestTooLargeError 返回默认 body binder 的 413 错误。
func requestTooLargeError() error {
	return errx.NewHTTPError(
		http.StatusRequestEntityTooLarge,
		CodeRequestTooLarge,
		"request body is too large",
	)
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
