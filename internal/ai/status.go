package ai

import (
	"sync"
	"time"
)

// AIStatus provides observability metrics for the AI system
type AIStatus struct {
	mu sync.RWMutex

	// Circuit breaker status
	CircuitState    CircuitState
	FailureCount    int
	LastFailureTime *time.Time
	OpenTime        *time.Time

	// Performance metrics
	TotalRequests      int64
	SuccessfulRequests int64
	FailedRequests     int64
	AverageLatency     time.Duration
	LastLatency        time.Duration

	// Model status
	ModelAvailable bool
	ModelName      string
	LastPingTime   time.Time

	// Queue metrics
	PendingItems    int
	ProcessingItems int
	CompletedItems  int
	FailedItems     int

	// Background queue status (if configured)
	QueueRunning  bool
	QueueWorkers  int
	QueueCapacity int
}

// NewAIStatus creates a new AI status tracker
func NewAIStatus() *AIStatus {
	return &AIStatus{
		CircuitState:   CircuitClosed,
		ModelAvailable: true,
		LastPingTime:   time.Now(),
	}
}

// RecordRequest records a request attempt
func (s *AIStatus) RecordRequest(success bool, latency time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.TotalRequests++

	if success {
		s.SuccessfulRequests++
	} else {
		s.FailedRequests++
	}

	s.LastLatency = latency
	if s.AverageLatency == 0 {
		s.AverageLatency = latency
	} else {
		s.AverageLatency = (s.AverageLatency*9 + latency) / 10
	}
}

// UpdateCircuitStatus updates circuit breaker state
func (s *AIStatus) UpdateCircuitStatus(state CircuitState, failureCount int, lastFailure *time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.CircuitState = state
	s.FailureCount = failureCount
	s.LastFailureTime = lastFailure

	if state == CircuitOpen && s.OpenTime == nil {
		now := time.Now()
		s.OpenTime = &now
	} else if state == CircuitClosed {
		s.OpenTime = nil
	}
}

// UpdateModelAvailability updates model availability status
func (s *AIStatus) UpdateModelAvailability(available bool, model string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ModelAvailable = available
	if model != "" {
		s.ModelName = model
	}
	s.LastPingTime = time.Now()
}

// UpdateQueueStats updates background queue statistics
func (s *AIStatus) UpdateQueueStats(pending, processing, completed, failed int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.PendingItems = pending
	s.ProcessingItems = processing
	s.CompletedItems = completed
	s.FailedItems = failed
}

// UpdateQueueConfig updates background queue configuration
func (s *AIStatus) UpdateQueueConfig(running bool, workers, capacity int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.QueueRunning = running
	s.QueueWorkers = workers
	s.QueueCapacity = capacity
}

// GetStatus returns a snapshot of current AI status
func (s *AIStatus) GetStatus() AIStatusSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return AIStatusSnapshot{
		CircuitState:       s.CircuitState,
		FailureCount:       s.FailureCount,
		LastFailureTime:    s.LastFailureTime,
		OpenTime:           s.OpenTime,
		TotalRequests:      s.TotalRequests,
		SuccessfulRequests: s.SuccessfulRequests,
		FailedRequests:     s.FailedRequests,
		AverageLatency:     s.AverageLatency,
		LastLatency:        s.LastLatency,
		ModelAvailable:     s.ModelAvailable,
		ModelName:          s.ModelName,
		LastPingTime:       s.LastPingTime,
		PendingItems:       s.PendingItems,
		ProcessingItems:    s.ProcessingItems,
		CompletedItems:     s.CompletedItems,
		FailedItems:        s.FailedItems,
		QueueRunning:       s.QueueRunning,
		QueueWorkers:       s.QueueWorkers,
		QueueCapacity:      s.QueueCapacity,
	}
}

// AIStatusSnapshot represents an immutable snapshot of AI status
type AIStatusSnapshot struct {
	CircuitState       CircuitState
	FailureCount       int
	LastFailureTime    *time.Time
	OpenTime           *time.Time
	TotalRequests      int64
	SuccessfulRequests int64
	FailedRequests     int64
	AverageLatency     time.Duration
	LastLatency        time.Duration
	ModelAvailable     bool
	ModelName          string
	LastPingTime       time.Time
	PendingItems       int
	ProcessingItems    int
	CompletedItems     int
	FailedItems        int
	QueueRunning       bool
	QueueWorkers       int
	QueueCapacity      int
}

// SuccessRate returns the success rate as a percentage (0-100)
func (snap *AIStatusSnapshot) SuccessRate() float64 {
	if snap.TotalRequests == 0 {
		return 100.0
	}
	return float64(snap.SuccessfulRequests) / float64(snap.TotalRequests) * 100.0
}

// IsHealthy returns true if the AI system is healthy
func (snap *AIStatusSnapshot) IsHealthy() bool {
	return snap.CircuitState == CircuitClosed &&
		snap.ModelAvailable &&
		snap.SuccessRate() >= 50.0
}
