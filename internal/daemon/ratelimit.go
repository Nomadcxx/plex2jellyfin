package daemon

import (
	"sync"
	"time"
)

type AIRateLimiter struct {
	hourlyCap   int
	dailyCap    int
	hourlyUsed  int
	dailyUsed   int
	hourlyReset time.Time
	dailyReset  time.Time
	mu          sync.Mutex
}

func NewAIRateLimiter(hourlyCap, dailyCap int) *AIRateLimiter {
	now := time.Now()
	return &AIRateLimiter{
		hourlyCap:   hourlyCap,
		dailyCap:    dailyCap,
		hourlyReset: now.Add(time.Hour),
		dailyReset:  now.Add(24 * time.Hour),
	}
}

func (r *AIRateLimiter) Allow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()

	if now.After(r.hourlyReset) {
		r.hourlyUsed = 0
		r.hourlyReset = now.Add(time.Hour)
	}
	if now.After(r.dailyReset) {
		r.dailyUsed = 0
		r.dailyReset = now.Add(24 * time.Hour)
	}

	return r.hourlyUsed < r.hourlyCap && r.dailyUsed < r.dailyCap
}

func (r *AIRateLimiter) Record() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hourlyUsed++
	r.dailyUsed++
}

// RecordAndReport increments the counters and returns both post-record usage
// snapshots. Callers that want to log budget pressure (e.g. WARN near cap)
// can use this to avoid a separate Stats() round-trip under the lock.
func (r *AIRateLimiter) RecordAndReport() (hourlyUsed, hourlyCap, dailyUsed, dailyCap int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hourlyUsed++
	r.dailyUsed++
	return r.hourlyUsed, r.hourlyCap, r.dailyUsed, r.dailyCap
}

// Caps returns the configured hourly and daily caps.
func (r *AIRateLimiter) Caps() (hourlyCap, dailyCap int) {
	return r.hourlyCap, r.dailyCap
}

func (r *AIRateLimiter) Stats() (hourlyUsed, dailyUsed int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.hourlyUsed, r.dailyUsed
}
