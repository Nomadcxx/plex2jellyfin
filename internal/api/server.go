package api

import (
	"net/http"

	"github.com/Nomadcxx/jellywatch/api"
	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/Nomadcxx/jellywatch/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
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

// Handler returns the HTTP handler
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.SetHeader("Content-Type", "application/json"))

	// Mount API routes
	api.HandlerFromMux(s, r)

	return r
}
