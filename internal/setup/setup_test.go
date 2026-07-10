package setup

import (
	"strings"
	"testing"

	"github.com/Nomadcxx/plex2jellyfin/internal/config"
)

func TestNeedsSetupState(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.Config
		want bool
	}{
		{name: "blank", cfg: config.DefaultConfig(), want: true},
		{name: "legacy tv", cfg: configWithPaths([]string{"/watch/tv"}, []string{"/library/tv"}, nil, nil), want: false},
		{name: "legacy movies", cfg: configWithPaths(nil, nil, []string{"/watch/movies"}, []string{"/library/movies"}), want: false},
		{name: "legacy incomplete", cfg: configWithPaths([]string{"/watch/tv"}, nil, nil, nil), want: true},
		{name: "versioned incomplete", cfg: versionedConfig(false), want: true},
		{name: "versioned complete", cfg: versionedConfig(true), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NeedsSetup(tt.cfg); got != tt.want {
				t.Fatalf("NeedsSetup() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateDraftAllowsEitherCompleteMediaType(t *testing.T) {
	for _, draft := range []Draft{
		validDraft([]string{"/watch/tv"}, []string{"/library/tv"}, nil, nil),
		validDraft(nil, nil, []string{"/watch/movies"}, []string{"/library/movies"}),
	} {
		if errs := ValidateDraft(draft, RuntimeInfo{Kind: RuntimeNative}); len(errs) != 0 {
			t.Fatalf("valid draft rejected: %+v", errs)
		}
	}
}

func TestValidateDraftRejectsBlankAndHalfPairs(t *testing.T) {
	tests := []Draft{
		validDraft(nil, nil, nil, nil),
		validDraft([]string{"/watch/tv"}, nil, nil, nil),
		validDraft(nil, []string{"/library/tv"}, nil, nil),
		validDraft(nil, nil, []string{"/watch/movies"}, nil),
	}
	for _, draft := range tests {
		if errs := ValidateDraft(draft, RuntimeInfo{Kind: RuntimeNative}); len(errs) == 0 {
			t.Fatalf("invalid media paths accepted: %+v", draft)
		}
	}
}

func TestValidateDraftRejectsInvalidRuntimeAndIntegrationValues(t *testing.T) {
	draft := validDraft([]string{"/watch/tv"}, []string{"/library/tv"}, nil, nil)
	draft.Runtime.ScanFrequency = "not-a-duration"
	draft.Runtime.Permissions = config.PermissionsConfig{User: "jellyfin", FileMode: "999"}
	draft.Sonarr = ServiceDraft{Enabled: true, URL: "sonarr", APIKey: ""}
	draft.AI = AIDraft{Enabled: true, Endpoint: "localhost:11434"}

	errs := ValidateDraft(draft, RuntimeInfo{Kind: RuntimeContainer})
	joined := fieldErrors(errs)
	for _, field := range []string{"runtime.scan_frequency", "runtime.permissions", "sonarr.url", "sonarr.api_key", "ai.endpoint", "ai.primary_model"} {
		if !strings.Contains(joined, field) {
			t.Fatalf("missing validation for %s in %+v", field, errs)
		}
	}
}

func TestApplyDraftPreservesMaskedSecretsAndUnmanagedConfig(t *testing.T) {
	current := config.DefaultConfig()
	current.PasswordHash = "password-hash"
	current.Sonarr.APIKey = "sonarr-secret"
	current.Jellyfin.APIKey = "jellyfin-secret"
	current.Jellyfin.WebhookSecret = "webhook-secret"
	current.Logging.Level = "debug"

	draft := validDraft([]string{"/watch/tv"}, []string{"/library/tv"}, nil, nil)
	draft.Sonarr = ServiceDraft{Enabled: true, URL: "http://sonarr:8989", APIKey: "****cret"}
	draft.Jellyfin = JellyfinDraft{Enabled: true, URL: "http://jellyfin:8096", APIKey: "****cret"}

	got := ApplyDraft(current, draft)
	if got.PasswordHash != "password-hash" || got.Logging.Level != "debug" {
		t.Fatalf("unmanaged config was overwritten: %+v", got)
	}
	if got.Sonarr.APIKey != "sonarr-secret" || got.Jellyfin.APIKey != "jellyfin-secret" || got.Jellyfin.WebhookSecret != "webhook-secret" {
		t.Fatalf("masked secrets were not preserved: sonarr=%q jellyfin=%q webhook=%q", got.Sonarr.APIKey, got.Jellyfin.APIKey, got.Jellyfin.WebhookSecret)
	}
	if !got.Daemon.Enabled || got.Setup.Version != CurrentVersion || got.Setup.Completed {
		t.Fatalf("apply did not prepare incomplete daemon config: daemon=%+v setup=%+v", got.Daemon, got.Setup)
	}
}

func TestDetectRuntimeUsesContainerEnvironment(t *testing.T) {
	t.Setenv("container", "docker")
	t.Setenv("PUID", "1234")
	t.Setenv("PGID", "5678")
	got := DetectRuntime()
	if got.Kind != RuntimeContainer || got.UID != 1234 || got.GID != 5678 {
		t.Fatalf("unexpected runtime: %+v", got)
	}
}

func configWithPaths(tvWatch, tvLibrary, movieWatch, movieLibrary []string) *config.Config {
	cfg := config.DefaultConfig()
	cfg.Watch.TV = tvWatch
	cfg.Libraries.TV = tvLibrary
	cfg.Watch.Movies = movieWatch
	cfg.Libraries.Movies = movieLibrary
	return cfg
}

func versionedConfig(completed bool) *config.Config {
	cfg := configWithPaths([]string{"/watch/tv"}, []string{"/library/tv"}, nil, nil)
	cfg.Setup.Version = CurrentVersion
	cfg.Setup.Completed = completed
	return cfg
}

func validDraft(tvWatch, tvLibrary, movieWatch, movieLibrary []string) Draft {
	return Draft{
		Watch:     PathsDraft{TV: tvWatch, Movies: movieWatch},
		Libraries: PathsDraft{TV: tvLibrary, Movies: movieLibrary},
		Runtime:   RuntimeDraft{ScanFrequency: "5m", DeleteSource: true},
		Jellyfin:  JellyfinDraft{},
		Sonarr:    ServiceDraft{},
		Radarr:    ServiceDraft{},
		AI:        AIDraft{},
	}
}

func fieldErrors(errs []FieldError) string {
	var fields []string
	for _, err := range errs {
		fields = append(fields, err.Field)
	}
	return strings.Join(fields, ",")
}
