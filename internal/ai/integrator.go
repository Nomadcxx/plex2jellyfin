package ai

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/config"
)

type ParseSource int

const (
	SourceRegex ParseSource = iota
	SourceCache
	SourceAI
)

func (s ParseSource) String() string {
	switch s {
	case SourceRegex:
		return "regex"
	case SourceCache:
		return "cache"
	case SourceAI:
		return "ai"
	default:
		return "unknown"
	}
}

type Integrator struct {
	enabled bool
	config  config.AIConfig

	matcher *Matcher
	cache   *Cache

	circuit   *CircuitBreaker
	keepalive *Keepalive

	bgQueue  *BackgroundQueue
	bgWg     sync.WaitGroup
	bgCtx    context.Context
	bgCancel context.CancelFunc

	status *AIStatus

	db         *sql.DB
	shutdownCh chan struct{}
	shutdownMu sync.RWMutex
	shutdown   bool
}

type DatabaseProvider interface {
	DB() *sql.DB
}

func NewIntegrator(cfg config.AIConfig, dbProvider DatabaseProvider) (*Integrator, error) {
	shutdownCh := make(chan struct{})
	bgCtx, bgCancel := context.WithCancel(context.Background())

	circuit := NewCircuitBreaker(
		cfg.CircuitBreaker.FailureThreshold,
		time.Duration(cfg.CircuitBreaker.FailureWindowSeconds)*time.Second,
		time.Duration(cfg.CircuitBreaker.CooldownSeconds)*time.Second,
	)

	if !cfg.Enabled {
		return &Integrator{
			enabled:    false,
			config:     cfg,
			circuit:    circuit,
			status:     NewAIStatus(),
			shutdownCh: shutdownCh,
			bgCtx:      bgCtx,
			bgCancel:   bgCancel,
		}, nil
	}

	matcher, err := NewMatcher(cfg)
	if err != nil {
		bgCancel()
		return nil, fmt.Errorf("failed to create matcher: %w", err)
	}

	var cache *Cache
	var db *sql.DB
	if dbProvider != nil {
		db = dbProvider.DB()
		cache = NewCache(db)
	}

	status := NewAIStatus()
	status.UpdateModelAvailability(true, cfg.Model)

	i := &Integrator{
		enabled:    true,
		config:     cfg,
		matcher:    matcher,
		cache:      cache,
		circuit:    circuit,
		status:     status,
		db:         db,
		shutdownCh: shutdownCh,
		bgCtx:      bgCtx,
		bgCancel:   bgCancel,
	}

	if cfg.Keepalive.Enabled {
		keepaliveCfg := KeepaliveConfig{
			Enabled:        true,
			Interval:       time.Duration(cfg.Keepalive.IntervalSeconds) * time.Second,
			FilenamePrompt: "test.keepalive",
		}
		i.keepalive = NewKeepalive(keepaliveCfg, matcher, status)
		i.keepalive.Start(bgCtx)
	}

	i.bgQueue = NewBackgroundQueue(100, 2, cfg.RetryDelay)
	i.bgWg.Add(1)
	go i.backgroundWorker()

	return i, nil
}

func (i *Integrator) EnhanceTitle(regexTitle, filename, mediaType string) (string, ParseSource, error) {
	if !i.enabled {
		return regexTitle, SourceRegex, nil
	}

	i.shutdownMu.RLock()
	if i.shutdown {
		i.shutdownMu.RUnlock()
		return regexTitle, SourceRegex, nil
	}
	i.shutdownMu.RUnlock()

	normalized := NormalizeInput(filename)

	if i.cache != nil {
		cached, err := i.cache.Get(normalized, mediaType, i.config.Model)
		if err == nil && cached != nil {
			i.status.RecordRequest(true, 0)
			return cached.Title, SourceCache, nil
		}
	}

	if !i.circuit.Allow() {
		i.status.UpdateCircuitStatus(i.circuit.State(), i.circuit.FailureCount(), nil)
		return regexTitle, SourceRegex, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(i.config.TimeoutSeconds)*time.Second)
	defer cancel()

	start := time.Now()
	result, err := i.tryAIWithRecovery(ctx, filename)
	latency := time.Since(start)

	if err != nil {
		i.circuit.RecordFailure(err.Error())
		i.status.RecordRequest(false, latency)
		i.status.UpdateCircuitStatus(i.circuit.State(), i.circuit.FailureCount(), nil)
		return regexTitle, SourceRegex, nil
	}

	i.circuit.RecordSuccess()
	i.status.RecordRequest(true, latency)
	i.status.UpdateCircuitStatus(i.circuit.State(), i.circuit.FailureCount(), nil)

	if result.Confidence < i.config.ConfidenceThreshold {
		return regexTitle, SourceRegex, nil
	}

	if i.cache != nil {
		_ = i.cache.Put(normalized, mediaType, i.config.Model, result, latency)
	}

	return result.Title, SourceAI, nil
}

func (i *Integrator) tryAIWithRecovery(ctx context.Context, filename string) (*Result, error) {
	result, err := i.matcher.ParseWithRetry(ctx, filename)
	if err == nil {
		return result, nil
	}

	if isConnectionError(err) {
		return nil, err
	}

	errStr := err.Error()
	if strings.Contains(errStr, "response:") {
		parts := strings.SplitN(errStr, "response:", 2)
		if len(parts) == 2 {
			if partial, ok := ExtractPartialResult(strings.TrimSpace(parts[1])); ok {
				return partial, nil
			}
		}
	}

	return nil, err
}

func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "no such host") ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "context deadline exceeded") ||
		strings.Contains(errStr, "network is unreachable")
}

func (i *Integrator) QueueForEnhancement(requestID, filename, regexTitle, mediaType string) bool {
	if !i.enabled || i.bgQueue == nil {
		return false
	}

	i.shutdownMu.RLock()
	if i.shutdown {
		i.shutdownMu.RUnlock()
		return false
	}
	i.shutdownMu.RUnlock()

	req := &EnhancementRequest{
		ID:        requestID,
		Filename:  filename,
		UserTitle: regexTitle,
		UserType:  mediaType,
		Timestamp: time.Now(),
	}

	err := i.bgQueue.Enqueue(req)
	if err != nil {
		return false
	}

	stats := i.bgQueue.QueueStats()
	i.status.UpdateQueueStats(stats.Pending, stats.Processing, stats.Completed, stats.Failed)

	return true
}

func (i *Integrator) backgroundWorker() {
	defer i.bgWg.Done()

	for {
		select {
		case <-i.bgCtx.Done():
			return
		case <-i.shutdownCh:
			return
		default:
			item := i.bgQueue.Dequeue()
			if item == nil {
				select {
				case <-i.bgCtx.Done():
					return
				case <-i.shutdownCh:
					return
				case <-time.After(100 * time.Millisecond):
					continue
				}
			}

			i.processBackgroundRequest(item)
		}
	}
}

func (i *Integrator) processBackgroundRequest(item *QueueItem) {
	if !i.circuit.Allow() {
		i.bgQueue.Fail(item, fmt.Errorf("circuit breaker open"))
		return
	}

	ctx, cancel := context.WithTimeout(i.bgCtx, time.Duration(i.config.TimeoutSeconds)*time.Second)
	defer cancel()

	start := time.Now()
	result, err := i.tryAIWithRecovery(ctx, item.Request.Filename)
	latency := time.Since(start)

	if err != nil {
		i.circuit.RecordFailure(err.Error())
		i.status.RecordRequest(false, latency)
		i.bgQueue.Fail(item, err)
		i.updateImprovementStatus(item.Request.ID, "failed", err.Error())
		return
	}

	i.circuit.RecordSuccess()
	i.status.RecordRequest(true, latency)

	if i.cache != nil {
		normalized := NormalizeInput(item.Request.Filename)
		_ = i.cache.Put(normalized, item.Request.UserType, i.config.Model, result, latency)
	}

	i.saveImprovementResult(item, result)
	i.bgQueue.Complete(item)

	stats := i.bgQueue.QueueStats()
	i.status.UpdateQueueStats(stats.Pending, stats.Processing, stats.Completed, stats.Failed)
}

func (i *Integrator) updateImprovementStatus(requestID, status, errorMsg string) {
	if i.db == nil {
		return
	}

	query := `UPDATE ai_improvements SET status = ?, updated_at = ?`
	args := []interface{}{status, time.Now()}

	if errorMsg != "" {
		query += `, error_message = ?, attempts = attempts + 1`
		args = append(args, errorMsg)
	}

	if status == "completed" {
		query += `, completed_at = ?`
		args = append(args, time.Now())
	}

	query += ` WHERE request_id = ?`
	args = append(args, requestID)

	_, _ = i.db.Exec(query, args...)
}

func (i *Integrator) saveImprovementResult(item *QueueItem, result *Result) {
	if i.db == nil {
		return
	}

	now := time.Now()
	_, _ = i.db.Exec(`
		INSERT INTO ai_improvements (
			request_id, filename, user_title, user_type, user_year,
			ai_title, ai_type, ai_year, ai_confidence,
			status, attempts, model, created_at, updated_at, completed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(request_id) DO UPDATE SET
			ai_title = excluded.ai_title,
			ai_type = excluded.ai_type,
			ai_year = excluded.ai_year,
			ai_confidence = excluded.ai_confidence,
			status = excluded.status,
			model = excluded.model,
			updated_at = excluded.updated_at,
			completed_at = excluded.completed_at`,
		item.Request.ID,
		item.Request.Filename,
		item.Request.UserTitle,
		item.Request.UserType,
		item.Request.UserYear,
		result.Title,
		result.Type,
		result.Year.Int(),
		result.Confidence,
		"completed",
		item.Attempts,
		i.config.Model,
		now, now, now,
	)
}

func (i *Integrator) Status() AIStatusSnapshot {
	if !i.enabled {
		return AIStatusSnapshot{
			CircuitState:   CircuitClosed,
			ModelAvailable: false,
			ModelName:      "disabled",
		}
	}

	i.status.UpdateCircuitStatus(i.circuit.State(), i.circuit.FailureCount(), nil)

	if i.bgQueue != nil {
		stats := i.bgQueue.QueueStats()
		i.status.UpdateQueueStats(stats.Pending, stats.Processing, stats.Completed, stats.Failed)
		i.status.UpdateQueueConfig(!i.bgQueue.IsStopped(), i.bgQueue.workers, i.bgQueue.queueSize)
	}

	return i.status.GetStatus()
}

func (i *Integrator) IsEnabled() bool {
	return i.enabled
}

func (i *Integrator) IsAvailable() bool {
	if !i.enabled {
		return false
	}
	return i.circuit.Allow()
}

func (i *Integrator) Close() error {
	i.shutdownMu.Lock()
	if i.shutdown {
		i.shutdownMu.Unlock()
		return nil
	}
	i.shutdown = true
	i.shutdownMu.Unlock()

	close(i.shutdownCh)
	i.bgCancel()

	if i.keepalive != nil {
		i.keepalive.Stop()
	}

	if i.bgQueue != nil {
		i.bgQueue.Stop()
	}

	i.bgWg.Wait()

	return nil
}
