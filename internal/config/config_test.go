package config

import (
	"os"
	"path/filepath"
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

func TestConfigToTOMLIncludesAllAISettings(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AI.Enabled = true
	cfg.AI.Model = "minimax-m2.5:cloud"
	cfg.AI.FallbackModel = "qwen3:8b"
	cfg.AI.CloudModel = "nemotron-3-nano:30b-cloud"
	cfg.AI.AutoResolveRisky = true
	cfg.AI.MaxRetries = 7
	cfg.AI.HourlyLimit = 11
	cfg.AI.DailyLimit = 22

	toml := cfg.ToTOML()
	for _, want := range []string{
		`model = "minimax-m2.5:cloud"`,
		`fallback_model = "qwen3:8b"`,
		`cloud_model = "nemotron-3-nano:30b-cloud"`,
		`auto_resolve_risky = true`,
		`max_retries = 7`,
		`hourly_limit = 11`,
		`daily_limit = 22`,
	} {
		if !strings.Contains(toml, want) {
			t.Fatalf("expected TOML to include %s:\n%s", want, toml)
		}
	}
}

func TestConfigToTOMLWritesPasswordHashNotPlainPassword(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Password = "secret"
	cfg.PasswordHash = "$2a$10$example"

	toml := cfg.ToTOML()
	if strings.Contains(toml, `password = "secret"`) {
		t.Fatalf("TOML should not write plaintext password:\n%s", toml)
	}
	if !strings.Contains(toml, `password_hash = "$2a$10$example"`) {
		t.Fatalf("TOML should write password_hash:\n%s", toml)
	}
}

func TestConfigSaveHashesPlaintextPassword(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SUDO_USER", "")
	cfg := DefaultConfig()
	cfg.Password = "secret"

	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(os.Getenv("HOME"), ".config", "plex2jellyfin", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	toml := string(data)
	if strings.Contains(toml, `password = "secret"`) {
		t.Fatalf("saved config contains plaintext password:\n%s", toml)
	}
	if !strings.Contains(toml, `password_hash = "$2`) {
		t.Fatalf("saved config does not contain bcrypt password_hash:\n%s", toml)
	}
	if cfg.Password != "" || cfg.PasswordHash == "" {
		t.Fatalf("Save should migrate in-memory password to hash, got password=%q hash=%q", cfg.Password, cfg.PasswordHash)
	}
}

func TestGetReportsPathUsesConfigDir(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SUDO_USER", "")

	got := GetReportsPath()
	if !strings.HasSuffix(got, filepath.Join(".config", "plex2jellyfin", "reports")) {
		t.Fatalf("GetReportsPath = %q", got)
	}
}

func TestLoadReturnsErrorForUnreadableExistingConfig(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read files regardless of owner mode")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SUDO_USER", "")

	configDir := filepath.Join(home, ".config", "plex2jellyfin")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "config.toml")
	if err := os.WriteFile(configPath, []byte("[libraries]\ntv = [\"/tv\"]\n"), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(configPath, 0o600) })

	_, err := Load()
	if err == nil {
		t.Fatal("expected unreadable config to return an error")
	}
	if !strings.Contains(err.Error(), "cannot read config file") {
		t.Fatalf("expected actionable config read error, got %v", err)
	}
}

func TestLoadTightensConfigFilePermissions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SUDO_USER", "")

	configDir := filepath.Join(home, ".config", "plex2jellyfin")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "config.toml")
	if err := os.WriteFile(configPath, []byte("[libraries]\ntv = [\"/tv\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("config mode = %o, want 0600", got)
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
