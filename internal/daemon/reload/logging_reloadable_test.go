package reload

import (
	"context"
	"testing"

	"github.com/Nomadcxx/plex2jellyfin/internal/config"
	"github.com/Nomadcxx/plex2jellyfin/internal/logging"
)

func TestLoggingReloadableSwapsLevelOnCommit(t *testing.T) {
	logger, err := logging.New(logging.Config{Level: "info", File: t.TempDir() + "/test.log"})
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Close()

	r := NewLoggingReloadable(logger)
	oldCfg := &config.Config{Logging: config.LoggingConfig{Level: "info"}}
	newCfg := &config.Config{Logging: config.LoggingConfig{Level: "debug"}}
	commit, _, err := r.Prepare(context.Background(), oldCfg, newCfg)
	if err != nil {
		t.Fatal(err)
	}
	if logger.GetLevel() != logging.LevelInfo {
		t.Fatalf("level changed before commit: %s", logger.GetLevel())
	}
	if err := commit(); err != nil {
		t.Fatal(err)
	}
	if logger.GetLevel() != logging.LevelDebug {
		t.Fatalf("level after commit = %s", logger.GetLevel())
	}
}
