package api

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/Nomadcxx/jellywatch/api"
	"github.com/Nomadcxx/jellywatch/internal/config"
)

const (
	// SessionCookieName is the name of the session cookie
	SessionCookieName = "jellywatch_session"
	// SessionDuration is how long a session is valid
	SessionDuration = 24 * time.Hour
	// SessionTokenLength is the number of bytes in a session token
	SessionTokenLength = 32
	maxLoginFailures   = 5
	loginWindow        = 5 * time.Minute
	loginLockout       = 15 * time.Minute
)

var secureRandomRead = rand.Read

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
	stopCh   chan struct{}
	once     sync.Once
}

type loginAttemptState struct {
	failures    int
	firstFailed time.Time
	lockedUntil time.Time
}

type loginRateLimiter struct {
	mu       sync.Mutex
	attempts map[string]loginAttemptState
}

func newLoginRateLimiter() *loginRateLimiter {
	return &loginRateLimiter{attempts: map[string]loginAttemptState{}}
}

func (l *loginRateLimiter) allow(key string, now time.Time) bool {
	if key == "" {
		key = "unknown"
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	state := l.attempts[key]
	return state.lockedUntil.IsZero() || now.After(state.lockedUntil)
}

func (l *loginRateLimiter) recordFailure(key string, now time.Time) {
	if key == "" {
		key = "unknown"
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	state := l.attempts[key]
	if state.firstFailed.IsZero() || now.Sub(state.firstFailed) > loginWindow {
		state = loginAttemptState{firstFailed: now}
	}
	state.failures++
	if state.failures >= maxLoginFailures {
		state.lockedUntil = now.Add(loginLockout)
	}
	l.attempts[key] = state
}

func (l *loginRateLimiter) recordSuccess(key string) {
	if key == "" {
		key = "unknown"
	}
	l.mu.Lock()
	delete(l.attempts, key)
	l.mu.Unlock()
}

// NewSessionStore creates a new session store
func NewSessionStore() *SessionStore {
	store := &SessionStore{
		sessions: make(map[string]*Session),
		stopCh:   make(chan struct{}),
	}
	// Start cleanup goroutine
	go store.cleanupExpiredSessions()
	return store
}

// Close stops the cleanup goroutine
func (s *SessionStore) Close() {
	s.once.Do(func() {
		close(s.stopCh)
	})
}

// Create creates a new session and returns the token
func (s *SessionStore) Create() (string, error) {
	token, err := generateToken()
	if err != nil {
		return "", err
	}
	session := &Session{
		Token:     token,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(SessionDuration),
	}

	s.mu.Lock()
	s.sessions[token] = session
	s.mu.Unlock()

	return token, nil
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

	for {
		select {
		case <-ticker.C:
			s.mu.Lock()
			now := time.Now()
			for token, session := range s.sessions {
				if now.After(session.ExpiresAt) {
					delete(s.sessions, token)
				}
			}
			s.mu.Unlock()
		case <-s.stopCh:
			return
		}
	}
}

// generateToken creates a secure random token
func generateToken() (string, error) {
	bytes := make([]byte, SessionTokenLength)
	if _, err := secureRandomRead(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// AuthEnabled checks if authentication is required
func (s *Server) AuthEnabled() bool {
	return s.cfg != nil && (s.cfg.Password != "" || s.cfg.PasswordHash != "")
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

// isSecureRequest checks if the request came over HTTPS (directly or via proxy)
func isSecureRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if r.Header.Get("X-Forwarded-Proto") == "https" {
		return true
	}
	return false
}

// shouldSetSecureCookie decides whether to set the Secure flag on session
// cookies. Config takes precedence: if `secure_cookies = true` is set,
// always force Secure (operator-asserted). Otherwise auto-detect from the
// request itself, which covers local HTTP dev and HTTPS-via-proxy prod.
func (s *Server) shouldSetSecureCookie(r *http.Request) bool {
	if s.cfg != nil && s.cfg.SecureCookies {
		return true
	}
	return isSecureRequest(r)
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

	limiter := s.ensureLoginLimiter()
	remoteKey := loginRateLimitKey(r)
	if !limiter.allow(remoteKey, time.Now()) {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{
			"error": "Too many failed login attempts",
		})
		return
	}

	// Validate password
	if !s.verifyLoginPassword(req.Password) {
		limiter.recordFailure(remoteKey, time.Now())
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "Invalid password",
		})
		return
	}
	limiter.recordSuccess(remoteKey)

	// Create session (race-safe lazy initialization)
	s.ensureSessionStore()

	token, err := s.sessions.Create()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "Could not create secure session",
		})
		return
	}

	// Set secure cookie
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.shouldSetSecureCookie(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(SessionDuration.Seconds()),
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

func (s *Server) verifyLoginPassword(password string) bool {
	if s == nil || s.cfg == nil {
		return false
	}
	if s.cfg.PasswordHash != "" {
		return config.VerifyPassword(password, s.cfg.PasswordHash)
	}
	return subtle.ConstantTimeCompare([]byte(password), []byte(s.cfg.Password)) == 1
}

func loginRateLimitKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || host == "" {
		return r.RemoteAddr
	}
	return host
}

func (s *Server) ensureLoginLimiter() *loginRateLimiter {
	s.loginLimiterOnce.Do(func() {
		s.loginLimiter = newLoginRateLimiter()
	})
	return s.loginLimiter
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
		Secure:   s.shouldSetSecureCookie(r),
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
