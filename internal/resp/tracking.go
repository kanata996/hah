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
	started bool
}

type trackingFlushWriter struct {
	*trackingResponseWriter
}

type trackingHijackWriter struct {
	*trackingResponseWriter
}

type trackingPushWriter struct {
	*trackingResponseWriter
}

type trackingFlushHijackWriter struct {
	*trackingResponseWriter
}

type trackingFlushPushWriter struct {
	*trackingResponseWriter
}

type trackingHijackPushWriter struct {
	*trackingResponseWriter
}

type trackingFlushHijackPushWriter struct {
	*trackingResponseWriter
}

func NewTrackingResponseWriter(w http.ResponseWriter) http.ResponseWriter {
	tw := &trackingResponseWriter{ResponseWriter: w}

	_, hasFlush := w.(http.Flusher)
	_, hasHijack := w.(http.Hijacker)
	_, hasPush := w.(http.Pusher)

	switch {
	case hasFlush && hasHijack && hasPush:
		return &trackingFlushHijackPushWriter{trackingResponseWriter: tw}
	case hasFlush && hasHijack:
		return &trackingFlushHijackWriter{trackingResponseWriter: tw}
	case hasFlush && hasPush:
		return &trackingFlushPushWriter{trackingResponseWriter: tw}
	case hasHijack && hasPush:
		return &trackingHijackPushWriter{trackingResponseWriter: tw}
	case hasFlush:
		return &trackingFlushWriter{trackingResponseWriter: tw}
	case hasHijack:
		return &trackingHijackWriter{trackingResponseWriter: tw}
	case hasPush:
		return &trackingPushWriter{trackingResponseWriter: tw}
	default:
		return tw
	}
}

func (w *trackingResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *trackingResponseWriter) responseStarted() bool {
	return w.started
}

func (w *trackingResponseWriter) WriteHeader(status int) {
	if w.started {
		return
	}

	w.started = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *trackingResponseWriter) Write(p []byte) (int, error) {
	if !w.started {
		w.WriteHeader(http.StatusOK)
	}

	return w.ResponseWriter.Write(p)
}

func (w *trackingResponseWriter) ReadFrom(r io.Reader) (int64, error) {
	if !w.started {
		w.WriteHeader(http.StatusOK)
	}

	if rf, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		return rf.ReadFrom(r)
	}

	n, err := io.Copy(struct{ io.Writer }{Writer: w}, r)
	return n, err
}

func (w *trackingResponseWriter) flush() {
	if !w.started {
		w.WriteHeader(http.StatusOK)
	}

	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *trackingResponseWriter) hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("hah: response writer does not support hijacking")
	}
	conn, rw, err := hijacker.Hijack()
	if err == nil {
		w.started = true
	}
	return conn, rw, err
}

func (w *trackingResponseWriter) push(target string, opts *http.PushOptions) error {
	pusher, ok := w.ResponseWriter.(http.Pusher)
	if !ok {
		return http.ErrNotSupported
	}
	return pusher.Push(target, opts)
}

func (w *trackingFlushWriter) Flush() {
	w.trackingResponseWriter.flush()
}

func (w *trackingHijackWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.trackingResponseWriter.hijack()
}

func (w *trackingPushWriter) Push(target string, opts *http.PushOptions) error {
	return w.trackingResponseWriter.push(target, opts)
}

func (w *trackingFlushHijackWriter) Flush() {
	w.trackingResponseWriter.flush()
}

func (w *trackingFlushHijackWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.trackingResponseWriter.hijack()
}

func (w *trackingFlushPushWriter) Flush() {
	w.trackingResponseWriter.flush()
}

func (w *trackingFlushPushWriter) Push(target string, opts *http.PushOptions) error {
	return w.trackingResponseWriter.push(target, opts)
}

func (w *trackingHijackPushWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.trackingResponseWriter.hijack()
}

func (w *trackingHijackPushWriter) Push(target string, opts *http.PushOptions) error {
	return w.trackingResponseWriter.push(target, opts)
}

func (w *trackingFlushHijackPushWriter) Flush() {
	w.trackingResponseWriter.flush()
}

func (w *trackingFlushHijackPushWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.trackingResponseWriter.hijack()
}

func (w *trackingFlushHijackPushWriter) Push(target string, opts *http.PushOptions) error {
	return w.trackingResponseWriter.push(target, opts)
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
