package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTestSonarrFailsWithBadURL(t *testing.T) {
	h := &TestHandlers{}
	body := []byte(`{"url":"http://127.0.0.1:1","api_key":"x","enabled":true}`)
	req := httptest.NewRequest("POST", "/settings/sonarr/test", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Sonarr(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte(`"ok":false`)) {
		t.Errorf("expected ok=false; body=%s", w.Body.String())
	}
}

func TestTestSonarrSucceedsAgainstMock(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"appName":"Sonarr","version":"4.0"}`))
	}))
	defer mock.Close()

	h := &TestHandlers{}
	body := []byte(`{"url":"` + mock.URL + `","api_key":"x","enabled":true}`)
	req := httptest.NewRequest("POST", "/settings/sonarr/test", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Sonarr(w, req)
	if !bytes.Contains(w.Body.Bytes(), []byte(`"ok":true`)) {
		t.Errorf("body=%s", w.Body.String())
	}
}
