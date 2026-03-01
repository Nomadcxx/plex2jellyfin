package daemon

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/jellyfin"
	"github.com/Nomadcxx/jellywatch/internal/logging"
	"github.com/Nomadcxx/jellywatch/internal/scanner"
)

type Server struct {
	httpServer    *http.Server
	handler       *MediaHandler
	scanner       *scanner.PeriodicScanner
	startTime     time.Time
	mu            sync.RWMutex
	healthy       bool
	logger        *logging.Logger
	webhookSecret string
}

type HealthResponse struct {
	Status        string                 `json:"status"`
	Uptime        string                 `json:"uptime"`
	Timestamp     time.Time              `json:"timestamp"`
	ScannerStatus *scanner.ScannerStatus `json:"scanner,omitempty"`
}

type MetricsResponse struct {
	MoviesProcessed  int64   `json:"movies_processed"`
	TVProcessed      int64   `json:"tv_processed"`
	TotalProcessed   int64   `json:"total_processed"`
	BytesTransferred int64   `json:"bytes_transferred"`
	BytesTransferMB  float64 `json:"bytes_transferred_mb"`
	Errors           int64   `json:"errors"`
	UptimeSeconds    float64 `json:"uptime_seconds"`
	LastProcessed    string  `json:"last_processed,omitempty"`
}

func NewServer(handler *MediaHandler, periodicScanner *scanner.PeriodicScanner, addr string, logger *logging.Logger, webhookSecret string) *Server {
	if logger == nil {
		logger = logging.Nop()
	}
	s := &Server{
		handler:       handler,
		scanner:       periodicScanner,
		startTime:     time.Now(),
		healthy:       true,
		logger:        logger,
		webhookSecret: strings.TrimSpace(webhookSecret),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/ready", s.handleReady)
	mux.HandleFunc("/metrics", s.handleMetrics)
	mux.HandleFunc("/stats", s.handleMetrics)
	mux.HandleFunc("/api/v1/webhooks/jellyfin", s.handleJellyfinWebhook)

	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return s
}

func (s *Server) Start() error {
	s.logger.Info("server", "Health server starting", logging.F("addr", s.httpServer.Addr))
	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("health server error: %w", err)
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) SetHealthy(healthy bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.healthy = healthy
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	healthy := s.healthy
	s.mu.RUnlock()

	// Check scanner health too
	scannerHealthy := true
	var scannerStatus *scanner.ScannerStatus
	if s.scanner != nil {
		scannerHealthy = s.scanner.IsHealthy()
		status := s.scanner.Status()
		scannerStatus = &status
	}

	overallHealthy := healthy && scannerHealthy

	response := HealthResponse{
		Uptime:        time.Since(s.startTime).Round(time.Second).String(),
		Timestamp:     time.Now(),
		ScannerStatus: scannerStatus,
	}

	if overallHealthy {
		response.Status = "healthy"
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	} else if healthy && !scannerHealthy {
		response.Status = "degraded"
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK) // Degraded but still serving
	} else {
		response.Status = "unhealthy"
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	json.NewEncoder(w).Encode(response)
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	healthy := s.healthy
	s.mu.RUnlock()

	if healthy {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ready"))
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("not ready"))
	}
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	stats := s.handler.Stats()

	response := MetricsResponse{
		MoviesProcessed:  stats.MoviesProcessed,
		TVProcessed:      stats.TVProcessed,
		TotalProcessed:   stats.MoviesProcessed + stats.TVProcessed,
		BytesTransferred: stats.BytesTransferred,
		BytesTransferMB:  float64(stats.BytesTransferred) / (1024 * 1024),
		Errors:           stats.Errors,
		UptimeSeconds:    stats.Uptime.Seconds(),
	}

	if !stats.LastProcessed.IsZero() {
		response.LastProcessed = stats.LastProcessed.Format(time.RFC3339)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) handleJellyfinWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !s.validateWebhookSecret(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var event jellyfin.WebhookEvent
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	if s.handler != nil {
		s.handler.HandleJellyfinWebhookEvent(event)
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) validateWebhookSecret(r *http.Request) bool {
	if s.webhookSecret == "" {
		return isLoopbackRequest(r)
	}
	provided := strings.TrimSpace(r.Header.Get("X-Jellywatch-Webhook-Secret"))
	if provided == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(s.webhookSecret)) == 1
}

func isLoopbackRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
