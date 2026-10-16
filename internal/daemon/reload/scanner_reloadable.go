package reload

import (
	"context"

	"github.com/Nomadcxx/plex2jellyfin/internal/config"
)

type WatchPathReplacer interface {
	ReplaceWatchPaths(paths []string) error
}

type PeriodicWatchPathReplacer interface {
	ReplaceWatchPaths(paths []string)
}

type SourcePolicyReplacer interface {
	ReplaceSourcePolicy(watch config.WatchConfig, deleteSource bool)
}

type scannerReloadable struct {
	watcher  WatchPathReplacer
	periodic PeriodicWatchPathReplacer
	policy   SourcePolicyReplacer
}

func NewScannerReloadable(watcher WatchPathReplacer, periodic PeriodicWatchPathReplacer, policy SourcePolicyReplacer) Reloadable {
	return &scannerReloadable{watcher: watcher, periodic: periodic, policy: policy}
}

func (r *scannerReloadable) Name() string {
	return "scanner"
}

func (r *scannerReloadable) Prepare(ctx context.Context, oldCfg, newCfg *config.Config) (Commit, Rollback, error) {
	if err := config.ValidateWatchLayout(newCfg.Watch, newCfg.Libraries); err != nil {
		return nil, nil, err
	}
	newPaths := newCfg.Watch.AllPaths()
	oldPaths := oldCfg.Watch.AllPaths()

	commit := func() error {
		r.policy.ReplaceSourcePolicy(newCfg.Watch, newCfg.Options.DeleteSource)
		r.periodic.ReplaceWatchPaths(newPaths)
		return r.watcher.ReplaceWatchPaths(newPaths)
	}
	rollback := func() {
		r.policy.ReplaceSourcePolicy(oldCfg.Watch, oldCfg.Options.DeleteSource)
		r.periodic.ReplaceWatchPaths(oldPaths)
		_ = r.watcher.ReplaceWatchPaths(oldPaths)
	}
	return commit, rollback, nil
}
