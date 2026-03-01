package api

import (
	"net/http"
	"os"
	"strings"

	"github.com/Nomadcxx/jellywatch"
	"github.com/Nomadcxx/jellywatch/api"
	"github.com/Nomadcxx/jellywatch/internal/activity"
	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/jellyfin"
	"github.com/Nomadcxx/jellywatch/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

// Server implements the API
type Server struct {
	db             *database.MediaDB
	cfg            *config.Config
	service        *service.CleanupService
	activityLogger *activity.Logger
	sessions       *SessionStore
	playbackLocks  *jellyfin.PlaybackLockManager
	deferredQueue  *jellyfin.DeferredQueue
}

// NewServer creates a new API server
func NewServer(db *database.MediaDB, cfg *config.Config) *Server {
	// Initialize activity logger
	configDir, err := getConfigDir()
	var activityLogger *activity.Logger
	if err == nil {
		activityLogger, _ = activity.NewLogger(configDir)
	}

	// Initialize session store if auth is enabled
	var sessions *SessionStore
	if cfg != nil && cfg.Password != "" {
		sessions = NewSessionStore()
	}

	return &Server{
		db:             db,
		cfg:            cfg,
		service:        service.NewCleanupService(db),
		activityLogger: activityLogger,
		sessions:       sessions,
		playbackLocks:  jellyfin.NewPlaybackLockManager(),
		deferredQueue:  jellyfin.NewDeferredQueue(),
	}
}

// getConfigDir returns the config directory path
func getConfigDir() (string, error) {
	// Use the paths package to get the config directory
	homeDir, err := getHomeDir()
	if err != nil {
		return "", err
	}
	return homeDir + "/.config/jellywatch", nil
}

// getHomeDir returns the user's home directory
func getHomeDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return home, nil
}

// Handler returns the HTTP handler with CORS, API routes, and static file serving
func (s *Server) Handler() *chi.Mux {
	r := chi.NewRouter()

	// Global middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// CORS middleware for development
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://localhost:3000"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Mount API routes at /api/v1
	r.Mount("/api/v1", s.apiRouter())

	// Serve static files with SPA fallback
	webFS := jellywatch.GetWebFS()
	r.Get("/*", spaFileServer(webFS))

	return r
}

// apiRouter returns a router with API routes
func (s *Server) apiRouter() *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.SetHeader("Content-Type", "application/json"))
	r.Use(s.authMiddleware)

	// Webhooks are intentionally mounted outside generated OpenAPI handlers.
	r.Post("/webhooks/jellyfin", s.HandleJellyfinWebhook)

	// Mount generated API routes
	api.HandlerFromMux(s, r)

	return r
}

// authMiddleware checks if authentication is required and validates session
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Public paths that don't require authentication
		publicPaths := []string{
			"/auth/login",
			"/auth/logout",
			"/auth/status",
			"/health",
			"/webhooks/jellyfin",
		}

		path := r.URL.Path

		// Check if path is public
		for _, prefix := range publicPaths {
			if strings.HasSuffix(path, prefix) {
				next.ServeHTTP(w, r)
				return
			}
		}

		// If auth is not enabled, allow all requests
		if !s.AuthEnabled() {
			next.ServeHTTP(w, r)
			return
		}

		// Check if authenticated
		if !s.IsAuthenticated(r) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"error": "Authentication required",
				"code":  "unauthorized",
			})
			return
		}

		next.ServeHTTP(w, r)
	})
}
