package hah_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kanata996/hah"
)

func TestRenderWritesEnvelope(t *testing.T) {
	rr := newResponseRecorder()
	req := newRequest()
	hah.Status(req, http.StatusCreated)

	err := hah.Render(rr, req, map[string]any{"id": "u_1"})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusCreated)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}

	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatalf("data envelope missing or wrong type: %#v", payload["data"])
	}
	if got := data["id"]; got != "u_1" {
		t.Fatalf("data.id = %#v, want u_1", got)
	}
	if _, ok := payload["meta"]; ok {
		t.Fatalf("meta should be omitted, payload = %#v", payload)
	}
}

func TestStatusWrapperInfluencesRender(t *testing.T) {
	rr := newResponseRecorder()
	req := newRequest()

	hah.Status(req, http.StatusAccepted)
	if err := hah.Render(rr, req, map[string]any{"ok": true}); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusAccepted)
	}
}

func TestRenderWithMetaWritesEnvelope(t *testing.T) {
	rr := newResponseRecorder()
	req := newRequest()

	err := hah.RenderWithMeta(
		rr,
		req,
		[]string{"a", "b"},
		map[string]any{"request_id": "req_1"},
	)
	if err != nil {
		t.Fatalf("RenderWithMeta() error = %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if _, ok := payload["data"].([]any); !ok {
		t.Fatalf("data should be an array, got %#v", payload["data"])
	}
	meta, ok := payload["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta missing or wrong type: %#v", payload["meta"])
	}
	if got := meta["request_id"]; got != "req_1" {
		t.Fatalf("meta.request_id = %#v, want req_1", got)
	}
}

func TestRenderEmptyWritesStatusWithoutBody(t *testing.T) {
	rr := newResponseRecorder()
	req := newRequest()

	err := hah.RenderEmpty(rr, req, http.StatusNoContent)
	if err != nil {
		t.Fatalf("RenderEmpty() error = %v", err)
	}

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNoContent)
	}
	if rr.Body.Len() != 0 {
		t.Fatalf("body = %q, want empty", rr.Body.String())
	}
	if got := rr.Header().Get("Content-Type"); got != "" {
		t.Fatalf("Content-Type = %q, want empty", got)
	}
}

func TestRenderHEADUsesStandardServerSemantics(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := hah.Render(w, r, map[string]any{"ok": true}); err != nil {
			t.Fatalf("Render() error = %v", err)
		}
	}))
	defer srv.Close()

	req, err := http.NewRequest(http.MethodHead, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Fatalf("resp.Body.Close() error = %v", closeErr)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if got := resp.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	if len(body) != 0 {
		t.Fatalf("body length = %d, want 0", len(body))
	}
}
