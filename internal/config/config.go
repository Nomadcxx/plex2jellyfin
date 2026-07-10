package config

import (
	"fmt"
	"log"
	"os"
	"os/user"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/paths"
	"github.com/spf13/viper"
	"golang.org/x/crypto/bcrypt"
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
	Watch            WatchConfig            `mapstructure:"watch"`
	Libraries        LibrariesConfig        `mapstructure:"libraries"`
	Daemon           DaemonConfig           `mapstructure:"daemon"`
	Options          OptionsConfig          `mapstructure:"options"`
	Sonarr           SonarrConfig           `mapstructure:"sonarr"`
	Radarr           RadarrConfig           `mapstructure:"radarr"`
	Jellyfin         JellyfinConfig         `mapstructure:"jellyfin"`
	TMDB             TMDBConfig             `mapstructure:"tmdb"`
	Logging          LoggingConfig          `mapstructure:"logging"`
	Permissions      PermissionsConfig      `mapstructure:"permissions"`
	AI               AIConfig               `mapstructure:"ai"`
	API              APIConfig              `mapstructure:"api"`
	MetadataRecovery MetadataRecoveryConfig `mapstructure:"metadata_recovery" toml:"metadata_recovery"`
	Password         string                 `mapstructure:"password" secret:"true"`
	PasswordHash     string                 `mapstructure:"password_hash" secret:"true"`
	SecureCookies    bool                   `mapstructure:"secure_cookies"`
}

// APIConfig holds HTTP-server-only concerns (CORS, etc.). CORS origins
// default to the Next.js dev server; same-origin production deployments
// don't need CORS, but external dev or reverse-proxied setups might.
type APIConfig struct {
	AllowedOrigins []string `mapstructure:"allowed_origins"`
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
	// MaxAgeDays is the maximum age of a rotated log file. Files older than
	// this (by mtime) are deleted at rotation time. Zero disables age-based
	// pruning — only MaxBackups gates retention.
	MaxAgeDays int `mapstructure:"max_age_days"`
	// Compress gzips rotated backups (plex2jellyfin.1.log → plex2jellyfin.1.log.gz).
	// Cuts disk footprint ~5-10×; decompressed on demand with zcat/gunzip.
	// Default false for backwards compatibility with existing deployments.
	Compress bool `mapstructure:"compress"`
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

type MetadataRecoveryConfig struct {
	PassiveEnabled         bool `mapstructure:"passive_enabled" toml:"passive_enabled"`
	RepairEnabled          bool `mapstructure:"repair_enabled" toml:"repair_enabled"`
	PassiveIntervalMinutes int  `mapstructure:"passive_interval_minutes" toml:"passive_interval_minutes"`
	PassiveBatchSize       int  `mapstructure:"passive_batch_size" toml:"passive_batch_size"`
	RepairBatchSize        int  `mapstructure:"repair_batch_size" toml:"repair_batch_size"`
	RepairCooldownHours    int  `mapstructure:"repair_cooldown_hours" toml:"repair_cooldown_hours"`
	NeedsReviewAfter       int  `mapstructure:"needs_review_after" toml:"needs_review_after"`
}

// AIConfig contains AI title matching configuration
type AIConfig struct {
	Enabled                    bool                 `mapstructure:"enabled"`
	OllamaEndpoint             string               `mapstructure:"ollama_endpoint"`
	Model                      string               `mapstructure:"model"`
	FallbackModel              string               `mapstructure:"fallback_model"`
	ConfidenceThreshold        float64              `mapstructure:"confidence_threshold"`
	AutoTriggerThreshold       float64              `mapstructure:"auto_trigger_threshold"`
	TimeoutSeconds             int                  `mapstructure:"timeout_seconds"`
	CacheEnabled               bool                 `mapstructure:"cache_enabled"`
	CloudModel                 string               `mapstructure:"cloud_model"`
	AutoResolveRisky           bool                 `mapstructure:"auto_resolve_risky"`
	CircuitBreaker             CircuitBreakerConfig `mapstructure:"circuit_breaker"`
	Keepalive                  KeepaliveConfig      `mapstructure:"keepalive"`
	RetryDelay                 time.Duration        `mapstructure:"retry_delay"`
	MaxRetries                 int                  `mapstructure:"max_retries"`
	HourlyLimit                int                  `mapstructure:"hourly_limit"`
	DailyLimit                 int                  `mapstructure:"daily_limit"`
	EnhancementIntervalSeconds int                  `mapstructure:"enhancement_interval_seconds"`
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
	// TransferConcurrencyPerVolume caps simultaneous transfers landing on
	// the same destination mount point. Heavy parallel writes to one disk
	// trigger I/O contention that surfaces as false-positive disk-health
	// failures and rsync no-progress timeouts. <=0 disables; 0 (unset)
	// uses the daemon default (2).
	TransferConcurrencyPerVolume int `mapstructure:"transfer_concurrency_per_volume"`
}

// SonarrConfig contains Sonarr integration settings
type SonarrConfig struct {
	Enabled        bool   `mapstructure:"enabled"`
	URL            string `mapstructure:"url"`
	APIKey         string `mapstructure:"api_key" secret:"true"`
	NotifyOnImport bool   `mapstructure:"notify_on_import"`
}

type RadarrConfig struct {
	Enabled        bool   `mapstructure:"enabled"`
	URL            string `mapstructure:"url"`
	APIKey         string `mapstructure:"api_key" secret:"true"`
	NotifyOnImport bool   `mapstructure:"notify_on_import"`
}

// TMDBConfig holds optional credentials for TMDB direct lookups. Used
// by the housekeeping verifier as a fallback when Jellyfin RemoteSearch
// is unavailable. Free key: themoviedb.org/settings/api.
type TMDBConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	APIKey  string `mapstructure:"api_key" secret:"true"`
}

// JellyfinConfig contains Jellyfin integration settings.
type JellyfinConfig struct {
	Enabled            bool   `mapstructure:"enabled"`
	URL                string `mapstructure:"url"`
	APIKey             string `mapstructure:"api_key" secret:"true"`
	NotifyOnImport     bool   `mapstructure:"notify_on_import"`
	PlaybackSafety     bool   `mapstructure:"playback_safety"`
	VerifyAfterRefresh bool   `mapstructure:"verify_after_refresh"`
	WebhookSecret      string `mapstructure:"webhook_secret" secret:"true"`
	// Plugin settings
	PluginEnabled         bool   `mapstructure:"plugin_enabled"`
	PluginSharedSecret    string `mapstructure:"plugin_shared_secret" secret:"true"`
	PluginAutoScan        bool   `mapstructure:"plugin_auto_scan"`
	PluginVerifyOnStartup bool   `mapstructure:"plugin_verify_on_startup"`
	PluginVerifyInterval  int    `mapstructure:"plugin_verify_interval"`
	// PathMappings translates between Jellyfin's view of a media file
	// (as seen inside its container/bind mount) and the daemon's view
	// (host filesystem). Without these, the sweeper and webhook handler
	// cannot correlate Jellyfin items to parse_decisions rows when
	// Jellyfin runs in a container with different mount roots.
	PathMappings []JellyfinPathMapping `mapstructure:"path_mappings"`
}

// JellyfinPathMapping is a single prefix translation pair. See
// JellyfinConfig.PathMappings for usage.
type JellyfinPathMapping struct {
	Jellyfin string `mapstructure:"jellyfin"`
	Daemon   string `mapstructure:"daemon"`
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
			Enabled:                    false,
			OllamaEndpoint:             "http://localhost:11434",
			Model:                      "qwen2.5vl:7b",
			ConfidenceThreshold:        0.8,
			AutoTriggerThreshold:       0.6,
			TimeoutSeconds:             30,
			CacheEnabled:               true,
			CloudModel:                 "nemotron-3-nano:30b-cloud",
			RetryDelay:                 500 * time.Millisecond,
			MaxRetries:                 3,
			HourlyLimit:                10,
			DailyLimit:                 50,
			EnhancementIntervalSeconds: 30,
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
			MaxAgeDays: 0,
			Compress:   false,
		},
		API: APIConfig{
			AllowedOrigins: []string{"http://localhost:3000"},
		},
		MetadataRecovery: MetadataRecoveryConfig{
			PassiveEnabled:         true,
			RepairEnabled:          false,
			PassiveIntervalMinutes: 60,
			PassiveBatchSize:       25,
			RepairBatchSize:        5,
			RepairCooldownHours:    6,
			NeedsReviewAfter:       4,
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
	if info, err := os.Stat(configPath); err == nil {
		if err := ensureConfigFilePrivate(configPath, info.Mode().Perm()); err != nil {
			return nil, err
		}
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("cannot read config file %s: %w", configPath, err)
		}
	} else if os.IsPermission(err) {
		return nil, fmt.Errorf("cannot stat config file %s: %w", configPath, err)
	}

	// Unmarshal config
	cfg := DefaultConfig()
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("unable to unmarshal config: %w", err)
	}

	// Warn on keys in the TOML that the schema doesn't know about. Helps
	// catch typos ("max_ages_days") that viper would otherwise silently
	// ignore, leaving users wondering why a setting has no effect.
	if unknown := findUnknownKeys(v, cfg); len(unknown) > 0 {
		fmt.Fprintf(os.Stderr, "config warning: %d unknown key(s) in %s: %v\n", len(unknown), configPath, unknown)
	}

	if cfg.Password != "" && cfg.PasswordHash == "" {
		hash, hashErr := HashPassword(cfg.Password)
		if hashErr == nil {
			cfg.PasswordHash = hash
			cfg.Password = ""
			if saveErr := cfg.Save(); saveErr != nil {
				fmt.Fprintf(os.Stderr, "config warning: failed to migrate plaintext password to password_hash: %v\n", saveErr)
			}
		} else {
			fmt.Fprintf(os.Stderr, "config warning: failed to hash plaintext password: %v\n", hashErr)
		}
	}

	return cfg, nil
}

func ensureConfigFilePrivate(path string, mode os.FileMode) error {
	if mode&0o077 == 0 {
		return nil
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("cannot secure config file %s: %w", path, err)
	}
	return nil
}

// findUnknownKeys returns config keys present in the loaded TOML but absent
// from the Config schema's mapstructure tags. Compares flattened dotted keys
// (e.g. "logging.max_size_mb") against the set of known paths reflected out
// of the struct.
func findUnknownKeys(v *viper.Viper, cfg *Config) []string {
	known := collectKnownKeys(reflect.TypeOf(*cfg), "")
	knownSet := make(map[string]struct{}, len(known))
	for _, k := range known {
		knownSet[strings.ToLower(k)] = struct{}{}
	}
	var unknown []string
	for _, k := range v.AllKeys() {
		if _, ok := knownSet[strings.ToLower(k)]; !ok {
			unknown = append(unknown, k)
		}
	}
	return unknown
}

// collectKnownKeys walks the Config struct, emitting a dotted key path for
// every leaf field (matching viper's AllKeys() format).
func collectKnownKeys(t reflect.Type, prefix string) []string {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		if prefix != "" {
			return []string{prefix}
		}
		return nil
	}
	var out []string
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("mapstructure")
		if tag == "" || tag == "-" {
			// No tag means viper uses the lowercase field name.
			tag = strings.ToLower(f.Name)
		}
		key := tag
		if prefix != "" {
			key = prefix + "." + tag
		}
		out = append(out, collectKnownKeys(f.Type, key)...)
	}
	// A parent struct with no exported fields still counts as a leaf.
	if len(out) == 0 && prefix != "" {
		return []string{prefix}
	}
	return out
}

// Save saves configuration to file
func (c *Config) Save() error {
	configFile, err := ConfigPath()
	if err != nil {
		return err
	}
	if err := c.NormalizeSecrets(); err != nil {
		return err
	}

	return AtomicWriteWithLock(configFile, []byte(c.ToTOML()), 0600)
}

func (c *Config) NormalizeSecrets() error {
	if c.Password != "" && c.PasswordHash == "" {
		hash, err := HashPassword(c.Password)
		if err != nil {
			return err
		}
		c.PasswordHash = hash
		c.Password = ""
	}
	return nil
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
	base := fmt.Sprintf(`# Plex2Jellyfin Configuration
# Generated by: plex2jellyfin config init

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
# Shared secret for validating incoming webhooks. Jellyfin webhook requests must
# send this value in the X-Plex2Jellyfin-Webhook-Secret header.
webhook_secret = "%s"

# Plex2Jellyfin Companion Plugin Settings (Phase 3)
# Enable companion plugin for webhooks and verification
plugin_enabled = %v
# Companion plugin shared secret. Use the same value as webhook_secret unless
# the plugin is configured for a separate endpoint that does not call Plex2Jellyfin webhooks.
plugin_shared_secret = "%s"
# Automatically trigger Jellyfin library scans after organizing
plugin_auto_scan = %v
# Run verification on daemon startup
plugin_verify_on_startup = %v
# Hours between automatic verifications (0 = disabled)
plugin_verify_interval = %d

# ============================================================================
# METADATA RECOVERY
# Passive recovery checks Jellyfin for metadata that arrived after import.
# Active repair is disabled by default because it asks Jellyfin to refresh items.
# ============================================================================
[metadata_recovery]
passive_enabled = %v
repair_enabled = %v
passive_interval_minutes = %d
passive_batch_size = %d
repair_batch_size = %d
repair_cooldown_hours = %d
needs_review_after = %d

# ============================================================================
# DAEMON SETTINGS
# For plex2jellyfin-daemon background service
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
fallback_model = "%s"
confidence_threshold = %.2f
auto_trigger_threshold = %.2f
timeout_seconds = %d
cache_enabled = %v
cloud_model = "%s"
auto_resolve_risky = %v
max_retries = %d
hourly_limit = %d
daily_limit = %d
enhancement_interval_seconds = %d

# ============================================================================
# LOGGING
# ============================================================================
[logging]
level = "%s"
file = "%s"
max_size_mb = %d
max_backups = %d
max_age_days = %d
compress = %v

# ============================================================================
# API / WEB SERVER
# CORS origins for the web UI. Same-origin production deployments don't
# need to change this; reverse proxies or external dev servers do.
# ============================================================================
[api]
allowed_origins = %s
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
		c.MetadataRecovery.PassiveEnabled,
		c.MetadataRecovery.RepairEnabled,
		c.MetadataRecovery.PassiveIntervalMinutes,
		c.MetadataRecovery.PassiveBatchSize,
		c.MetadataRecovery.RepairBatchSize,
		c.MetadataRecovery.RepairCooldownHours,
		c.MetadataRecovery.NeedsReviewAfter,
		c.Daemon.Enabled,
		c.Daemon.ScanFrequency,
		c.Daemon.HealthAddr,
		c.Options.DryRun,
		c.Options.VerifyChecksums,
		c.Options.DeleteSource,
		c.AI.Enabled,
		c.AI.OllamaEndpoint,
		c.AI.Model,
		c.AI.FallbackModel,
		c.AI.ConfidenceThreshold,
		c.AI.AutoTriggerThreshold,
		c.AI.TimeoutSeconds,
		c.AI.CacheEnabled,
		c.AI.CloudModel,
		c.AI.AutoResolveRisky,
		c.AI.MaxRetries,
		c.AI.HourlyLimit,
		c.AI.DailyLimit,
		c.AI.EnhancementIntervalSeconds,
		c.Logging.Level,
		c.Logging.File,
		c.Logging.MaxSizeMB,
		c.Logging.MaxBackups,
		c.Logging.MaxAgeDays,
		c.Logging.Compress,
		formatStringSlice(c.API.AllowedOrigins),
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

	// Top-level keys must be emitted before the first [table] header — a bare
	// key appended after one is parsed as a member of that table and dropped
	// as unknown on the next Load. The legacy plaintext password field is
	// still read for compatibility and migrated by Load, but new writes
	// should not persist plaintext credentials.
	if c.PasswordHash != "" || c.SecureCookies {
		top := "\n# Web UI authentication (managed via the web UI; delete password_hash to reset)\n"
		if c.PasswordHash != "" {
			top += fmt.Sprintf("password_hash = \"%s\"\n", c.PasswordHash)
		}
		if c.SecureCookies {
			top += "secure_cookies = true\n"
		}
		marker := "# Generated by: plex2jellyfin config init\n"
		base = strings.Replace(base, marker, marker+top, 1)
	}

	return base
}

func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func VerifyPassword(password, hash string) bool {
	if strings.TrimSpace(hash) == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
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
		log.Printf("[config] warning: could not determine database path, falling back to ./media.db: %v", err)
		return "./media.db"
	}
	return dbPath
}

// GetReportsPath returns the directory for Plex2Jellyfin postmortem reports.
func GetReportsPath() string {
	reportsPath, err := paths.ReportsDir()
	if err != nil {
		log.Printf("[config] warning: could not determine reports path, falling back to ./reports: %v", err)
		return "./reports"
	}
	return reportsPath
}

// DefaultAIConfig returns default AI configuration
func DefaultAIConfig() AIConfig {
	return AIConfig{
		Enabled:                    false,
		OllamaEndpoint:             "http://localhost:11434",
		Model:                      "qwen2.5vl:7b",
		ConfidenceThreshold:        0.8,
		AutoTriggerThreshold:       0.6,
		TimeoutSeconds:             5,
		CacheEnabled:               true,
		RetryDelay:                 100 * time.Millisecond,
		HourlyLimit:                10,
		DailyLimit:                 50,
		EnhancementIntervalSeconds: 30,
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
