package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/jellyfin"
)

func TestWebhookInvalidPayload(t *testing.T) {
	s := &Server{
		cfg: &config.Config{
			Jellyfin: config.JellyfinConfig{WebhookSecret: "test-secret"},
		},
		playbackLocks: jellyfin.NewPlaybackLockManager(),
		deferredQueue: jellyfin.NewDeferredQueue(),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/jellyfin", bytes.NewBufferString("{"))
	req.Header.Set("X-Jellywatch-Webhook-Secret", "test-secret")
	w := httptest.NewRecorder()

	s.HandleJellyfinWebhook(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestWebhookPlaybackStartAndStop(t *testing.T) {
	s := &Server{
		cfg: &config.Config{
			Jellyfin: config.JellyfinConfig{WebhookSecret: "test-secret"},
		},
		playbackLocks: jellyfin.NewPlaybackLockManager(),
		deferredQueue: jellyfin.NewDeferredQueue(),
	}

	path := "/media/Movies/The Matrix (1999)/The Matrix (1999).mkv"
	startPayload := []byte(`{"NotificationType":"PlaybackStart","NotificationUsername":"alice","DeviceName":"TV","ClientName":"Jellyfin","ItemPath":"` + path + `","ItemId":"123"}`)

	startReq := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/jellyfin", bytes.NewReader(startPayload))
	startReq.Header.Set("X-Jellywatch-Webhook-Secret", "test-secret")
	startW := httptest.NewRecorder()
	s.HandleJellyfinWebhook(startW, startReq)

	if startW.Code != http.StatusOK {
		t.Fatalf("expected 200 on playback start, got %d", startW.Code)
	}
	if locked, _ := s.playbackLocks.IsLocked(path); !locked {
		t.Fatalf("expected path to be locked after playback start")
	}

	// Add deferred op for same path; stop should clear lock + drain deferred ops for path.
	s.deferredQueue.Add(path, jellyfin.DeferredOp{Type: "organize_movie", SourcePath: path})
	stopPayload := []byte(`{"NotificationType":"PlaybackStop","ItemPath":"` + path + `"}`)
	stopReq := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/jellyfin", bytes.NewReader(stopPayload))
	stopReq.Header.Set("X-Jellywatch-Webhook-Secret", "test-secret")
	stopW := httptest.NewRecorder()
	s.HandleJellyfinWebhook(stopW, stopReq)

	if stopW.Code != http.StatusOK {
		t.Fatalf("expected 200 on playback stop, got %d", stopW.Code)
	}
	if locked, _ := s.playbackLocks.IsLocked(path); locked {
		t.Fatalf("expected path to be unlocked after playback stop")
	}
	if ops := s.deferredQueue.GetForPath(path); len(ops) != 0 {
		t.Fatalf("expected deferred ops to be removed for path, got %d", len(ops))
	}
}

func TestWebhookUnknownEvent(t *testing.T) {
	s := &Server{
		cfg: &config.Config{
			Jellyfin: config.JellyfinConfig{WebhookSecret: "test-secret"},
		},
		playbackLocks: jellyfin.NewPlaybackLockManager(),
		deferredQueue: jellyfin.NewDeferredQueue(),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/jellyfin", bytes.NewBufferString(`{"NotificationType":"Nope"}`))
	req.Header.Set("X-Jellywatch-Webhook-Secret", "test-secret")
	w := httptest.NewRecorder()

	s.HandleJellyfinWebhook(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for unknown event, got %d", w.Code)
	}
}

func TestWebhookItemAddedPersistsToDB(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "api-webhook.db")
	db, err := database.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("OpenPath() failed: %v", err)
	}
	defer db.Close()

	s := &Server{
		cfg: &config.Config{
			Jellyfin: config.JellyfinConfig{WebhookSecret: "test-secret"},
		},
		db:            db,
		playbackLocks: jellyfin.NewPlaybackLockManager(),
		deferredQueue: jellyfin.NewDeferredQueue(),
	}

	path := "/library/Movies/The Matrix (1999)/The Matrix (1999).mkv"
	payload := []byte(`{"NotificationType":"ItemAdded","ItemPath":"` + path + `","ItemId":"jf-123","Name":"The Matrix","ItemType":"Movie"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/jellyfin", bytes.NewReader(payload))
	req.Header.Set("X-Jellywatch-Webhook-Secret", "test-secret")
	w := httptest.NewRecorder()

	s.HandleJellyfinWebhook(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	item, err := db.GetJellyfinItemByPath(path)
	if err != nil {
		t.Fatalf("GetJellyfinItemByPath() failed: %v", err)
	}
	if item == nil {
		t.Fatalf("expected persisted jellyfin item")
	}
	if item.JellyfinItemID != "jf-123" {
		t.Fatalf("expected item id jf-123, got %s", item.JellyfinItemID)
	}
}

func TestWebhookSecretValidation(t *testing.T) {
	s := &Server{
		cfg: &config.Config{
			Jellyfin: config.JellyfinConfig{
				WebhookSecret: "expected-secret",
			},
		},
		playbackLocks: jellyfin.NewPlaybackLockManager(),
		deferredQueue: jellyfin.NewDeferredQueue(),
	}

	reqMissing := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/jellyfin", bytes.NewBufferString(`{"NotificationType":"PlaybackStart"}`))
	wMissing := httptest.NewRecorder()
	s.HandleJellyfinWebhook(wMissing, reqMissing)
	if wMissing.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when webhook secret missing, got %d", wMissing.Code)
	}

	reqWrong := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/jellyfin", bytes.NewBufferString(`{"NotificationType":"PlaybackStart"}`))
	reqWrong.Header.Set("X-Jellywatch-Webhook-Secret", "wrong")
	wWrong := httptest.NewRecorder()
	s.HandleJellyfinWebhook(wWrong, reqWrong)
	if wWrong.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when webhook secret is wrong, got %d", wWrong.Code)
	}

	reqOK := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/jellyfin", bytes.NewBufferString(`{"NotificationType":"PlaybackStart","ItemPath":"/x.mkv"}`))
	reqOK.Header.Set("X-Jellywatch-Webhook-Secret", "expected-secret")
	wOK := httptest.NewRecorder()
	s.HandleJellyfinWebhook(wOK, reqOK)
	if wOK.Code != http.StatusOK {
		t.Fatalf("expected 200 when webhook secret is valid, got %d", wOK.Code)
	}

	// Query-string secret is intentionally not accepted.
	reqQuery := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/jellyfin?secret=expected-secret", bytes.NewBufferString(`{"NotificationType":"PlaybackStart","ItemPath":"/x.mkv"}`))
	wQuery := httptest.NewRecorder()
	s.HandleJellyfinWebhook(wQuery, reqQuery)
	if wQuery.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when secret is only provided in query string, got %d", wQuery.Code)
	}
}

func TestAuthMiddlewareWebhookPublicPath(t *testing.T) {
	server := &Server{
		cfg:      &config.Config{Password: "secret"},
		sessions: NewSessionStore(),
	}

	handler := server.authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/jellyfin", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected webhook path to bypass auth, got %d", w.Code)
	}
}
