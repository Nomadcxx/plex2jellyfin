package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/Nomadcxx/jellywatch/api"
)

const (
	// SessionCookieName is the name of the session cookie
	SessionCookieName = "jellywatch_session"
	// SessionDuration is how long a session is valid
	SessionDuration = 24 * time.Hour
	// SessionTokenLength is the number of bytes in a session token
	SessionTokenLength = 32
)

// Session represents an active user session
type Session struct {
	Token     string
	CreatedAt time.Time
	ExpiresAt time.Time
}

// SessionStore manages active sessions
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

// NewSessionStore creates a new session store
func NewSessionStore() *SessionStore {
	store := &SessionStore{
		sessions: make(map[string]*Session),
	}
	// Start cleanup goroutine
	go store.cleanupExpiredSessions()
	return store
}

// Create creates a new session and returns the token
func (s *SessionStore) Create() string {
	token := generateToken()
	session := &Session{
		Token:     token,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(SessionDuration),
	}

	s.mu.Lock()
	s.sessions[token] = session
	s.mu.Unlock()

	return token
}

// Get returns the session if it exists and is not expired
func (s *SessionStore) Get(token string) (*Session, bool) {
	s.mu.RLock()
	session, exists := s.sessions[token]
	s.mu.RUnlock()

	if !exists {
		return nil, false
	}

	if time.Now().After(session.ExpiresAt) {
		s.Delete(token)
		return nil, false
	}

	return session, true
}

// Delete removes a session
func (s *SessionStore) Delete(token string) {
	s.mu.Lock()
	delete(s.sessions, token)
	s.mu.Unlock()
}

// cleanupExpiredSessions periodically removes expired sessions
func (s *SessionStore) cleanupExpiredSessions() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for token, session := range s.sessions {
			if now.After(session.ExpiresAt) {
				delete(s.sessions, token)
			}
		}
		s.mu.Unlock()
	}
}

// generateToken creates a secure random token
func generateToken() string {
	bytes := make([]byte, SessionTokenLength)
	if _, err := rand.Read(bytes); err != nil {
		// Fall back to timestamp-based token if crypto/rand fails
		return hex.EncodeToString([]byte(time.Now().String()))
	}
	return hex.EncodeToString(bytes)
}

// AuthEnabled checks if authentication is required
func (s *Server) AuthEnabled() bool {
	return s.cfg != nil && s.cfg.Password != ""
}

// IsAuthenticated checks if the request has a valid session
func (s *Server) IsAuthenticated(r *http.Request) bool {
	// If auth is disabled, always authenticated
	if !s.AuthEnabled() {
		return true
	}

	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return false
	}

	if s.sessions == nil {
		return false
	}

	_, valid := s.sessions.Get(cookie.Value)
	return valid
}

// Login implements api.ServerInterface
func (s *Server) Login(w http.ResponseWriter, r *http.Request) {
	// Parse request body
	var req api.LoginJSONRequestBody
	if err := parseJSONBody(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Invalid request body",
		})
		return
	}

	// Check if auth is enabled
	if !s.AuthEnabled() {
		// Auth disabled - always "authenticated"
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success":       true,
			"authenticated": true,
			"enabled":       false,
		})
		return
	}

	// Validate password
	if req.Password != s.cfg.Password {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "Invalid password",
		})
		return
	}

	// Create session
	if s.sessions == nil {
		s.sessions = NewSessionStore()
	}

	token := s.sessions.Create()

	// Set secure cookie
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   false, // Set to true in production with HTTPS
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(SessionDuration.Seconds()),
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

// Logout implements api.ServerInterface
func (s *Server) Logout(w http.ResponseWriter, r *http.Request) {
	// Clear session if it exists
	if cookie, err := r.Cookie(SessionCookieName); err == nil && s.sessions != nil {
		s.sessions.Delete(cookie.Value)
	}

	// Clear cookie
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1, // Delete cookie
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

// GetAuthStatus implements api.ServerInterface
func (s *Server) GetAuthStatus(w http.ResponseWriter, r *http.Request) {
	enabled := s.AuthEnabled()
	authenticated := s.IsAuthenticated(r)

	writeJSON(w, http.StatusOK, api.AuthStatus{
		Enabled:       &enabled,
		Authenticated: &authenticated,
	})
}

// parseJSONBody is a helper to parse JSON request body
func parseJSONBody(r *http.Request, v interface{}) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}