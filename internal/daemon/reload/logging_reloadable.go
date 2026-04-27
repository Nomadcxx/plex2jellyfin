package reload

import (
	"context"
	"errors"
	"strings"

	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/logging"
)

type loggingReloadable struct {
	logger *logging.Logger
}

func NewLoggingReloadable(logger *logging.Logger) Reloadable {
	return &loggingReloadable{logger: logger}
}

func (r *loggingReloadable) Name() string { return "logging" }

func (r *loggingReloadable) Prepare(ctx context.Context, oldCfg, newCfg *config.Config) (Commit, Rollback, error) {
	if r.logger == nil {
		return nil, nil, nil
	}
	target, err := parseReloadLogLevel(newCfg.Logging.Level)
	if err != nil {
		return nil, nil, err
	}
	prev := r.logger.GetLevel()
	return func() error {
			r.logger.SetLevel(target)
			return nil
		}, func() {
			r.logger.SetLevel(prev)
		}, nil
}

func parseReloadLogLevel(level string) (logging.Level, error) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return logging.LevelDebug, nil
	case "info", "":
		return logging.LevelInfo, nil
	case "warn", "warning":
		return logging.LevelWarn, nil
	case "error":
		return logging.LevelError, nil
	default:
		return logging.LevelInfo, errors.New("invalid log level: " + level)
	}
}
