package resp

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
)

type responseState interface {
	responseStarted() bool
}

type responseUnwrapper interface {
	Unwrap() http.ResponseWriter
}

type responseStatus interface {
	Status() int
}

type responseBytesWritten interface {
	BytesWritten() int
}

type trackingResponseWriter struct {
	http.ResponseWriter
	started  bool
	hijacked bool
	status   int
	bytes    int
}

func NewTrackingResponseWriter(w http.ResponseWriter) http.ResponseWriter {
	return &trackingResponseWriter{ResponseWriter: w}
}

func (w *trackingResponseWriter) responseStarted() bool {
	return w.started || w.hijacked
}

func (w *trackingResponseWriter) WriteHeader(status int) {
	if w.started {
		return
	}

	w.started = true
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *trackingResponseWriter) Write(p []byte) (int, error) {
	if !w.started {
		w.WriteHeader(http.StatusOK)
	}

	n, err := w.ResponseWriter.Write(p)
	w.bytes += n
	return n, err
}

func (w *trackingResponseWriter) ReadFrom(r io.Reader) (int64, error) {
	if !w.started {
		w.WriteHeader(http.StatusOK)
	}

	if rf, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		n, err := rf.ReadFrom(r)
		w.bytes += int(n)
		return n, err
	}

	n, err := io.Copy(struct{ io.Writer }{Writer: w}, r)
	return n, err
}

func (w *trackingResponseWriter) Flush() {
	if !w.started {
		w.WriteHeader(http.StatusOK)
	}

	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *trackingResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("hah: response writer does not support hijacking")
	}
	conn, rw, err := hijacker.Hijack()
	if err == nil {
		w.hijacked = true
		w.started = true
	}
	return conn, rw, err
}

func (w *trackingResponseWriter) Push(target string, opts *http.PushOptions) error {
	pusher, ok := w.ResponseWriter.(http.Pusher)
	if !ok {
		return http.ErrNotSupported
	}
	return pusher.Push(target, opts)
}

func ResponseStarted(w http.ResponseWriter) bool {
	for w != nil {
		if state, ok := w.(responseState); ok && state.responseStarted() {
			return true
		}
		if bytesWritten, ok := w.(responseBytesWritten); ok && bytesWritten.BytesWritten() > 0 {
			return true
		}
		if status, ok := w.(responseStatus); ok && status.Status() > 0 {
			return true
		}

		unwrapper, ok := w.(responseUnwrapper)
		if !ok {
			return false
		}

		next := unwrapper.Unwrap()
		if next == w {
			return false
		}
		w = next
	}

	return false
}
