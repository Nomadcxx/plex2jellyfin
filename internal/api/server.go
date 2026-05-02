package api

import (
	"net/http"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Nomadcxx/jellywatch"
	"github.com/Nomadcxx/jellywatch/api"
	"github.com/Nomadcxx/jellywatch/internal/activity"
	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/daemon/ipc"
	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/jellyfin"
	"github.com/Nomadcxx/jellywatch/internal/jellyweb/daemonctl"
	"github.com/Nomadcxx/jellywatch/internal/paths"
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
	sessionOnce    sync.Once
	playbackLocks  *jellyfin.PlaybackLockManager
	deferredQueue  *jellyfin.DeferredQueue
	pathTranslator *jellyfin.PathTranslator
	ipc            IPCCaller
	launcher       *daemonctl.Launcher
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

	s := &Server{
		db:             db,
		cfg:            cfg,
		service:        service.NewCleanupService(db),
		activityLogger: activityLogger,
		sessions:       sessions,
		playbackLocks:  jellyfin.NewPlaybackLockManager(),
		deferredQueue:  jellyfin.NewDeferredQueue(),
	}
	if cfg != nil {
		mappings := make([]jellyfin.PathMapping, 0, len(cfg.Jellyfin.PathMappings))
		for _, m := range cfg.Jellyfin.PathMappings {
			mappings = append(mappings, jellyfin.PathMapping{Jellyfin: m.Jellyfin, Daemon: m.Daemon})
		}
		s.pathTranslator = jellyfin.NewPathTranslator(mappings)
	}
	if configDir, err := paths.JellyWatchDir(); err == nil {
		s.ipc = ipc.NewClient(filepath.Join(configDir, "control.sock"))
	}
	return s
}

// SetLauncher attaches a daemon Launcher used by /daemon/{start,restart} routes.
func (s *Server) SetLauncher(l *daemonctl.Launcher) {
	s.launcher = l
}

// Close releases server resources (stops SessionStore cleanup goroutine, etc.)
func (s *Server) Close() {
	if s.sessions != nil {
		s.sessions.Close()
	}
}

// ensureSessionStore initializes the session store exactly once (race-safe).
func (s *Server) ensureSessionStore() {
	s.sessionOnce.Do(func() {
		if s.sessions == nil {
			s.sessions = NewSessionStore()
		}
	})
}

// getConfigDir returns the config directory path using sudo-aware paths package
func getConfigDir() (string, error) {
	return paths.JellyWatchDir()
}

// Handler returns the HTTP handler with CORS, API routes, and static file serving
func (s *Server) Handler() *chi.Mux {
	r := chi.NewRouter()

	// Global middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// CORS middleware. Origins are config-driven so production deployments
	// behind reverse proxies or on non-default ports don't silently fail
	// preflight checks with the previously-hardcoded dev origin.
	allowedOrigins := []string{"http://localhost:3000"}
	if s.cfg != nil && len(s.cfg.API.AllowedOrigins) > 0 {
		allowedOrigins = s.cfg.API.AllowedOrigins
	}
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   allowedOrigins,
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
	r.Post("/paths/preflight", PreflightHandler{}.ServeHTTP)
	testH := &TestHandlers{Cfg: s.cfg}
	r.Post("/settings/sonarr/test", testH.Sonarr)
	r.Post("/settings/radarr/test", testH.Radarr)
	r.Post("/settings/jellyfin/test", testH.Jellyfin)
	r.Post("/ai/test-connection", s.TestAIConnection)
	r.Post("/ai/test-prompt", s.TestAIPrompt)
	r.Get("/ai/models", s.ListAIModels)
	r.Put("/ai/settings", s.UpdateAISettings)

	if s.ipc != nil {
		daemonH := &DaemonHandlers{IPC: s.ipc, Launcher: s.launcher}
		r.Route("/daemon", func(r chi.Router) {
			r.Get("/status", daemonH.Status)
			r.Post("/stop", daemonH.Stop)
			r.Post("/reload", daemonH.Reload)
			r.Post("/start", daemonH.Start)
			r.Post("/restart", daemonH.Restart)
			r.Post("/recover", daemonH.Recover)
		})

		dbH := &DatabaseHandlers{IPC: s.ipc}
		r.Route("/database", func(r chi.Router) {
			r.Post("/rescan", dbH.Rescan)
			r.Post("/reset", dbH.Reset)
			r.Get("/rescan/last", dbH.LastRescans)
		})

		if attacher, ok := s.ipc.(IPCAttacher); ok {
			sse := &SSERelay{IPC: attacher}
			r.Get("/events/op/{op_id}", sse.Stream)
		}

		opsH := &OpsHandlers{IPC: s.ipc}
		r.Route("/ops", func(r chi.Router) {
			r.Get("/", opsH.List)
			r.Post("/{op_id}/cancel", opsH.Cancel)
		})

		deferredH := &DeferredHandlers{IPC: s.ipc}
		r.Get("/deferred", deferredH.List)

		jfH := &JellyfinHandlers{DB: s.db}
		r.Route("/jellyfin", func(r chi.Router) {
			r.Get("/identification", jfH.Identification)
			r.Get("/identification/items", jfH.IdentificationItems)
		})

		opsStream := &StreamingOpHandlers{IPC: s.ipc}
		r.Route("/jobs", func(r chi.Router) {
			r.Post("/consolidate", opsStream.Consolidate)
			r.Post("/duplicates/scan", opsStream.DupScan)
			r.Post("/ai/batch", opsStream.AIBatch)
			r.Post("/metadata/refresh", opsStream.MetadataRefresh)
			r.Post("/sweep", opsStream.Sweep)
			r.Post("/parses/audit", opsStream.ParsesAudit)
		})

		schedH := &SchedulerHandlers{IPC: s.ipc}
		r.Route("/scheduler", func(r chi.Router) {
			r.Get("/jobs", schedH.ListJobs)
			r.Post("/jobs/{name}/run", schedH.RunJob)
			r.Post("/jobs/{name}/stop", schedH.StopJob)
			r.Patch("/jobs/{name}", schedH.UpdateJob)
		})

		hkH := &HousekeepingHandlers{IPC: s.ipc}
		r.Route("/housekeeping", func(r chi.Router) {
			r.Get("/tasks", hkH.ListTasks)
			r.Delete("/tasks", hkH.PurgeTasks)
			r.Post("/tasks/bulk", hkH.BulkAction)
			r.Get("/tasks/{id}", hkH.GetTask)
			r.Post("/tasks/{id}/retry", hkH.RetryTask)
			r.Post("/tasks/{id}/cancel", hkH.CancelTask)
			r.Post("/tasks/{id}/verify", hkH.VerifyTask)
			r.Get("/tasks/{id}/group", hkH.GetTaskGroup)
			r.Post("/tasks/{id}/approve", hkH.ApproveTask)
			r.Post("/verify-flagged", hkH.VerifyFlagged)
		})

		settingsH := &SettingsHandlers{Cfg: s.cfg, IPC: s.ipc}
		pathsH := &PathsHandlers{Cfg: s.cfg, IPC: s.ipc}
		libsH := &LibrariesHandlers{Cfg: s.cfg, IPC: s.ipc}
		r.Route("/settings", func(r chi.Router) {
			r.Get("/{section}", settingsH.Get)
			r.Put("/{section}", settingsH.Put)
			r.Route("/paths", func(r chi.Router) {
				r.Get("/{kind}", pathsH.Get)
				r.Post("/{kind}", pathsH.Add)
				r.Delete("/{kind}/{index}", pathsH.Remove)
				r.Put("/{kind}", pathsH.Replace)
			})
			r.Route("/libraries", func(r chi.Router) {
				r.Get("/{kind}", libsH.Get)
				r.Post("/{kind}", libsH.Add)
				r.Delete("/{kind}/{index}", libsH.Remove)
				r.Put("/{kind}", libsH.Replace)
			})
		})
	}

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
