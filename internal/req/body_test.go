package req

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

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
	if len(p) == 0 {
		return 0, nil
	}
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
	t.Run("nil request", func(t *testing.T) {
		if got, err := HasBody(nil); err != nil || got {
			t.Fatalf("HasBody(nil) = (%v, %v), want (false, nil)", got, err)
		}
	})

	t.Run("nil body is treated as empty", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Body = nil

		if got, err := HasBody(req); err != nil || got {
			t.Fatalf("HasBody(nil body) = (%v, %v), want (false, nil)", got, err)
		}
	})

	t.Run("non-empty body is detected and cached", func(t *testing.T) {
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
			t.Fatalf("HasBody(second call after read) = (%v, %v), want (true, nil)", got, err)
		}
	})

	t.Run("empty body is detected and cached", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Body = &staticReadCloser{}

		if got, err := HasBody(req); err != nil || got {
			t.Fatalf("HasBody(empty) = (%v, %v), want (false, nil)", got, err)
		}

		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("ReadAll(empty replayed body) error = %v", err)
		}
		if len(body) != 0 {
			t.Fatalf("empty replayed body = %q, want empty", body)
		}

		if got, err := HasBody(req); err != nil || got {
			t.Fatalf("HasBody(second call after empty read) = (%v, %v), want (false, nil)", got, err)
		}
	})

	t.Run("read error after a byte preserves replay body", func(t *testing.T) {
		wantErr := errors.New("boom")
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Body = &byteThenErrorReadCloser{err: wantErr}

		if got, err := HasBody(req); !errors.Is(err, wantErr) || got {
			t.Fatalf("HasBody(byte+error) = (%v, %v), want (false, %v)", got, err, wantErr)
		}

		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("ReadAll(replayed error body) error = %v", err)
		}
		if string(body) != "x" {
			t.Fatalf("replayed error body = %q, want x", body)
		}
	})

	t.Run("read error without bytes is returned and not cached", func(t *testing.T) {
		wantErr := errors.New("read failed")
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.Body = errorReadCloser{err: wantErr}

		if got, err := HasBody(req); !errors.Is(err, wantErr) || got {
			t.Fatalf("HasBody(read error) = (%v, %v), want (false, %v)", got, err, wantErr)
		}

		if _, err := io.ReadAll(req.Body); !errors.Is(err, wantErr) {
			t.Fatalf("ReadAll(error body) error = %v, want %v", err, wantErr)
		}

		if got, err := HasBody(req); !errors.Is(err, wantErr) || got {
			t.Fatalf("HasBody(second read error) = (%v, %v), want (false, %v)", got, err, wantErr)
		}
	})
}
