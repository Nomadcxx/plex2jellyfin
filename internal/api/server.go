package api

import (
	"github.com/Nomadcxx/jellywatch"
	"github.com/Nomadcxx/jellywatch/api"
	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

// Server implements the API
type Server struct {
	db      *database.MediaDB
	service *service.CleanupService
}

// NewServer creates a new API server
func NewServer(db *database.MediaDB) *Server {
	return &Server{
		db:      db,
		service: service.NewCleanupService(db),
	}
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

	// Mount generated API routes
	api.HandlerFromMux(s, r)

	return r
}
