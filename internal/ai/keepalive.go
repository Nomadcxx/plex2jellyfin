package ai

import (
	"context"
	"fmt"
	"time"
)

// KeepaliveConfig holds configuration for AI model keepalive
type KeepaliveConfig struct {
	Enabled        bool
	Interval       time.Duration
	FilenamePrompt string
}

// DefaultKeepaliveConfig returns default keepalive configuration
func DefaultKeepaliveConfig() KeepaliveConfig {
	return KeepaliveConfig{
		Enabled:        true,
		Interval:       5 * time.Minute,
		FilenamePrompt: "test.keepalive",
	}
}

// Keepalive maintains AI model availability through periodic pings
type Keepalive struct {
	config   KeepaliveConfig
	matcher  *Matcher
	status   *AIStatus
	stopChan chan struct{}
	ticker   *time.Ticker
}

// NewKeepalive creates a new keepalive instance
func NewKeepalive(cfg KeepaliveConfig, matcher *Matcher, status *AIStatus) *Keepalive {
	return &Keepalive{
		config:   cfg,
		matcher:  matcher,
		status:   status,
		stopChan: make(chan struct{}),
	}
}

// Start begins the keepalive loop
func (k *Keepalive) Start(ctx context.Context) {
	if !k.config.Enabled {
		return
	}

	k.ticker = time.NewTicker(k.config.Interval)

	go func() {
		defer k.ticker.Stop()

		for {
			select {
			case <-k.ticker.C:
				k.performKeepalive(ctx)

			case <-ctx.Done():
				return

			case <-k.stopChan:
				return
			}
		}
	}()
}

// Stop gracefully stops the keepalive loop
func (k *Keepalive) Stop() {
	close(k.stopChan)
	if k.ticker != nil {
		k.ticker.Stop()
	}
}

// performKeepalive executes a single keepalive ping
func (k *Keepalive) performKeepalive(ctx context.Context) {
	startTime := time.Now()
	available := k.matcher.IsAvailable(ctx)
	latency := time.Since(startTime)

	k.status.UpdateModelAvailability(available, k.matcher.config.Model)

	if !available {
		fmt.Printf("[Keepalive] Model unavailable: %s\n", k.matcher.config.Model)
		return
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := k.matcher.Parse(ctx, k.config.FilenamePrompt)
	if err != nil {
		k.status.RecordRequest(false, latency)
		fmt.Printf("[Keepalive] Failed to warm model: %v\n", err)
		return
	}

	k.status.RecordRequest(true, latency)
	fmt.Printf("[Keepalive] Model warmed: %s (latency: %v)\n", k.matcher.config.Model, latency)
}

// IsRunning returns true if keepalive is active
func (k *Keepalive) IsRunning() bool {
	return k.ticker != nil
}
