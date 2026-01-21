package config

import (
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
