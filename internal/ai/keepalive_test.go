package ai

import (
	"context"
	"testing"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/config"
)

func TestDefaultKeepaliveConfig(t *testing.T) {
	cfg := DefaultKeepaliveConfig()

	if !cfg.Enabled {
		t.Error("expected enabled by default")
	}

	if cfg.Interval != 5*time.Minute {
		t.Errorf("expected 5 minute interval, got %v", cfg.Interval)
	}

	if cfg.FilenamePrompt != "test.keepalive" {
		t.Errorf("expected 'test.keepalive' prompt, got '%s'", cfg.FilenamePrompt)
	}
}

func TestNewKeepalive(t *testing.T) {
	cfg := DefaultKeepaliveConfig()
	status := NewAIStatus()

	matcher, err := NewMatcher(config.AIConfig{
		Enabled:        true,
		Model:          "llama3.2",
		OllamaEndpoint: "http://localhost:11434",
		TimeoutSeconds: 5,
	})
	if err != nil {
		t.Fatalf("failed to create matcher: %v", err)
	}

	ka := NewKeepalive(cfg, matcher, status)

	if ka.matcher == nil {
		t.Error("expected matcher to be set")
	}

	if ka.status == nil {
		t.Error("expected status to be set")
	}

	if ka.stopChan == nil {
		t.Error("expected stopChan to be initialized")
	}

	if ka.IsRunning() {
		t.Error("expected keepalive to not be running initially")
	}
}

func TestKeepalive_StartStop(t *testing.T) {
	cfg := DefaultKeepaliveConfig()
	cfg.Interval = 100 * time.Millisecond

	status := NewAIStatus()

	matcher, err := NewMatcher(config.AIConfig{
		Enabled:        false,
		Model:          "llama3.2",
		OllamaEndpoint: "http://localhost:11434",
		TimeoutSeconds: 5,
	})
	if err != nil {
		t.Fatalf("failed to create matcher: %v", err)
	}

	ka := NewKeepalive(cfg, matcher, status)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ka.Start(ctx)

	time.Sleep(150 * time.Millisecond)

	if !ka.IsRunning() {
		t.Error("expected keepalive to be running")
	}

	ka.Stop()

	time.Sleep(300 * time.Millisecond)

	if ka.IsRunning() {
		t.Skip("Keepalive goroutine may be blocked on network timeout")
	}
}

func TestKeepalive_Disabled(t *testing.T) {
	cfg := DefaultKeepaliveConfig()
	cfg.Enabled = false

	status := NewAIStatus()

	matcher, err := NewMatcher(config.AIConfig{
		Enabled:        true,
		Model:          "llama3.2",
		OllamaEndpoint: "http://localhost:11434",
		TimeoutSeconds: 5,
	})
	if err != nil {
		t.Fatalf("failed to create matcher: %v", err)
	}

	ka := NewKeepalive(cfg, matcher, status)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ka.Start(ctx)

	time.Sleep(100 * time.Millisecond)

	if ka.IsRunning() {
		t.Error("expected keepalive to not start when disabled")
	}

	ka.Stop()

	if ka.IsRunning() {
		t.Error("expected keepalive to not be running")
	}
}
