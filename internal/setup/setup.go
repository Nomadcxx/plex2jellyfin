package setup

import (
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/config"
)

const CurrentVersion = 1

const (
	RuntimeNative    = "native"
	RuntimeContainer = "container"
)

type RuntimeInfo struct {
	Kind string `json:"kind"`
	UID  int    `json:"uid"`
	GID  int    `json:"gid"`
}

type PathsDraft struct {
	TV     []string `json:"tv"`
	Movies []string `json:"movies"`
}

type ServiceDraft struct {
	Enabled bool   `json:"enabled"`
	URL     string `json:"url"`
	APIKey  string `json:"api_key"`
}

type JellyfinDraft struct {
	Enabled      bool                         `json:"enabled"`
	URL          string                       `json:"url"`
	APIKey       string                       `json:"api_key"`
	PathMappings []config.JellyfinPathMapping `json:"path_mappings"`
}

type AIDraft struct {
	Enabled       bool   `json:"enabled"`
	Endpoint      string `json:"endpoint"`
	PrimaryModel  string `json:"primary_model"`
	FallbackModel string `json:"fallback_model"`
}

type RuntimeDraft struct {
	ScanFrequency   string                   `json:"scan_frequency"`
	DeleteSource    bool                     `json:"delete_source"`
	VerifyChecksums bool                     `json:"verify_checksums"`
	Permissions     config.PermissionsConfig `json:"permissions"`
}

type Draft struct {
	Watch     PathsDraft    `json:"watch"`
	Libraries PathsDraft    `json:"libraries"`
	Sonarr    ServiceDraft  `json:"sonarr"`
	Radarr    ServiceDraft  `json:"radarr"`
	Jellyfin  JellyfinDraft `json:"jellyfin"`
	AI        AIDraft       `json:"ai"`
	Runtime   RuntimeDraft  `json:"runtime"`
}

type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func NeedsSetup(cfg *config.Config) bool {
	if cfg == nil {
		return true
	}
	if cfg.Setup.Version > 0 {
		return !cfg.Setup.Completed
	}
	return !HasMediaPair(cfg)
}

func HasMediaPair(cfg *config.Config) bool {
	if cfg == nil {
		return false
	}
	tv := len(nonEmpty(cfg.Watch.TV)) > 0 && len(nonEmpty(cfg.Libraries.TV)) > 0
	movies := len(nonEmpty(cfg.Watch.Movies)) > 0 && len(nonEmpty(cfg.Libraries.Movies)) > 0
	return tv || movies
}

// AdoptLegacyCompletion stamps the explicit setup marker onto configs that
// predate it: version 0 but recognizably configured (a complete media pair).
// This converts the HasMediaPair heuristic into durable state once, so later
// hand-edits to paths can never re-trigger a wizard for a configured install.
// Returns true when the config was mutated; the caller persists it.
func AdoptLegacyCompletion(cfg *config.Config) bool {
	if cfg == nil || cfg.Setup.Version > 0 {
		return false
	}
	if !HasMediaPair(cfg) {
		return false
	}
	cfg.Setup.Version = CurrentVersion
	cfg.Setup.Completed = true
	return true
}

func DraftFromConfig(cfg *config.Config) Draft {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	return Draft{
		Watch:     PathsDraft{TV: clone(cfg.Watch.TV), Movies: clone(cfg.Watch.Movies)},
		Libraries: PathsDraft{TV: clone(cfg.Libraries.TV), Movies: clone(cfg.Libraries.Movies)},
		Sonarr:    ServiceDraft{Enabled: cfg.Sonarr.Enabled, URL: cfg.Sonarr.URL, APIKey: mask(cfg.Sonarr.APIKey)},
		Radarr:    ServiceDraft{Enabled: cfg.Radarr.Enabled, URL: cfg.Radarr.URL, APIKey: mask(cfg.Radarr.APIKey)},
		Jellyfin: JellyfinDraft{
			Enabled: cfg.Jellyfin.Enabled, URL: cfg.Jellyfin.URL, APIKey: mask(cfg.Jellyfin.APIKey),
			PathMappings: append([]config.JellyfinPathMapping{}, cfg.Jellyfin.PathMappings...),
		},
		AI: AIDraft{
			Enabled: cfg.AI.Enabled, Endpoint: cfg.AI.OllamaEndpoint,
			PrimaryModel: cfg.AI.Model, FallbackModel: cfg.AI.FallbackModel,
		},
		Runtime: RuntimeDraft{
			ScanFrequency: cfg.Daemon.ScanFrequency, DeleteSource: cfg.Options.DeleteSource,
			VerifyChecksums: cfg.Options.VerifyChecksums, Permissions: cfg.Permissions,
		},
	}
}

func ValidateDraft(d Draft, runtime RuntimeInfo) []FieldError {
	var errs []FieldError
	tvWatch, tvLibrary := nonEmpty(d.Watch.TV), nonEmpty(d.Libraries.TV)
	movieWatch, movieLibrary := nonEmpty(d.Watch.Movies), nonEmpty(d.Libraries.Movies)
	if len(tvWatch)+len(tvLibrary)+len(movieWatch)+len(movieLibrary) == 0 {
		errs = append(errs, FieldError{Field: "media", Message: "configure TV, Movies, or both"})
	}
	validatePair := func(kind string, incoming, library []string) {
		if (len(incoming) == 0) != (len(library) == 0) {
			errs = append(errs, FieldError{Field: kind, Message: "incoming and library paths must both be configured"})
		}
	}
	validatePair("tv", tvWatch, tvLibrary)
	validatePair("movies", movieWatch, movieLibrary)

	if duration, err := time.ParseDuration(strings.TrimSpace(d.Runtime.ScanFrequency)); err != nil || duration <= 0 {
		errs = append(errs, FieldError{Field: "runtime.scan_frequency", Message: "must be a positive duration such as 5m"})
	}

	for name, svc := range map[string]ServiceDraft{"sonarr": d.Sonarr, "radarr": d.Radarr} {
		if !svc.Enabled {
			continue
		}
		if !validHTTPURL(svc.URL) {
			errs = append(errs, FieldError{Field: name + ".url", Message: "must be an http or https URL"})
		}
		if strings.TrimSpace(svc.APIKey) == "" {
			errs = append(errs, FieldError{Field: name + ".api_key", Message: "is required when enabled"})
		}
	}
	if d.Jellyfin.Enabled {
		if !validHTTPURL(d.Jellyfin.URL) {
			errs = append(errs, FieldError{Field: "jellyfin.url", Message: "must be an http or https URL"})
		}
		if strings.TrimSpace(d.Jellyfin.APIKey) == "" {
			errs = append(errs, FieldError{Field: "jellyfin.api_key", Message: "is required when enabled"})
		}
	}
	for i, mapping := range d.Jellyfin.PathMappings {
		if strings.TrimSpace(mapping.Jellyfin) == "" || strings.TrimSpace(mapping.Daemon) == "" {
			errs = append(errs, FieldError{Field: "jellyfin.path_mappings", Message: "mapping " + strconv.Itoa(i+1) + " requires both paths"})
		}
	}

	if d.AI.Enabled {
		if !validHTTPURL(d.AI.Endpoint) {
			errs = append(errs, FieldError{Field: "ai.endpoint", Message: "must be an http or https URL"})
		}
		if strings.TrimSpace(d.AI.PrimaryModel) == "" {
			errs = append(errs, FieldError{Field: "ai.primary_model", Message: "is required when enabled"})
		}
		if d.AI.FallbackModel != "" && d.AI.FallbackModel == d.AI.PrimaryModel {
			errs = append(errs, FieldError{Field: "ai.fallback_model", Message: "must differ from the primary model"})
		}
	}

	permissions := d.Runtime.Permissions
	if runtime.Kind == RuntimeContainer && (permissions.WantsOwnership() || permissions.WantsMode()) {
		errs = append(errs, FieldError{Field: "runtime.permissions", Message: "ownership and modes are controlled by the container PUID/PGID"})
	} else if runtime.Kind != RuntimeContainer {
		if _, err := permissions.ParseFileMode(); err != nil {
			errs = append(errs, FieldError{Field: "runtime.permissions.file_mode", Message: "must be an octal mode"})
		}
		if _, err := permissions.ParseDirMode(); err != nil {
			errs = append(errs, FieldError{Field: "runtime.permissions.dir_mode", Message: "must be an octal mode"})
		}
	}
	return errs
}

func ApplyDraft(current *config.Config, d Draft) *config.Config {
	if current == nil {
		current = config.DefaultConfig()
	}
	next := *current
	next.Watch.TV = nonEmpty(d.Watch.TV)
	next.Watch.Movies = nonEmpty(d.Watch.Movies)
	next.Libraries.TV = nonEmpty(d.Libraries.TV)
	next.Libraries.Movies = nonEmpty(d.Libraries.Movies)
	next.Daemon.Enabled = true
	next.Daemon.ScanFrequency = strings.TrimSpace(d.Runtime.ScanFrequency)
	next.Options.DeleteSource = d.Runtime.DeleteSource
	next.Options.VerifyChecksums = d.Runtime.VerifyChecksums
	next.Permissions = d.Runtime.Permissions

	next.Sonarr.Enabled = d.Sonarr.Enabled
	next.Sonarr.URL = strings.TrimRight(strings.TrimSpace(d.Sonarr.URL), "/")
	next.Sonarr.APIKey = preserveSecret(d.Sonarr.APIKey, current.Sonarr.APIKey)
	next.Radarr.Enabled = d.Radarr.Enabled
	next.Radarr.URL = strings.TrimRight(strings.TrimSpace(d.Radarr.URL), "/")
	next.Radarr.APIKey = preserveSecret(d.Radarr.APIKey, current.Radarr.APIKey)
	next.Jellyfin.Enabled = d.Jellyfin.Enabled
	next.Jellyfin.URL = strings.TrimRight(strings.TrimSpace(d.Jellyfin.URL), "/")
	next.Jellyfin.APIKey = preserveSecret(d.Jellyfin.APIKey, current.Jellyfin.APIKey)
	next.Jellyfin.PathMappings = append([]config.JellyfinPathMapping(nil), d.Jellyfin.PathMappings...)

	next.AI.Enabled = d.AI.Enabled
	next.AI.OllamaEndpoint = strings.TrimRight(strings.TrimSpace(d.AI.Endpoint), "/")
	next.AI.Model = strings.TrimSpace(d.AI.PrimaryModel)
	next.AI.FallbackModel = strings.TrimSpace(d.AI.FallbackModel)
	next.Setup.Version = CurrentVersion
	next.Setup.Completed = false
	return &next
}

func DetectRuntime() RuntimeInfo {
	kind := RuntimeNative
	if os.Getenv("container") != "" || os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		kind = RuntimeContainer
	} else if _, err := os.Stat("/.dockerenv"); err == nil {
		kind = RuntimeContainer
	}
	uid, gid := os.Getuid(), os.Getgid()
	if kind == RuntimeContainer {
		uid = envInt("PUID", uid)
		gid = envInt("PGID", gid)
	}
	return RuntimeInfo{Kind: kind, UID: uid, GID: gid}
}

func validHTTPURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func preserveSecret(supplied, current string) string {
	if strings.HasPrefix(supplied, "****") {
		return current
	}
	return strings.TrimSpace(supplied)
}

func mask(secret string) string {
	if secret == "" {
		return ""
	}
	if len(secret) <= 4 {
		return "****"
	}
	return "****" + secret[len(secret)-4:]
}

func nonEmpty(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return out
}

func clone(values []string) []string {
	return append([]string{}, values...)
}

func envInt(name string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(name))
	if err != nil {
		return fallback
	}
	return value
}
