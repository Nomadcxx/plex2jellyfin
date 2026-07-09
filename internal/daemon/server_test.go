package daemon

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/Nomadcxx/plex2jellyfin/internal/database"
	"github.com/Nomadcxx/plex2jellyfin/internal/jellyfin"
	"github.com/Nomadcxx/plex2jellyfin/internal/transfer"
)

func TestServerHealth(t *testing.T) {
	handler, err := NewMediaHandler(MediaHandlerConfig{
		TVLibraries: []string{"/tv"},
		MovieLibs:   []string{"/movies"},
		Backend:     transfer.BackendNative,
	})
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}
	defer handler.Shutdown()

	server := NewServer(handler, nil, ":0", nil, "")

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	server.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "healthy" {
		t.Errorf("expected status healthy, got %s", resp.Status)
	}
}

func TestServerHealthUnhealthy(t *testing.T) {
	handler, err := NewMediaHandler(MediaHandlerConfig{
		TVLibraries: []string{"/tv"},
		MovieLibs:   []string{"/movies"},
		Backend:     transfer.BackendNative,
	})
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}
	defer handler.Shutdown()

	server := NewServer(handler, nil, ":0", nil, "")
	server.SetHealthy(false)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	server.handleHealth(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", w.Code)
	}

	var resp HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "unhealthy" {
		t.Errorf("expected status unhealthy, got %s", resp.Status)
	}
}

func TestServerReady(t *testing.T) {
	handler, err := NewMediaHandler(MediaHandlerConfig{
		TVLibraries: []string{"/tv"},
		MovieLibs:   []string{"/movies"},
		Backend:     transfer.BackendNative,
	})
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}
	defer handler.Shutdown()

	server := NewServer(handler, nil, ":0", nil, "")

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()

	server.handleReady(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if w.Body.String() != "ready" {
		t.Errorf("expected body 'ready', got '%s'", w.Body.String())
	}
}

func TestServerMetrics(t *testing.T) {
	handler, err := NewMediaHandler(MediaHandlerConfig{
		TVLibraries: []string{"/tv"},
		MovieLibs:   []string{"/movies"},
		Backend:     transfer.BackendNative,
	})
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}
	defer handler.Shutdown()

	handler.stats.RecordMovie(1024 * 1024 * 100)
	handler.stats.RecordTV(1024 * 1024 * 50)
	handler.stats.RecordError()

	server := NewServer(handler, nil, ":0", nil, "")

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	server.handleMetrics(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp MetricsResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.MoviesProcessed != 1 {
		t.Errorf("expected 1 movie, got %d", resp.MoviesProcessed)
	}
	if resp.TVProcessed != 1 {
		t.Errorf("expected 1 TV, got %d", resp.TVProcessed)
	}
	if resp.TotalProcessed != 2 {
		t.Errorf("expected 2 total, got %d", resp.TotalProcessed)
	}
	if resp.Errors != 1 {
		t.Errorf("expected 1 error, got %d", resp.Errors)
	}
	if resp.BytesTransferMB < 149 || resp.BytesTransferMB > 151 {
		t.Errorf("expected ~150 MB, got %.2f", resp.BytesTransferMB)
	}
}

func TestStats(t *testing.T) {
	stats := NewStats()

	stats.RecordMovie(100)
	stats.RecordTV(200)
	stats.RecordError()

	snap := stats.Snapshot()

	if snap.MoviesProcessed != 1 {
		t.Errorf("expected 1 movie, got %d", snap.MoviesProcessed)
	}
	if snap.TVProcessed != 1 {
		t.Errorf("expected 1 TV, got %d", snap.TVProcessed)
	}
	if snap.BytesTransferred != 300 {
		t.Errorf("expected 300 bytes, got %d", snap.BytesTransferred)
	}
	if snap.Errors != 1 {
		t.Errorf("expected 1 error, got %d", snap.Errors)
	}
	if snap.Uptime <= 0 {
		t.Error("expected positive uptime")
	}
}

func TestServerJellyfinWebhookSecretValidation(t *testing.T) {
	handler := &MediaHandler{
		playbackLocks: jellyfin.NewPlaybackLockManager(),
		deferredQueue: jellyfin.NewDeferredQueue(),
	}
	server := NewServer(handler, nil, ":0", nil, "secret")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/jellyfin", bytes.NewBufferString(`{"NotificationType":"PlaybackStart"}`))
	w := httptest.NewRecorder()
	server.handleJellyfinWebhook(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without secret header, got %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/jellyfin", bytes.NewBufferString(`{"NotificationType":"PlaybackStart","ItemPath":"/x.mkv"}`))
	req.Header.Set("X-Plex2Jellyfin-Webhook-Secret", "wrong")
	w = httptest.NewRecorder()
	server.handleJellyfinWebhook(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with wrong secret header, got %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/jellyfin", bytes.NewBufferString(`{"NotificationType":"PlaybackStart","ItemPath":"/x.mkv"}`))
	req.Header.Set("X-Plex2Jellyfin-Webhook-Secret", "secret")
	w = httptest.NewRecorder()
	server.handleJellyfinWebhook(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with valid secret, got %d", w.Code)
	}
}

func TestServerJellyfinWebhookSecretValidation_EmptySecretLoopbackAllowed(t *testing.T) {
	handler := &MediaHandler{
		playbackLocks: jellyfin.NewPlaybackLockManager(),
		deferredQueue: jellyfin.NewDeferredQueue(),
	}
	server := NewServer(handler, nil, ":0", nil, "")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/jellyfin", bytes.NewBufferString(`{"NotificationType":"PlaybackStart","ItemPath":"/x.mkv"}`))
	req.RemoteAddr = "127.0.0.1:9999"
	w := httptest.NewRecorder()
	server.handleJellyfinWebhook(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for loopback request when webhook secret is empty, got %d", w.Code)
	}
}

func TestServerJellyfinWebhookSecretValidation_EmptySecretNonLoopbackDenied(t *testing.T) {
	handler := &MediaHandler{
		playbackLocks: jellyfin.NewPlaybackLockManager(),
		deferredQueue: jellyfin.NewDeferredQueue(),
	}
	server := NewServer(handler, nil, ":0", nil, "")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/jellyfin", bytes.NewBufferString(`{"NotificationType":"PlaybackStart","ItemPath":"/x.mkv"}`))
	req.RemoteAddr = "192.168.1.10:9999"
	w := httptest.NewRecorder()
	server.handleJellyfinWebhook(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for non-loopback request when webhook secret is empty, got %d", w.Code)
	}
}

func TestServerJellyfinWebhookReturnsServerErrorOnProcessingFailure(t *testing.T) {
	db, err := database.OpenPath(filepath.Join(t.TempDir(), "webhook.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	handler := &MediaHandler{db: db}
	server := NewServer(handler, nil, ":0", nil, "secret")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/jellyfin", bytes.NewBufferString(`{
		"NotificationType":"ItemAdded",
		"ItemId":"jf-1",
		"ItemPath":"/library/Movie.mkv",
		"Name":"Movie",
		"ItemType":"Movie"
	}`))
	req.Header.Set("X-Plex2Jellyfin-Webhook-Secret", "secret")
	w := httptest.NewRecorder()

	server.handleJellyfinWebhook(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 on processing failure, got %d", w.Code)
	}
}

func TestServerJellyfinWebhookPlaybackStartStop(t *testing.T) {
	handler := &MediaHandler{
		playbackLocks: jellyfin.NewPlaybackLockManager(),
		deferredQueue: jellyfin.NewDeferredQueue(),
	}
	server := NewServer(handler, nil, ":0", nil, "secret")

	path := "/library/Movies/Movie (2025)/Movie (2025).mkv"
	startReq := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/jellyfin", bytes.NewBufferString(`{"NotificationType":"PlaybackStart","ItemPath":"`+path+`","NotificationUsername":"u"}`))
	startReq.Header.Set("X-Plex2Jellyfin-Webhook-Secret", "secret")
	startW := httptest.NewRecorder()
	server.handleJellyfinWebhook(startW, startReq)
	if startW.Code != http.StatusOK {
		t.Fatalf("expected 200 on playback start, got %d", startW.Code)
	}
	if locked, _ := handler.playbackLocks.IsLocked(path); !locked {
		t.Fatalf("expected playback lock to be added")
	}

	handler.deferredQueue.Add(path, jellyfin.DeferredOp{Type: "organize_movie", SourcePath: path})

	stopReq := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/jellyfin", bytes.NewBufferString(`{"NotificationType":"PlaybackStop","ItemPath":"`+path+`"}`))
	stopReq.Header.Set("X-Plex2Jellyfin-Webhook-Secret", "secret")
	stopW := httptest.NewRecorder()
	server.handleJellyfinWebhook(stopW, stopReq)
	if stopW.Code != http.StatusOK {
		t.Fatalf("expected 200 on playback stop, got %d", stopW.Code)
	}
	if locked, _ := handler.playbackLocks.IsLocked(path); locked {
		t.Fatalf("expected playback lock to be removed")
	}
	if ops := handler.deferredQueue.GetForPath(path); len(ops) != 0 {
		t.Fatalf("expected deferred operations to be drained, got %d", len(ops))
	}
}

func TestJellyfinWebhookFlatPayload(t *testing.T) {
	var event jellyfin.WebhookEvent
	payload := `{"NotificationType":"ItemAdded","ItemId":"flat-id","ItemPath":"/flat/path.mkv","Provider_imdb":"tt0133093"}`
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		t.Fatalf("failed to decode flat payload: %v", err)
	}
	if event.NotificationType != jellyfin.EventItemAdded {
		t.Errorf("expected NotificationType=%s, got %s", jellyfin.EventItemAdded, event.NotificationType)
	}
	if event.ItemID != "flat-id" {
		t.Errorf("expected ItemID=flat-id, got %s", event.ItemID)
	}
	if event.ItemPath != "/flat/path.mkv" {
		t.Errorf("expected ItemPath=/flat/path.mkv, got %s", event.ItemPath)
	}
	if event.ProviderImdb != "tt0133093" {
		t.Errorf("expected ProviderImdb=tt0133093, got %s", event.ProviderImdb)
	}
}

func TestJellyfinWebhookNestedPayload(t *testing.T) {
	var event jellyfin.WebhookEvent
	payload := `{"eventType":"ItemAdded","timestamp":"2025-01-01T00:00:00Z","payload":{"item":{"id":"abc-nested-id","path":"/media/Movies/The Matrix.mkv","name":"The Matrix","type":"Movie","providerIds":{"Imdb":"tt0133093","Tmdb":"603","Tvdb":""}}}}`
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		t.Fatalf("failed to decode nested payload: %v", err)
	}
	if event.NotificationType != jellyfin.EventItemAdded {
		t.Errorf("expected NotificationType=%s, got %s", jellyfin.EventItemAdded, event.NotificationType)
	}
	if event.ItemID != "abc-nested-id" {
		t.Errorf("expected ItemID=abc-nested-id, got %s", event.ItemID)
	}
	if event.ItemPath != "/media/Movies/The Matrix.mkv" {
		t.Errorf("expected ItemPath, got %s", event.ItemPath)
	}
	if event.ProviderImdb != "tt0133093" {
		t.Errorf("expected ProviderImdb=tt0133093, got %s", event.ProviderImdb)
	}
	if event.ProviderTmdb != "603" {
		t.Errorf("expected ProviderTmdb=603, got %s", event.ProviderTmdb)
	}
	if event.ItemName != "The Matrix" {
		t.Errorf("expected ItemName=The Matrix, got %s", event.ItemName)
	}
	if event.ItemType != "Movie" {
		t.Errorf("expected ItemType=Movie, got %s", event.ItemType)
	}
}
