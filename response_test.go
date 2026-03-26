package hah_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/kanata996/hah"
)

func TestWriteSuccess(t *testing.T) {
	rr := newResponseRecorder()

	err := hah.Respond(rr, http.StatusCreated, map[string]any{"id": "u_1"})
	if err != nil {
		t.Fatalf("Respond() error = %v", err)
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

func TestWriteMetaSuccess(t *testing.T) {
	rr := newResponseRecorder()

	err := hah.RespondWithMeta(
		rr,
		http.StatusOK,
		[]string{"a", "b"},
		map[string]any{"request_id": "req_1"},
	)
	if err != nil {
		t.Fatalf("RespondWithMeta() error = %v", err)
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

func TestWriteEmpty(t *testing.T) {
	rr := newResponseRecorder()

	err := hah.RespondEmpty(rr, http.StatusNoContent)
	if err != nil {
		t.Fatalf("RespondEmpty() error = %v", err)
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
