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

func (r *AIRateLimiter) Stats() (hourlyUsed, dailyUsed int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.hourlyUsed, r.dailyUsed
}
