package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

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

	for _, notificationType := range []string{"ItemAdded", "ItemUpdated"} {
		reqEvent := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/jellyfin", bytes.NewBufferString(`{"NotificationType":"`+notificationType+`","ItemPath":"/x.mkv","ItemId":"jf-1"}`))
		reqEvent.Header.Set("X-Jellywatch-Webhook-Secret", "expected-secret")
		wEvent := httptest.NewRecorder()
		s.HandleJellyfinWebhook(wEvent, reqEvent)
		if wEvent.Code != http.StatusOK {
			t.Fatalf("expected 200 for %s with valid webhook secret, got %d", notificationType, wEvent.Code)
		}
	}

	// Query-string secret is intentionally not accepted.
	reqQuery := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/jellyfin?secret=expected-secret", bytes.NewBufferString(`{"NotificationType":"PlaybackStart","ItemPath":"/x.mkv"}`))
	wQuery := httptest.NewRecorder()
	s.HandleJellyfinWebhook(wQuery, reqQuery)
	if wQuery.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when secret is only provided in query string, got %d", wQuery.Code)
	}
}

func TestWebhookSecretValidation_EmptySecretLoopbackDenied(t *testing.T) {
	s := &Server{
		cfg: &config.Config{
			Jellyfin: config.JellyfinConfig{
				WebhookSecret: "",
			},
		},
		playbackLocks: jellyfin.NewPlaybackLockManager(),
		deferredQueue: jellyfin.NewDeferredQueue(),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/jellyfin", bytes.NewBufferString(`{"NotificationType":"PlaybackStart","ItemPath":"/x.mkv"}`))
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	s.HandleJellyfinWebhook(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for loopback request when webhook secret is empty, got %d", w.Code)
	}
}

func TestWebhookSecretValidation_EmptySecretNonLoopbackDenied(t *testing.T) {
	s := &Server{
		cfg: &config.Config{
			Jellyfin: config.JellyfinConfig{
				WebhookSecret: "",
			},
		},
		playbackLocks: jellyfin.NewPlaybackLockManager(),
		deferredQueue: jellyfin.NewDeferredQueue(),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/jellyfin", bytes.NewBufferString(`{"NotificationType":"PlaybackStart","ItemPath":"/x.mkv"}`))
	req.RemoteAddr = "192.168.1.50:12345"
	w := httptest.NewRecorder()
	s.HandleJellyfinWebhook(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for non-loopback request when webhook secret is empty, got %d", w.Code)
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

func TestWebhookAuth_FullFlowWithGeneratedSecret(t *testing.T) {
	secret, err := config.GenerateWebhookSecret()
	if err != nil {
		t.Fatalf("GenerateWebhookSecret() failed: %v", err)
	}

	s := &Server{
		cfg: &config.Config{
			Jellyfin: config.JellyfinConfig{WebhookSecret: secret},
		},
		playbackLocks: jellyfin.NewPlaybackLockManager(),
		deferredQueue: jellyfin.NewDeferredQueue(),
	}

	body := `{"NotificationType":"PlaybackStart","ItemPath":"/library/TV/The Pitt/The Pitt S02E08.mkv"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/jellyfin", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Jellywatch-Webhook-Secret", secret)
	req.RemoteAddr = "10.0.0.50:54321"

	w := httptest.NewRecorder()
	s.HandleJellyfinWebhook(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with valid generated secret, got %d", w.Code)
	}
}

func TestWebhookFlatPayload(t *testing.T) {
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

func TestWebhookNestedPayload(t *testing.T) {
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

func TestHandleItemAdded_UpdatesParseDecision(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "api-decision.db")
	db, err := database.OpenPath(dbPath)
	if err != nil {
		t.Fatalf("OpenPath: %v", err)
	}
	defer db.Close()

	targetPath := "/library/Movies/The Matrix (1999)/The Matrix (1999).mkv"
	id, err := db.InsertDecision(database.ParseDecision{
		SourcePath:      "/downloads/the.matrix.1999.mkv",
		SourceFilename:  "the.matrix.1999.mkv",
		EventAt:         time.Now().UTC(),
		TargetPath:      targetPath,
		OrganizeOutcome: "success",
	})
	if err != nil {
		t.Fatalf("InsertDecision: %v", err)
	}

	s := &Server{
		cfg: &config.Config{
			Jellyfin: config.JellyfinConfig{WebhookSecret: "test-secret"},
		},
		db:            db,
		playbackLocks: jellyfin.NewPlaybackLockManager(),
		deferredQueue: jellyfin.NewDeferredQueue(),
	}

	payload := []byte(`{"NotificationType":"ItemAdded","ItemPath":"` + targetPath + `","ItemId":"jf-api-1","Name":"The Matrix","ItemType":"Movie","Provider_imdb":"tt0133093","Provider_tmdb":"603"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/jellyfin", bytes.NewReader(payload))
	req.Header.Set("X-Jellywatch-Webhook-Secret", "test-secret")
	w := httptest.NewRecorder()
	s.HandleJellyfinWebhook(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	dec, err := db.GetDecision(id)
	if err != nil {
		t.Fatalf("GetDecision: %v", err)
	}
	if dec.JellyfinItemID != "jf-api-1" {
		t.Fatalf("expected JellyfinItemID=jf-api-1, got %q", dec.JellyfinItemID)
	}
	if dec.JellyfinImdbID != "tt0133093" {
		t.Fatalf("expected JellyfinImdbID=tt0133093, got %q", dec.JellyfinImdbID)
	}
	if dec.JellyfinTmdbID != "603" {
		t.Fatalf("expected JellyfinTmdbID=603, got %q", dec.JellyfinTmdbID)
	}
	if dec.JellyfinResolvedAt == nil {
		t.Fatalf("expected JellyfinResolvedAt to be set")
	}
}
