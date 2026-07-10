package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Nomadcxx/plex2jellyfin/api"
	"github.com/Nomadcxx/plex2jellyfin/internal/config"
)

func TestAPIRouterRejectsLargeBodies(t *testing.T) {
	server := &Server{cfg: &config.Config{}}
	body := strings.NewReader(`{"password":"` + strings.Repeat("x", int(maxRequestBodyBytes)) + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/auth/login", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	server.apiRouter().ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 for oversized API body, got %d body %s", w.Code, w.Body.String())
	}
}

func TestAuthEnabled(t *testing.T) {
	tests := []struct {
		name     string
		password string
		expected bool
	}{
		{
			name:     "no password - auth disabled",
			password: "",
			expected: false,
		},
		{
			name:     "password set - auth enabled",
			password: "secret",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{Password: tt.password}
			server := &Server{cfg: cfg}
			result := server.AuthEnabled()
			if result != tt.expected {
				t.Errorf("AuthEnabled() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestSessionStore(t *testing.T) {
	store := NewSessionStore()

	// Test create session
	token, err := store.Create()
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if token == "" {
		t.Error("Create() returned empty token")
	}

	// Test get session
	session, exists := store.Get(token)
	if !exists {
		t.Error("Get() returned false for valid token")
	}
	if session.Token != token {
		t.Errorf("Get() returned wrong token: got %v, want %v", session.Token, token)
	}

	// Test delete session
	store.Delete(token)
	_, exists = store.Get(token)
	if exists {
		t.Error("Get() returned true after Delete()")
	}
}

func TestGenerateTokenFailsClosedWhenRandomFails(t *testing.T) {
	orig := secureRandomRead
	secureRandomRead = func([]byte) (int, error) {
		return 0, errors.New("entropy unavailable")
	}
	defer func() { secureRandomRead = orig }()

	if token, err := generateToken(); err == nil || token != "" {
		t.Fatalf("expected no token when random fails, got token=%q err=%v", token, err)
	}
}

func TestGetAuthStatus(t *testing.T) {
	tests := []struct {
		name        string
		password    string
		cookieValue string
		wantEnabled bool
		wantAuthed  bool
	}{
		{
			name:        "auth disabled - always authenticated",
			password:    "",
			cookieValue: "",
			wantEnabled: false,
			wantAuthed:  true,
		},
		{
			name:        "auth enabled - no cookie - not authenticated",
			password:    "secret",
			cookieValue: "",
			wantEnabled: true,
			wantAuthed:  false,
		},
		{
			name:        "auth enabled - invalid cookie - not authenticated",
			password:    "secret",
			cookieValue: "invalid-token",
			wantEnabled: true,
			wantAuthed:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{Password: tt.password}
			server := &Server{
				cfg:      cfg,
				sessions: NewSessionStore(),
			}

			req := httptest.NewRequest("GET", "/api/v1/auth/status", nil)
			if tt.cookieValue != "" {
				req.AddCookie(&http.Cookie{
					Name:  SessionCookieName,
					Value: tt.cookieValue,
				})
			}

			w := httptest.NewRecorder()
			server.GetAuthStatus(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("GetAuthStatus() status = %d, want %d", w.Code, http.StatusOK)
			}

			var status api.AuthStatus
			if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
				t.Fatalf("Failed to parse response: %v", err)
			}

			if *status.Enabled != tt.wantEnabled {
				t.Errorf("Enabled = %v, want %v", *status.Enabled, tt.wantEnabled)
			}
			if *status.Authenticated != tt.wantAuthed {
				t.Errorf("Authenticated = %v, want %v", *status.Authenticated, tt.wantAuthed)
			}
		})
	}
}

func TestLogin(t *testing.T) {
	tests := []struct {
		name        string
		password    string
		loginPass   string
		wantStatus  int
		wantSuccess bool
	}{
		{
			name:        "auth disabled - any password works",
			password:    "",
			loginPass:   "anything",
			wantStatus:  http.StatusOK,
			wantSuccess: true,
		},
		{
			name:        "auth enabled - correct password",
			password:    "secret",
			loginPass:   "secret",
			wantStatus:  http.StatusOK,
			wantSuccess: true,
		},
		{
			name:        "auth enabled - wrong password",
			password:    "secret",
			loginPass:   "wrong",
			wantStatus:  http.StatusUnauthorized,
			wantSuccess: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{Password: tt.password}
			server := &Server{
				cfg:      cfg,
				sessions: NewSessionStore(),
			}

			body := `{"password":"` + tt.loginPass + `"}`
			req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			server.Login(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("Login() status = %d, want %d", w.Code, tt.wantStatus)
			}

			if tt.wantStatus == http.StatusOK {
				// Check for session cookie
				cookies := w.Result().Cookies()
				var foundCookie bool
				for _, c := range cookies {
					if c.Name == SessionCookieName {
						foundCookie = true
						if c.Value == "" {
							t.Error("Session cookie has empty value")
						}
						break
					}
				}
				if !foundCookie && tt.password != "" {
					t.Error("No session cookie set on successful login")
				}
			}
		})
	}
}

func TestLoginAcceptsPasswordHash(t *testing.T) {
	hash, err := config.HashPassword("secret")
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		cfg:      &config.Config{PasswordHash: hash},
		sessions: NewSessionStore(),
	}

	req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(`{"password":"secret"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.Login(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected hashed password login to succeed, got %d body %s", w.Code, w.Body.String())
	}
}

func TestLoginRateLimitsFailedAttempts(t *testing.T) {
	server := &Server{
		cfg:      &config.Config{Password: "secret"},
		sessions: NewSessionStore(),
	}

	for i := 0; i < maxLoginFailures; i++ {
		req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(`{"password":"wrong"}`))
		req.RemoteAddr = "192.0.2.10:12345"
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		server.Login(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected 401, got %d", i+1, w.Code)
		}
	}

	req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(`{"password":"wrong"}`))
	req.RemoteAddr = "192.0.2.10:12345"
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.Login(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after repeated failures, got %d body %s", w.Code, w.Body.String())
	}
}

func TestLogout(t *testing.T) {
	cfg := &config.Config{Password: "secret"}
	store := NewSessionStore()
	token, err := store.Create()
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		cfg:      cfg,
		sessions: store,
	}

	req := httptest.NewRequest("POST", "/api/v1/auth/logout", nil)
	req.AddCookie(&http.Cookie{
		Name:  SessionCookieName,
		Value: token,
	})

	w := httptest.NewRecorder()
	server.Logout(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Logout() status = %d, want %d", w.Code, http.StatusOK)
	}

	// Check session was deleted
	_, exists := store.Get(token)
	if exists {
		t.Error("Session still exists after logout")
	}

	// Check cookie is cleared
	cookies := w.Result().Cookies()
	for _, c := range cookies {
		if c.Name == SessionCookieName && c.MaxAge != -1 {
			t.Error("Session cookie not set to delete (MaxAge should be -1)")
		}
	}
}

func TestAuthMiddleware(t *testing.T) {
	tests := []struct {
		name       string
		password   string
		path       string
		cookie     string
		wantStatus int
	}{
		{
			name:       "public path - no auth needed",
			password:   "secret",
			path:       "/auth/status",
			cookie:     "",
			wantStatus: http.StatusOK,
		},
		{
			name:       "health endpoint - no auth needed",
			password:   "secret",
			path:       "/health",
			cookie:     "",
			wantStatus: http.StatusOK,
		},
		{
			name:       "protected path - auth disabled",
			password:   "",
			path:       "/dashboard",
			cookie:     "",
			wantStatus: http.StatusOK,
		},
		{
			name:       "protected path - no cookie - unauthorized",
			password:   "secret",
			path:       "/dashboard",
			cookie:     "",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "protected path - invalid cookie - unauthorized",
			password:   "secret",
			path:       "/dashboard",
			cookie:     "invalid",
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{Password: tt.password}
			store := NewSessionStore()

			// If we need a valid cookie, create a session
			if tt.cookie == "valid" {
				token, err := store.Create()
				if err != nil {
					t.Fatal(err)
				}
				tt.cookie = token
			}

			server := &Server{
				cfg:      cfg,
				sessions: store,
			}

			// Create a test handler that just returns 200
			handler := server.authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest("GET", tt.path, nil)
			if tt.cookie != "" {
				req.AddCookie(&http.Cookie{
					Name:  SessionCookieName,
					Value: tt.cookie,
				})
			}

			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("authMiddleware() status = %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}

func newAuthLifecycleServer(t *testing.T, cfg *config.Config) *Server {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SUDO_USER", "")
	return &Server{cfg: cfg}
}

func postJSON(t *testing.T, h http.Handler, path, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func sessionCookie(t *testing.T, w *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range w.Result().Cookies() {
		if c.Name == SessionCookieName && c.Value != "" {
			return c
		}
	}
	t.Fatal("expected session cookie in response")
	return nil
}

func TestSetupAuthFirstRun(t *testing.T) {
	server := newAuthLifecycleServer(t, &config.Config{})
	router := server.apiRouter()

	w := postJSON(t, router, "/auth/setup", `{"password":"correct horse"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("setup status = %d body %s", w.Code, w.Body.String())
	}
	sessionCookie(t, w)

	if !server.AuthEnabled() {
		t.Error("AuthEnabled() = false after setup")
	}
	if server.cfg.PasswordHash == "" {
		t.Error("PasswordHash not set after setup")
	}
	if !server.verifyLoginPassword("correct horse") {
		t.Error("configured password does not verify")
	}
}

func TestSetupAuthRejectsWhenConfigured(t *testing.T) {
	hash, err := config.HashPassword("existing-password")
	if err != nil {
		t.Fatal(err)
	}
	server := newAuthLifecycleServer(t, &config.Config{PasswordHash: hash})

	w := postJSON(t, server.apiRouter(), "/auth/setup", `{"password":"new password"}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("setup on configured server status = %d, want 409", w.Code)
	}
}

func TestSetupAuthRejectsShortPassword(t *testing.T) {
	server := newAuthLifecycleServer(t, &config.Config{})

	w := postJSON(t, server.apiRouter(), "/auth/setup", `{"password":"short"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("short password status = %d, want 400", w.Code)
	}
	if server.AuthEnabled() {
		t.Error("AuthEnabled() = true after rejected setup")
	}
}

func TestChangePasswordLifecycle(t *testing.T) {
	server := newAuthLifecycleServer(t, &config.Config{})
	router := server.apiRouter()

	// First-run setup, keep the session cookie.
	w := postJSON(t, router, "/auth/setup", `{"password":"original-pass"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("setup status = %d", w.Code)
	}
	oldCookie := sessionCookie(t, w)

	// Wrong current password is rejected.
	w = postJSON(t, router, "/auth/password",
		`{"current_password":"wrong","new_password":"replacement-pass"}`, oldCookie)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("wrong current password status = %d, want 401", w.Code)
	}

	// Short new password is rejected.
	w = postJSON(t, router, "/auth/password",
		`{"current_password":"original-pass","new_password":"tiny"}`, oldCookie)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("short new password status = %d, want 400", w.Code)
	}

	// Valid change succeeds and rotates the session.
	w = postJSON(t, router, "/auth/password",
		`{"current_password":"original-pass","new_password":"replacement-pass"}`, oldCookie)
	if w.Code != http.StatusOK {
		t.Fatalf("change password status = %d body %s", w.Code, w.Body.String())
	}
	newCookie := sessionCookie(t, w)

	if !server.verifyLoginPassword("replacement-pass") {
		t.Error("new password does not verify")
	}
	if server.verifyLoginPassword("original-pass") {
		t.Error("old password still verifies")
	}

	// Old session was invalidated; the fresh one works.
	req := httptest.NewRequest(http.MethodPost, "/auth/password", strings.NewReader(
		`{"current_password":"replacement-pass","new_password":"replacement-pass2"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(oldCookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("old session after change status = %d, want 401", rec.Code)
	}

	w = postJSON(t, router, "/auth/password",
		`{"current_password":"replacement-pass","new_password":"replacement-pass2"}`, newCookie)
	if w.Code != http.StatusOK {
		t.Errorf("fresh session change status = %d body %s", w.Code, w.Body.String())
	}
}

func TestChangePasswordRequiresAuthConfigured(t *testing.T) {
	server := newAuthLifecycleServer(t, &config.Config{})

	w := postJSON(t, server.apiRouter(), "/auth/password",
		`{"current_password":"","new_password":"whatever-pass"}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("change without setup status = %d, want 409", w.Code)
	}
}
