package req

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// 测试清单：
// - 标记说明：[✓] 已核对且已有真实覆盖；[x] 尚未完成，不得作为验收依据。
// - [✓] HasBody 会对 nil request、缓存结果分支和 body 为空/非空做稳定判定。
// - [✓] HasBody / detectBody 会保留后续读取所需的 body 数据。
// - [✓] detectBody 会在 nil body 和读错场景下稳定退化。

type staticReadCloser struct {
	data []byte
}

func (r *staticReadCloser) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	if len(r.data) == 0 {
		return n, io.EOF
	}
	return n, nil
}

func (r *staticReadCloser) Close() error {
	return nil
}

type byteThenErrorReadCloser struct {
	done bool
	err  error
}

type errorReadCloser struct {
	err error
}

func (r *byteThenErrorReadCloser) Read(p []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}
	r.done = true
	p[0] = 'x'
	return 1, r.err
}

func (r *byteThenErrorReadCloser) Close() error {
	return nil
}

func (r errorReadCloser) Read(_ []byte) (int, error) {
	return 0, r.err
}

func (r errorReadCloser) Close() error {
	return nil
}

func TestHasBody(t *testing.T) {
	if got, err := HasBody(nil); err != nil || got {
		t.Fatalf("HasBody(nil) = (%v, %v), want (false, nil)", got, err)
	}

	cachedReq := httptest.NewRequest(http.MethodPost, "/", nil)
	cachedReq = cachedReq.WithContext(context.WithValue(cachedReq.Context(), presenceKey{}, presenceState{
		known: true,
		has:   true,
	}))
	if got, err := HasBody(cachedReq); err != nil || !got {
		t.Fatalf("HasBody(cached) = (%v, %v), want (true, nil)", got, err)
	}

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Body = &staticReadCloser{data: []byte(`{"name":"kanata"}`)}
	if got, err := HasBody(req); err != nil || !got {
		t.Fatalf("HasBody(non-empty) = (%v, %v), want (true, nil)", got, err)
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("ReadAll(replayed body) error = %v", err)
	}
	if string(body) != `{"name":"kanata"}` {
		t.Fatalf("replayed body = %q, want original body", body)
	}
	if got, err := HasBody(req); err != nil || !got {
		t.Fatalf("HasBody(cached second call) = (%v, %v), want (true, nil)", got, err)
	}

	emptyReq := httptest.NewRequest(http.MethodPost, "/", nil)
	emptyReq.Body = &staticReadCloser{}
	if got, err := HasBody(emptyReq); err != nil || got {
		t.Fatalf("HasBody(empty) = (%v, %v), want (false, nil)", got, err)
	}
	body, err = io.ReadAll(emptyReq.Body)
	if err != nil {
		t.Fatalf("ReadAll(empty replayed body) error = %v", err)
	}
	if len(body) != 0 {
		t.Fatalf("empty replayed body = %q, want empty", body)
	}
	if got, err := HasBody(emptyReq); err != nil || got {
		t.Fatalf("HasBody(empty cached second call) = (%v, %v), want (false, nil)", got, err)
	}

	wantErr := errors.New("read failed")
	errReq := httptest.NewRequest(http.MethodPost, "/", nil)
	errReq.Body = errorReadCloser{err: wantErr}
	if got, err := HasBody(errReq); !errors.Is(err, wantErr) || got {
		t.Fatalf("HasBody(read error) = (%v, %v), want (false, %v)", got, err, wantErr)
	}
}

func TestDetectBody(t *testing.T) {
	if got, err := detectBody(nil); err != nil || got {
		t.Fatalf("detectBody(nil) = (%v, %v), want (false, nil)", got, err)
	}

	nilBodyReq := httptest.NewRequest(http.MethodPost, "/", nil)
	nilBodyReq.Body = nil
	if got, err := detectBody(nilBodyReq); err != nil || got {
		t.Fatalf("detectBody(nil body) = (%v, %v), want (false, nil)", got, err)
	}

	emptyReq := httptest.NewRequest(http.MethodPost, "/", nil)
	emptyReq.Body = &staticReadCloser{}
	if got, err := detectBody(emptyReq); err != nil || got {
		t.Fatalf("detectBody(empty) = (%v, %v), want (false, nil)", got, err)
	}
	body, err := io.ReadAll(emptyReq.Body)
	if err != nil {
		t.Fatalf("ReadAll(empty replay) error = %v", err)
	}
	if len(body) != 0 {
		t.Fatalf("empty replay body = %q, want empty", body)
	}

	wantErr := errors.New("boom")
	errReq := httptest.NewRequest(http.MethodPost, "/", nil)
	errReq.Body = &byteThenErrorReadCloser{err: wantErr}
	got, err := detectBody(errReq)
	if !errors.Is(err, wantErr) || got {
		t.Fatalf("detectBody(byte+error) = (%v, %v), want (false, %v)", got, err, wantErr)
	}
	body, readErr := io.ReadAll(errReq.Body)
	if readErr != nil {
		t.Fatalf("ReadAll(error replay) error = %v", readErr)
	}
	if string(body) != "x" {
		t.Fatalf("error replay body = %q, want x", body)
	}
}
