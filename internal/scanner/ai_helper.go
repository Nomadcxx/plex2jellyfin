package scanner

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/ai"
	"github.com/Nomadcxx/jellywatch/internal/config"
)

var ErrCircuitOpen = errors.New("circuit breaker is open")

// AIHelper manages AI parsing with circuit breaker and caching
type AIHelper struct {
	matcher *ai.Matcher
	cache   *ai.Cache
	cfg     config.AIConfig

	// Circuit breaker state
	mu           sync.Mutex
	failures     int
	firstFailure time.Time
	circuitOpen  bool
	openedAt     time.Time
}

// NewAIHelper creates a new AI helper
func NewAIHelper(cfg config.AIConfig, db *sql.DB, matcher *ai.Matcher) *AIHelper {
	var cache *ai.Cache
	if db != nil && cfg.CacheEnabled {
		cache = ai.NewCache(db)
	}

	return &AIHelper{
		matcher: matcher,
		cache:   cache,
		cfg:     cfg,
	}
}

// TryParse attempts to parse filename with AI, respecting circuit breaker and cache
// Returns: result, fromCache, error
func (h *AIHelper) TryParse(ctx context.Context, filename, mediaType string) (*ai.Result, bool, error) {
	// Check circuit breaker
	if h.IsCircuitOpen() {
		return nil, false, ErrCircuitOpen
	}

	// Check cache first
	if h.cache != nil {
		normalized := ai.NormalizeInput(filename)
		cached, err := h.cache.Get(normalized, mediaType, h.cfg.Model)
		if err == nil && cached != nil {
			return cached, true, nil
		}
	}

	// Call AI matcher
	if h.matcher == nil {
		return nil, false, errors.New("AI matcher not initialized")
	}

	result, err := h.matcher.ParseWithRetry(ctx, filename)
	if err != nil {
		h.RecordFailure()
		return nil, false, err
	}

	// Cache successful result
	if h.cache != nil {
		normalized := ai.NormalizeInput(filename)
		h.cache.Put(normalized, mediaType, h.cfg.Model, result, 0)
	}

	h.ResetFailures()
	return result, false, nil
}

// IsCircuitOpen checks if circuit breaker is open
func (h *AIHelper) IsCircuitOpen() bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.circuitOpen {
		return false
	}

	// Check if cooldown has passed
	cooldown := time.Duration(h.cfg.CircuitBreaker.CooldownSeconds) * time.Second
	if time.Since(h.openedAt) >= cooldown {
		h.circuitOpen = false
		h.failures = 0
		return false
	}

	return true
}

// RecordFailure records an AI failure for circuit breaker
func (h *AIHelper) RecordFailure() {
	h.mu.Lock()
	defer h.mu.Unlock()

	now := time.Now()
	window := time.Duration(h.cfg.CircuitBreaker.FailureWindowSeconds) * time.Second

	// Reset if outside failure window
	if h.failures > 0 && now.Sub(h.firstFailure) > window {
		h.failures = 0
	}

	if h.failures == 0 {
		h.firstFailure = now
	}

	h.failures++

	// Open circuit if threshold exceeded
	if h.failures >= h.cfg.CircuitBreaker.FailureThreshold {
		h.circuitOpen = true
		h.openedAt = now
	}
}

// ResetFailures resets failure counter (call after successful AI call)
func (h *AIHelper) ResetFailures() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.failures = 0
}

// IsEnabled returns true if AI is enabled and matcher is available
func (h *AIHelper) IsEnabled() bool {
	return h.cfg.Enabled && h.matcher != nil
}

// GetAutoTriggerThreshold returns threshold for auto-triggering AI
func (h *AIHelper) GetAutoTriggerThreshold() float64 {
	return h.cfg.AutoTriggerThreshold
}

// GetConfidenceThreshold returns minimum AI confidence to accept
func (h *AIHelper) GetConfidenceThreshold() float64 {
	return h.cfg.ConfidenceThreshold
}
