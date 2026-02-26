package config

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/paths"
	"github.com/spf13/viper"
)

type PermissionsConfig struct {
	// User can be a username (e.g., "jellyfin") or numeric UID (e.g., "1000").
	User string `mapstructure:"user"`
	// Group can be a group name (e.g., "jellyfin") or numeric GID (e.g., "1000").
	Group string `mapstructure:"group"`
	// Modes are strings in octal (e.g., "0644" or "644"). Empty means preserve source.
	FileMode string `mapstructure:"file_mode"`
	DirMode  string `mapstructure:"dir_mode"`
}

type Config struct {
	Watch         WatchConfig       `mapstructure:"watch"`
	Libraries     LibrariesConfig   `mapstructure:"libraries"`
	Daemon        DaemonConfig      `mapstructure:"daemon"`
	Options       OptionsConfig     `mapstructure:"options"`
	Sonarr        SonarrConfig      `mapstructure:"sonarr"`
	Radarr        RadarrConfig      `mapstructure:"radarr"`
	Jellyfin      JellyfinConfig    `mapstructure:"jellyfin"`
	Logging       LoggingConfig     `mapstructure:"logging"`
	Permissions   PermissionsConfig `mapstructure:"permissions"`
	AI            AIConfig          `mapstructure:"ai"`
	Password      string            `mapstructure:"password"`
	SecureCookies bool              `mapstructure:"secure_cookies"`
}

// Helper methods for permissions resolution and parsing
func (p *PermissionsConfig) WantsOwnership() bool {
	return strings.TrimSpace(p.User) != "" || strings.TrimSpace(p.Group) != ""
}

func (p *PermissionsConfig) WantsMode() bool {
	return strings.TrimSpace(p.FileMode) != "" || strings.TrimSpace(p.DirMode) != ""
}

func (p *PermissionsConfig) ResolveUID() (int, error) {
	if p.User == "" {
		return -1, nil
	}
	// If numeric
	if uid, err := strconv.Atoi(p.User); err == nil {
		return uid, nil
	}
	usr, err := user.Lookup(p.User)
	if err != nil {
		return -1, err
	}
	uid, err := strconv.Atoi(usr.Uid)
	if err != nil {
		return -1, err
	}
	return uid, nil
}

func (p *PermissionsConfig) ResolveGID() (int, error) {
	if p.Group == "" {
		return -1, nil
	}
	if gid, err := strconv.Atoi(p.Group); err == nil {
		return gid, nil
	}
	grp, err := user.LookupGroup(p.Group)
	if err != nil {
		return -1, err
	}
	gid, err := strconv.Atoi(grp.Gid)
	if err != nil {
		return -1, err
	}
	return gid, nil
}

func (p *PermissionsConfig) ParseFileMode() (os.FileMode, error) {
	if strings.TrimSpace(p.FileMode) == "" {
		return 0, nil
	}
	m := strings.TrimSpace(p.FileMode)
	if len(m) == 3 { // allow "644"
		m = "0" + m
	}
	v, err := strconv.ParseUint(m, 8, 32)
	if err != nil {
		return 0, err
	}
	return os.FileMode(v), nil
}

func (p *PermissionsConfig) ParseDirMode() (os.FileMode, error) {
	if strings.TrimSpace(p.DirMode) == "" {
		return 0, nil
	}
	m := strings.TrimSpace(p.DirMode)
	if len(m) == 3 {
		m = "0" + m
	}
	v, err := strconv.ParseUint(m, 8, 32)
	if err != nil {
		return 0, err
	}
	return os.FileMode(v), nil
}

type LoggingConfig struct {
	Level      string `mapstructure:"level"`
	File       string `mapstructure:"file"`
	MaxSizeMB  int    `mapstructure:"max_size_mb"`
	MaxBackups int    `mapstructure:"max_backups"`
}

type CircuitBreakerConfig struct {
	FailureThreshold     int `mapstructure:"failure_threshold"`
	FailureWindowSeconds int `mapstructure:"failure_window_seconds"`
	CooldownSeconds      int `mapstructure:"cooldown_seconds"`
}

type KeepaliveConfig struct {
	Enabled            bool `mapstructure:"enabled"`
	IntervalSeconds    int  `mapstructure:"interval_seconds"`
	IdleTimeoutSeconds int  `mapstructure:"idle_timeout_seconds"`
}

// AIConfig contains AI title matching configuration
type AIConfig struct {
	Enabled              bool                 `mapstructure:"enabled"`
	OllamaEndpoint       string               `mapstructure:"ollama_endpoint"`
	Model                string               `mapstructure:"model"`
	FallbackModel        string               `mapstructure:"fallback_model"`
	ConfidenceThreshold  float64              `mapstructure:"confidence_threshold"`
	AutoTriggerThreshold float64              `mapstructure:"auto_trigger_threshold"`
	TimeoutSeconds       int                  `mapstructure:"timeout_seconds"`
	CacheEnabled         bool                 `mapstructure:"cache_enabled"`
	CloudModel           string               `mapstructure:"cloud_model"`
	AutoResolveRisky     bool                 `mapstructure:"auto_resolve_risky"`
	CircuitBreaker       CircuitBreakerConfig `mapstructure:"circuit_breaker"`
	Keepalive            KeepaliveConfig      `mapstructure:"keepalive"`
	RetryDelay           time.Duration        `mapstructure:"retry_delay"`
	MaxRetries           int                  `mapstructure:"max_retries"`
}

// WatchConfig contains directories to watch
type WatchConfig struct {
	Movies []string `mapstructure:"movies"`
	TV     []string `mapstructure:"tv"`
}

// LibrariesConfig contains destination library paths
type LibrariesConfig struct {
	Movies []string `mapstructure:"movies"`
	TV     []string `mapstructure:"tv"`
}

type DaemonConfig struct {
	Enabled       bool   `mapstructure:"enabled"`
	ScanFrequency string `mapstructure:"scan_frequency"`
	HealthAddr    string `mapstructure:"health_addr"`
}

// OptionsConfig contains general options
type OptionsConfig struct {
	DryRun          bool `mapstructure:"dry_run"`
	VerifyChecksums bool `mapstructure:"verify_checksums"`
	DeleteSource    bool `mapstructure:"delete_source"`
}

// SonarrConfig contains Sonarr integration settings
type SonarrConfig struct {
	Enabled        bool   `mapstructure:"enabled"`
	URL            string `mapstructure:"url"`
	APIKey         string `mapstructure:"api_key"`
	NotifyOnImport bool   `mapstructure:"notify_on_import"`
}

type RadarrConfig struct {
	Enabled        bool   `mapstructure:"enabled"`
	URL            string `mapstructure:"url"`
	APIKey         string `mapstructure:"api_key"`
	NotifyOnImport bool   `mapstructure:"notify_on_import"`
}

// JellyfinConfig contains Jellyfin integration settings.
type JellyfinConfig struct {
	Enabled            bool   `mapstructure:"enabled"`
	URL                string `mapstructure:"url"`
	APIKey             string `mapstructure:"api_key"`
	NotifyOnImport     bool   `mapstructure:"notify_on_import"`
	PlaybackSafety     bool   `mapstructure:"playback_safety"`
	VerifyAfterRefresh bool   `mapstructure:"verify_after_refresh"`
	WebhookSecret      string `mapstructure:"webhook_secret"`
	// Plugin settings
	PluginEnabled         bool   `mapstructure:"plugin_enabled"`
	PluginSharedSecret    string `mapstructure:"plugin_shared_secret"`
	PluginAutoScan        bool   `mapstructure:"plugin_auto_scan"`
	PluginVerifyOnStartup bool   `mapstructure:"plugin_verify_on_startup"`
	PluginVerifyInterval  int    `mapstructure:"plugin_verify_interval"`
}

// DefaultConfig returns default configuration
func DefaultConfig() *Config {
	return &Config{
		Watch: WatchConfig{
			Movies: []string{},
			TV:     []string{},
		},
		Libraries: LibrariesConfig{
			Movies: []string{},
			TV:     []string{},
		},
		Daemon: DaemonConfig{
			Enabled:       false,
			ScanFrequency: "5m",
			HealthAddr:    ":8686",
		},
		Options: OptionsConfig{
			DryRun:          false,
			VerifyChecksums: false,
			DeleteSource:    true,
		},
		Sonarr: SonarrConfig{
			Enabled:        false,
			URL:            "",
			APIKey:         "",
			NotifyOnImport: true,
		},
		Radarr: RadarrConfig{
			Enabled:        false,
			URL:            "",
			APIKey:         "",
			NotifyOnImport: true,
		},
		Jellyfin: JellyfinConfig{
			Enabled:               false,
			URL:                   "",
			APIKey:                "",
			NotifyOnImport:        true,
			PlaybackSafety:        true,
			VerifyAfterRefresh:    false,
			WebhookSecret:         "",
			PluginEnabled:         false,
			PluginSharedSecret:    "",
			PluginAutoScan:        true,
			PluginVerifyOnStartup: false,
			PluginVerifyInterval:  0,
		},
		AI: AIConfig{
			Enabled:              false,
			OllamaEndpoint:       "http://localhost:11434",
			Model:                "qwen2.5vl:7b",
			ConfidenceThreshold:  0.8,
			AutoTriggerThreshold: 0.6,
			TimeoutSeconds:       30,
			CacheEnabled:         true,
			CloudModel:           "nemotron-3-nano:30b-cloud",
			RetryDelay:           500 * time.Millisecond,
			MaxRetries:           3,
			CircuitBreaker: CircuitBreakerConfig{
				FailureThreshold:     5,
				FailureWindowSeconds: 120,
				CooldownSeconds:      30,
			},
			Keepalive: KeepaliveConfig{
				Enabled:            true,
				IntervalSeconds:    300,
				IdleTimeoutSeconds: 1800,
			},
		},
		Logging: LoggingConfig{
			Level:      "info",
			File:       "",
			MaxSizeMB:  10,
			MaxBackups: 5,
		},
	}
}

// Load loads configuration from file or returns defaults
func Load() (*Config, error) {
	v := viper.New()

	// Set config file location
	configPath, err := paths.ConfigPath()
	if err != nil {
		return nil, fmt.Errorf("unable to get config path: %w", err)
	}
	v.SetConfigFile(configPath)

	// Read config file if it exists
	if _, err := os.Stat(configPath); err == nil {
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("unable to read config file: %w", err)
		}
	}

	// Unmarshal config
	cfg := DefaultConfig()
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("unable to unmarshal config: %w", err)
	}

	return cfg, nil
}

// Save saves configuration to file
func (c *Config) Save() error {
	configFile, err := ConfigPath()
	if err != nil {
		return err
	}

	configDir := filepath.Dir(configFile)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("unable to create config dir: %w", err)
	}

	content := c.ToTOML()
	return os.WriteFile(configFile, []byte(content), 0644)
}

func ConfigPath() (string, error) {
	return paths.ConfigPath()
}

func ConfigExists() bool {
	path, err := ConfigPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

func (c *Config) ToTOML() string {
	base := fmt.Sprintf(`# JellyWatch Configuration
# Generated by: jellywatch config init

# ============================================================================
# WATCH DIRECTORIES
# Directories where new downloads arrive (from Sabnzbd, qBittorrent, etc.)
# ============================================================================
[watch]
# TV show downloads - daemon will watch these for new episodes
tv = %s

# Movie downloads - daemon will watch these for new movies  
movies = %s

# ============================================================================
# JELLYFIN LIBRARY DIRECTORIES
# Where organized media files should be moved to
# ============================================================================
[libraries]
# TV library paths - files organized as: Show Name (Year)/Season XX/Show S01E01.ext
tv = %s

# Movie library paths - files organized as: Movie Name (Year)/Movie Name (Year).ext
movies = %s

# ============================================================================
# SONARR INTEGRATION (TV Shows)
# Optional: Notify Sonarr after importing TV episodes
# Get API key from: Sonarr -> Settings -> General -> API Key
# ============================================================================
[sonarr]
enabled = %v
url = "%s"
api_key = "%s"
notify_on_import = %v

# ============================================================================
# RADARR INTEGRATION (Movies)
# Optional: Notify Radarr after importing movies
# Get API key from: Radarr -> Settings -> General -> API Key
# ============================================================================
[radarr]
enabled = %v
url = "%s"
api_key = "%s"
notify_on_import = %v

# ============================================================================
# JELLYFIN INTEGRATION
# Optional: Trigger library refresh and playback safety checks
# Get API key from: Jellyfin -> Dashboard -> Expert -> API Keys
# ============================================================================
[jellyfin]
# Enable Jellyfin integration for library refresh and playback safety
enabled = %v
# Jellyfin server URL (e.g., http://localhost:8096)
url = "%s"
# API key from Jellyfin Dashboard > Expert > API Keys
api_key = "%s"
# Trigger library refresh after organizing files
notify_on_import = %v
# Block file moves when media is being actively streamed
playback_safety = %v
# Query Jellyfin after refresh to verify correct identification (Phase 2)
verify_after_refresh = %v
# Shared secret for validating incoming webhooks (required when Jellyfin webhooks are enabled)
webhook_secret = "%s"

# JellyWatch Companion Plugin Settings (Phase 3)
# Enable companion plugin for webhooks and verification
plugin_enabled = %v
# Shared secret for webhook authentication
plugin_shared_secret = "%s"
# Automatically trigger Jellyfin library scans after organizing
plugin_auto_scan = %v
# Run verification on daemon startup
plugin_verify_on_startup = %v
# Hours between automatic verifications (0 = disabled)
plugin_verify_interval = %d

# ============================================================================
# DAEMON SETTINGS
# For jellywatchd background service
# ============================================================================
[daemon]
enabled = %v
scan_frequency = "%s"
health_addr = "%s"

# ============================================================================
# GENERAL OPTIONS
# ============================================================================
[options]
# Preview mode - don't actually move files
dry_run = %v

# Verify file integrity after transfer
verify_checksums = %v

# Delete source file after successful transfer (false = copy instead of move)
delete_source = %v

# ============================================================================
# AI TITLE MATCHING
# Optional: Use AI for improved title parsing (requires Ollama)
# ============================================================================
[ai]
enabled = %v
ollama_endpoint = "%s"
model = "%s"
confidence_threshold = %.2f
auto_trigger_threshold = %.2f
timeout_seconds = %d
cache_enabled = %v
cloud_model = "%s"

# ============================================================================
# LOGGING
# ============================================================================
[logging]
level = "%s"
file = "%s"
max_size_mb = %d
max_backups = %d
`,
		formatStringSlice(c.Watch.TV),
		formatStringSlice(c.Watch.Movies),
		formatStringSlice(c.Libraries.TV),
		formatStringSlice(c.Libraries.Movies),
		c.Sonarr.Enabled,
		c.Sonarr.URL,
		c.Sonarr.APIKey,
		c.Sonarr.NotifyOnImport,
		c.Radarr.Enabled,
		c.Radarr.URL,
		c.Radarr.APIKey,
		c.Radarr.NotifyOnImport,
		c.Jellyfin.Enabled,
		c.Jellyfin.URL,
		c.Jellyfin.APIKey,
		c.Jellyfin.NotifyOnImport,
		c.Jellyfin.PlaybackSafety,
		c.Jellyfin.VerifyAfterRefresh,
		c.Jellyfin.WebhookSecret,
		c.Jellyfin.PluginEnabled,
		c.Jellyfin.PluginSharedSecret,
		c.Jellyfin.PluginAutoScan,
		c.Jellyfin.PluginVerifyOnStartup,
		c.Jellyfin.PluginVerifyInterval,
		c.Daemon.Enabled,
		c.Daemon.ScanFrequency,
		c.Daemon.HealthAddr,
		c.Options.DryRun,
		c.Options.VerifyChecksums,
		c.Options.DeleteSource,
		c.AI.Enabled,
		c.AI.OllamaEndpoint,
		c.AI.Model,
		c.AI.ConfidenceThreshold,
		c.AI.AutoTriggerThreshold,
		c.AI.TimeoutSeconds,
		c.AI.CacheEnabled,
		c.AI.CloudModel,
		c.Logging.Level,
		c.Logging.File,
		c.Logging.MaxSizeMB,
		c.Logging.MaxBackups,
	)

	// Append permissions if configured
	if c.Permissions.WantsOwnership() || c.Permissions.WantsMode() {
		perm := "\n# ============================================================================\n# PERMISSIONS\n# Control ownership and permissions of transferred files\n# ============================================================================\n[permissions]\n"
		if c.Permissions.User != "" {
			perm += fmt.Sprintf("user = \"%s\"\n", c.Permissions.User)
		}
		if c.Permissions.Group != "" {
			perm += fmt.Sprintf("group = \"%s\"\n", c.Permissions.Group)
		}
		if c.Permissions.FileMode != "" {
			perm += fmt.Sprintf("file_mode = \"%s\"\n", c.Permissions.FileMode)
		}
		if c.Permissions.DirMode != "" {
			perm += fmt.Sprintf("dir_mode = \"%s\"\n", c.Permissions.DirMode)
		}
		base += perm
	}

	// Append password if configured
	if c.Password != "" {
		base += fmt.Sprintf("\n# ============================================================================\n# AUTHENTICATION\n# Optional password to protect the web UI\n# Leave empty to disable authentication\n# ============================================================================\npassword = \"%s\"\n", c.Password)
	}

	return base
}

func formatStringSlice(s []string) string {
	if len(s) == 0 {
		return "[]"
	}
	quoted := make([]string, len(s))
	for i, v := range s {
		quoted[i] = fmt.Sprintf("%q", v)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

// GetDatabasePath returns the path to the HOLDEN database file
func GetDatabasePath() string {
	dbPath, err := paths.DatabasePath()
	if err != nil {
		return "./media.db"
	}
	return dbPath
}

// DefaultAIConfig returns default AI configuration
func DefaultAIConfig() AIConfig {
	return AIConfig{
		Enabled:              false,
		OllamaEndpoint:       "http://localhost:11434",
		Model:                "qwen2.5vl:7b",
		ConfidenceThreshold:  0.8,
		AutoTriggerThreshold: 0.6,
		TimeoutSeconds:       5,
		CacheEnabled:         true,
		RetryDelay:           100 * time.Millisecond,
		CircuitBreaker: CircuitBreakerConfig{
			FailureThreshold:     5,
			FailureWindowSeconds: 120,
			CooldownSeconds:      30,
		},
		Keepalive: KeepaliveConfig{
			Enabled:            true,
			IntervalSeconds:    300,
			IdleTimeoutSeconds: 1800,
		},
	}
}
