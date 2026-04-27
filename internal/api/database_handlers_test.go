package api

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestRescanReturnsOpID(t *testing.T) {
	h := &DatabaseHandlers{IPC: stubDaemonIPC{
		statusBody: json.RawMessage(`{"op_id":"op-123"}`),
	}}
	body := []byte(`{"dry_run":false}`)
	req := httptest.NewRequest("POST", "/database/rescan", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Rescan(w, req)
	if w.Code != 202 {
		t.Fatalf("status %d", w.Code)
	}
	var got map[string]string
	json.Unmarshal(w.Body.Bytes(), &got)
	if got["op_id"] == "" {
		t.Error("expected op_id in response")
	}
}

func TestResetRequiresConfirm(t *testing.T) {
	h := &DatabaseHandlers{IPC: stubDaemonIPC{}}
	req := httptest.NewRequest("POST", "/database/reset", bytes.NewReader([]byte(`{"confirm":"wrong"}`)))
	w := httptest.NewRecorder()
	h.Reset(w, req)
	if w.Code != 400 {
		t.Errorf("want 400, got %d", w.Code)
	}
}
