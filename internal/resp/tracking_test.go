package resp_test

import (
	"bufio"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kanata996/hah/internal/resp"
)

func newResponseRecorder() *httptest.ResponseRecorder {
	return httptest.NewRecorder()
}

type readerFromRecorder struct {
	*httptest.ResponseRecorder
	readFromCalls int
}

func (w *readerFromRecorder) ReadFrom(r io.Reader) (int64, error) {
	w.readFromCalls++
	return io.Copy(w.ResponseRecorder, r)
}

type hijackableRecorder struct {
	*httptest.ResponseRecorder
	hijackCalls int
}

func (w *hijackableRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	w.hijackCalls++
	return nil, nil, nil
}

type pushableRecorder struct {
	*httptest.ResponseRecorder
	pushCalls int
	target    string
}

type deadlineRecorder struct {
	*httptest.ResponseRecorder
	writeDeadline time.Time
}

func (w *pushableRecorder) Push(target string, _ *http.PushOptions) error {
	w.pushCalls++
	w.target = target
	return nil
}

func (w *deadlineRecorder) SetWriteDeadline(deadline time.Time) error {
	w.writeDeadline = deadline
	return nil
}

type statusTrackingRecorder struct {
	*httptest.ResponseRecorder
	status       int
	bytesWritten int
}

type statusOnlyRecorder struct {
	*httptest.ResponseRecorder
	status int
}

type unwrapRecorder struct {
	http.ResponseWriter
}

type selfUnwrapRecorder struct {
	*httptest.ResponseRecorder
}

func (w *unwrapRecorder) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *selfUnwrapRecorder) Unwrap() http.ResponseWriter {
	return w
}

func (w *statusTrackingRecorder) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseRecorder.WriteHeader(status)
}

func (w *statusTrackingRecorder) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseRecorder.Write(p)
	w.bytesWritten += n
	return n, err
}

func (w *statusTrackingRecorder) Status() int {
	return w.status
}

func (w *statusTrackingRecorder) BytesWritten() int {
	return w.bytesWritten
}

func (w *statusOnlyRecorder) Status() int {
	return w.status
}

func TestTrackingWriterWriteHeaderMarksResponseStarted(t *testing.T) {
	rr := newResponseRecorder()
	tw := resp.NewTrackingResponseWriter(rr)

	if resp.ResponseStarted(tw) {
		t.Fatalf("response should not be started yet")
	}

	tw.WriteHeader(http.StatusAccepted)

	if !resp.ResponseStarted(tw) {
		t.Fatalf("response should be started after WriteHeader")
	}
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusAccepted)
	}
}

func TestTrackingWriterWriteHeaderIgnoresSecondCall(t *testing.T) {
	rr := newResponseRecorder()
	tw := resp.NewTrackingResponseWriter(rr)

	tw.WriteHeader(http.StatusAccepted)
	tw.WriteHeader(http.StatusConflict)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusAccepted)
	}
}

func TestTrackingWriterWriteStartsResponse(t *testing.T) {
	rr := newResponseRecorder()
	tw := resp.NewTrackingResponseWriter(rr)

	if _, err := tw.Write([]byte("partial")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	if !resp.ResponseStarted(tw) {
		t.Fatalf("response should be started after Write")
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if got := rr.Body.String(); got != "partial" {
		t.Fatalf("body = %q, want partial", got)
	}
}

func TestTrackingWriterReadFromStartsResponse(t *testing.T) {
	rr := newResponseRecorder()
	tw := resp.NewTrackingResponseWriter(rr)

	readerFrom, ok := tw.(io.ReaderFrom)
	if !ok {
		t.Fatalf("tracking writer should implement io.ReaderFrom")
	}

	n, err := readerFrom.ReadFrom(strings.NewReader("stream"))
	if err != nil {
		t.Fatalf("ReadFrom() error = %v", err)
	}

	if n != int64(len("stream")) {
		t.Fatalf("copied bytes = %d, want %d", n, len("stream"))
	}
	if !resp.ResponseStarted(tw) {
		t.Fatalf("response should be started after ReadFrom")
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if got := rr.Body.String(); got != "stream" {
		t.Fatalf("body = %q, want stream", got)
	}
}

func TestTrackingWriterReadFromUsesUnderlyingReaderFrom(t *testing.T) {
	rr := &readerFromRecorder{ResponseRecorder: newResponseRecorder()}
	tw := resp.NewTrackingResponseWriter(rr)

	readerFrom, ok := tw.(io.ReaderFrom)
	if !ok {
		t.Fatalf("tracking writer should implement io.ReaderFrom")
	}

	n, err := readerFrom.ReadFrom(strings.NewReader("stream"))
	if err != nil {
		t.Fatalf("ReadFrom() error = %v", err)
	}

	if n != int64(len("stream")) {
		t.Fatalf("copied bytes = %d, want %d", n, len("stream"))
	}
	if rr.readFromCalls != 1 {
		t.Fatalf("underlying ReadFrom calls = %d, want 1", rr.readFromCalls)
	}
}

func TestTrackingWriterFlushStartsResponse(t *testing.T) {
	rr := newResponseRecorder()
	tw := resp.NewTrackingResponseWriter(rr)

	flusher, ok := tw.(http.Flusher)
	if !ok {
		t.Fatalf("tracking writer should implement http.Flusher")
	}

	flusher.Flush()

	if !resp.ResponseStarted(tw) {
		t.Fatalf("response should be started after Flush")
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestTrackingWriterHijackUnsupported(t *testing.T) {
	tw := resp.NewTrackingResponseWriter(newResponseRecorder())

	hijacker, ok := tw.(http.Hijacker)
	if !ok {
		t.Fatalf("tracking writer should implement http.Hijacker")
	}

	_, _, err := hijacker.Hijack()
	if err == nil {
		t.Fatalf("expected hijack error, got nil")
	}
}

func TestTrackingWriterHijackUsesUnderlyingHijacker(t *testing.T) {
	rr := &hijackableRecorder{ResponseRecorder: newResponseRecorder()}
	tw := resp.NewTrackingResponseWriter(rr)

	hijacker, ok := tw.(http.Hijacker)
	if !ok {
		t.Fatalf("tracking writer should implement http.Hijacker")
	}

	_, _, err := hijacker.Hijack()
	if err != nil {
		t.Fatalf("Hijack() error = %v", err)
	}
	if rr.hijackCalls != 1 {
		t.Fatalf("underlying Hijack calls = %d, want 1", rr.hijackCalls)
	}
	if !resp.ResponseStarted(tw) {
		t.Fatalf("response should be considered started after Hijack")
	}
}

func TestTrackingWriterPushUnsupported(t *testing.T) {
	tw := resp.NewTrackingResponseWriter(newResponseRecorder())

	pusher, ok := tw.(http.Pusher)
	if !ok {
		t.Fatalf("tracking writer should implement http.Pusher")
	}

	err := pusher.Push("/asset.js", nil)
	if !errors.Is(err, http.ErrNotSupported) {
		t.Fatalf("Push() error = %v, want %v", err, http.ErrNotSupported)
	}
}

func TestTrackingWriterPushUsesUnderlyingPusher(t *testing.T) {
	rr := &pushableRecorder{ResponseRecorder: newResponseRecorder()}
	tw := resp.NewTrackingResponseWriter(rr)

	pusher, ok := tw.(http.Pusher)
	if !ok {
		t.Fatalf("tracking writer should implement http.Pusher")
	}

	err := pusher.Push("/asset.js", nil)
	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if rr.pushCalls != 1 {
		t.Fatalf("underlying Push calls = %d, want 1", rr.pushCalls)
	}
	if rr.target != "/asset.js" {
		t.Fatalf("push target = %q, want /asset.js", rr.target)
	}
}

func TestTrackingWriterUnwrapAllowsResponseControllerPassthrough(t *testing.T) {
	rr := &deadlineRecorder{ResponseRecorder: newResponseRecorder()}
	tw := resp.NewTrackingResponseWriter(rr)

	deadline := time.Unix(123, 0)
	if err := http.NewResponseController(tw).SetWriteDeadline(deadline); err != nil {
		t.Fatalf("SetWriteDeadline() error = %v", err)
	}
	if !rr.writeDeadline.Equal(deadline) {
		t.Fatalf("write deadline = %v, want %v", rr.writeDeadline, deadline)
	}
}

func TestResponseStartedUsesStatusReportingWriter(t *testing.T) {
	rr := &statusTrackingRecorder{ResponseRecorder: newResponseRecorder()}

	if resp.ResponseStarted(rr) {
		t.Fatalf("response should not be started yet")
	}

	if _, err := rr.Write([]byte("partial")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	if !resp.ResponseStarted(rr) {
		t.Fatalf("response should be considered started")
	}
}

func TestResponseStartedUsesStatusOnlyWriter(t *testing.T) {
	rr := &statusOnlyRecorder{
		ResponseRecorder: newResponseRecorder(),
		status:           http.StatusAccepted,
	}

	if !resp.ResponseStarted(rr) {
		t.Fatalf("response should be considered started from status alone")
	}
}

func TestResponseStartedFollowsUnwrapChain(t *testing.T) {
	inner := resp.NewTrackingResponseWriter(newResponseRecorder())
	if _, err := inner.Write([]byte("partial")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	outer := &unwrapRecorder{ResponseWriter: inner}
	if !resp.ResponseStarted(outer) {
		t.Fatalf("response should be considered started through Unwrap")
	}
}

func TestResponseStartedReturnsFalseForNilWriter(t *testing.T) {
	if resp.ResponseStarted(nil) {
		t.Fatalf("nil writer should not be considered started")
	}
}

func TestResponseStartedReturnsFalseForPlainWriter(t *testing.T) {
	if resp.ResponseStarted(newResponseRecorder()) {
		t.Fatalf("plain recorder should not be considered started")
	}
}

func TestResponseStartedStopsOnSelfReferentialUnwrap(t *testing.T) {
	rr := &selfUnwrapRecorder{ResponseRecorder: newResponseRecorder()}

	if resp.ResponseStarted(rr) {
		t.Fatalf("self-unwrapping writer should not be considered started")
	}
}
