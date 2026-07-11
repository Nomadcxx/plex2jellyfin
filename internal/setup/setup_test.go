package setup

import (
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

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

func TestAdoptLegacyCompletion(t *testing.T) {
	tests := []struct {
		name        string
		cfg         *config.Config
		wantAdopted bool
	}{
		{name: "legacy complete adopts", cfg: configWithPaths([]string{"/watch/tv"}, []string{"/library/tv"}, nil, nil), wantAdopted: true},
		{name: "legacy incomplete untouched", cfg: configWithPaths([]string{"/watch/tv"}, nil, nil, nil), wantAdopted: false},
		{name: "blank untouched", cfg: config.DefaultConfig(), wantAdopted: false},
		{name: "versioned complete untouched", cfg: versionedConfig(true), wantAdopted: false},
		{name: "versioned incomplete untouched", cfg: versionedConfig(false), wantAdopted: false},
		{name: "negative version untouched", cfg: configWithSetupVersion(-1), wantAdopted: false},
		{name: "nil untouched", cfg: nil, wantAdopted: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var beforeVersion int
			var beforeCompleted bool
			if tt.cfg != nil {
				beforeVersion = tt.cfg.Setup.Version
				beforeCompleted = tt.cfg.Setup.Completed
			}

			if got := AdoptLegacyCompletion(tt.cfg); got != tt.wantAdopted {
				t.Fatalf("AdoptLegacyCompletion() = %v, want %v", got, tt.wantAdopted)
			}

			if tt.cfg == nil {
				return
			}
			if tt.wantAdopted {
				if tt.cfg.Setup.Version != CurrentVersion || !tt.cfg.Setup.Completed {
					t.Fatalf("adopted config not stamped: %+v", tt.cfg.Setup)
				}
				if NeedsSetup(tt.cfg) {
					t.Fatal("adopted config must not need setup")
				}
			} else if tt.cfg.Setup.Version != beforeVersion || tt.cfg.Setup.Completed != beforeCompleted {
				t.Fatalf("non-adopted config was mutated: %+v", tt.cfg.Setup)
			}
		})
	}
}

func TestAdoptLegacyCompletionPreservesConcurrentConfigChange(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp+"/.config")
	t.Setenv("SUDO_USER", "")

	legacy := configWithPaths([]string{"/watch/tv"}, []string{"/library/tv"}, nil, nil)
	if err := legacy.Save(); err != nil {
		t.Fatal(err)
	}
	path, err := config.ConfigPath()
	if err != nil {
		t.Fatal(err)
	}

	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}

	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		close(started)
		_, err := config.UpdateWithLock(AdoptLegacyCompletion)
		done <- err
	}()
	<-started
	select {
	case err := <-done:
		t.Fatalf("update completed while another writer held the lock: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	newer := configWithPaths([]string{"/watch/tv"}, []string{"/library/tv"}, nil, nil)
	newer.Logging.Level = "debug"
	if err := os.WriteFile(path, []byte(newer.ToTOML()), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Logging.Level != "debug" {
		t.Fatalf("concurrent config change was lost: logging.level = %q", loaded.Logging.Level)
	}
	if loaded.Setup.Version != CurrentVersion || !loaded.Setup.Completed {
		t.Fatalf("setup marker missing after locked update: %+v", loaded.Setup)
	}
}

// The stamp must survive a real Save/Load cycle - this is what protects a
// legacy user whose config paths are later hand-edited (the heuristic would
// flip, the explicit marker must not).
func TestAdoptLegacyCompletionPersistsThroughSaveLoad(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp+"/.config")
	t.Setenv("SUDO_USER", "")

	cfg := configWithPaths([]string{"/watch/tv"}, []string{"/library/tv"}, nil, nil)
	if !AdoptLegacyCompletion(cfg) {
		t.Fatal("expected adoption")
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("save adopted config: %v", err)
	}

	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("reload adopted config: %v", err)
	}
	if loaded.Setup.Version != CurrentVersion || !loaded.Setup.Completed {
		t.Fatalf("marker did not survive round-trip: %+v", loaded.Setup)
	}

	// The heuristic-breaking edit: wipe the media pair. Explicit marker wins.
	loaded.Watch.TV = nil
	loaded.Libraries.TV = nil
	if NeedsSetup(loaded) {
		t.Fatal("explicit marker must keep setup complete after path edits")
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

func configWithSetupVersion(version int) *config.Config {
	cfg := configWithPaths([]string{"/watch/tv"}, []string{"/library/tv"}, nil, nil)
	cfg.Setup.Version = version
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
