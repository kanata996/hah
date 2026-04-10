package bind

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/kanata996/hah/errx"
	"github.com/kanata996/hah/internal/req"
)

// 本文件负责 body 绑定相关逻辑，包括 body 读取、Content-Type 判定、JSON 解码和 body 侧错误收敛。
//
// 这里承载的默认 body 契约包括：
//   - 实际读取到零字节 body 时直接 no-op
//   - 非空 body 当前只接受 application/json
//   - body 按默认大小限制读取，超限返回 413 request_too_large
//   - 非法 JSON 返回 400 invalid_json
//   - 不支持的 Content-Type 返回 415 unsupported_media_type

// bindBodyDefault 实现默认 body 绑定契约。
func bindBodyDefault(r *http.Request, target any, cfg bindBodyConfig) error {
	if r == nil {
		return errorsf("request must not be nil")
	}
	if err := validateBindingDestination(target); err != nil {
		return err
	}

	hasBody, err := req.HasBody(r)
	if err != nil {
		return err
	}
	if !hasBody {
		return nil
	}

	mediaType, err := bodyMediaType(r)
	if err != nil {
		return unsupportedMediaTypeError()
	}
	if mediaType != mimeApplicationJSON {
		return unsupportedMediaTypeError()
	}

	body, err := readBody(r.Body, cfg.maxBodyBytes)
	if err != nil {
		if errors.Is(err, errRequestTooLarge) {
			return requestTooLargeError()
		}
		return err
	}

	return decodeJSONBody(body, target, cfg.allowUnknownFields)
}

// bodyMediaType 解析请求头中的主 media type。
func bodyMediaType(r *http.Request) (string, error) {
	if r == nil {
		return "", nil
	}

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
func decodeJSONBody(body []byte, target any, allowUnknownFields bool) error {
	dec := json.NewDecoder(bytes.NewReader(body))
	if !allowUnknownFields {
		dec.DisallowUnknownFields()
	}
	if err := dec.Decode(target); err != nil {
		return mapJSONBodyDecodeError(err)
	}
	var extra any
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
var errRequestTooLarge = errors.New("bind: request body too large")

// readBody 在默认大小限制内完整读取请求 body。
func readBody(body io.ReadCloser, maxBytes int64) ([]byte, error) {
	if body == nil {
		return nil, nil
	}
	if maxBytes <= 0 {
		maxBytes = defaultMaxBodyBytes
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
