package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Nomadcxx/jellywatch/api"
	"github.com/Nomadcxx/jellywatch/internal/config"
)

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
	token := store.Create()
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

func TestGetAuthStatus(t *testing.T) {
	tests := []struct {
		name         string
		password     string
		cookieValue  string
		wantEnabled  bool
		wantAuthed   bool
	}{
		{
			name:         "auth disabled - always authenticated",
			password:     "",
			cookieValue:  "",
			wantEnabled:  false,
			wantAuthed:   true,
		},
		{
			name:         "auth enabled - no cookie - not authenticated",
			password:     "secret",
			cookieValue:  "",
			wantEnabled:  true,
			wantAuthed:   false,
		},
		{
			name:         "auth enabled - invalid cookie - not authenticated",
			password:     "secret",
			cookieValue:  "invalid-token",
			wantEnabled:  true,
			wantAuthed:   false,
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
		name         string
		password     string
		loginPass    string
		wantStatus   int
		wantSuccess  bool
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

func TestLogout(t *testing.T) {
	cfg := &config.Config{Password: "secret"}
	store := NewSessionStore()
	token := store.Create()
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
				tt.cookie = store.Create()
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