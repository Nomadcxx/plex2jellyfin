package config

import (
	"strings"
	"testing"
)

func TestPermissionsResolveNumeric(t *testing.T) {
	p := &PermissionsConfig{User: "0", Group: "0", FileMode: "0644", DirMode: "0755"}
	if uid, err := p.ResolveUID(); err != nil || uid != 0 {
		t.Fatalf("unexpected uid: %d %v", uid, err)
	}
	if gid, err := p.ResolveGID(); err != nil || gid != 0 {
		t.Fatalf("unexpected gid: %d %v", gid, err)
	}
	if fm, err := p.ParseFileMode(); err != nil || fm == 0 {
		t.Fatalf("unexpected file mode: %v %v", fm, err)
	}
	if dm, err := p.ParseDirMode(); err != nil || dm == 0 {
		t.Fatalf("unexpected dir mode: %v %v", dm, err)
	}
}

func TestPermissionsParseShortMode(t *testing.T) {
	p := &PermissionsConfig{FileMode: "644", DirMode: "755"}
	if fm, err := p.ParseFileMode(); err != nil || fm.String() == "" {
		t.Fatalf("unexpected file mode: %v %v", fm, err)
	}
	if dm, err := p.ParseDirMode(); err != nil || dm.String() == "" {
		t.Fatalf("unexpected dir mode: %v %v", dm, err)
	}
}

func TestAIConfig_CircuitBreakerDefaults(t *testing.T) {
	cfg := DefaultAIConfig()

	if cfg.CircuitBreaker.FailureThreshold != 5 {
		t.Errorf("expected failure threshold 5, got %d", cfg.CircuitBreaker.FailureThreshold)
	}
	if cfg.CircuitBreaker.FailureWindowSeconds != 120 {
		t.Errorf("expected failure window 120s, got %d", cfg.CircuitBreaker.FailureWindowSeconds)
	}
	if cfg.CircuitBreaker.CooldownSeconds != 30 {
		t.Errorf("expected cooldown 30s, got %d", cfg.CircuitBreaker.CooldownSeconds)
	}
}

func TestDefaultConfig_JellyfinDefaults(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Jellyfin.Enabled {
		t.Error("expected jellyfin disabled by default")
	}
	if cfg.Jellyfin.NotifyOnImport != true {
		t.Error("expected jellyfin notify_on_import default true")
	}
	if cfg.Jellyfin.PlaybackSafety != true {
		t.Error("expected jellyfin playback_safety default true")
	}
	if cfg.Jellyfin.VerifyAfterRefresh {
		t.Error("expected jellyfin verify_after_refresh default false")
	}
	if cfg.Jellyfin.WebhookSecret != "" {
		t.Errorf("expected jellyfin webhook_secret default empty, got %q", cfg.Jellyfin.WebhookSecret)
	}
}

func TestConfigToTOMLIncludesJellyfinSection(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Jellyfin.Enabled = true
	cfg.Jellyfin.URL = "http://localhost:8096"
	cfg.Jellyfin.APIKey = "abc123"
	cfg.Jellyfin.NotifyOnImport = true
	cfg.Jellyfin.WebhookSecret = "secret-token"

	toml := cfg.ToTOML()
	if !strings.Contains(toml, "[jellyfin]") {
		t.Fatal("expected [jellyfin] section in TOML output")
	}
	if !strings.Contains(toml, "playback_safety = true") {
		t.Fatal("expected playback_safety key in TOML output")
	}
	if !strings.Contains(toml, "webhook_secret = \"secret-token\"") {
		t.Fatal("expected webhook_secret key in TOML output")
	}
}
