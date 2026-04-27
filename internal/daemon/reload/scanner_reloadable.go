package reload

import (
	"context"

	"github.com/Nomadcxx/jellywatch/internal/config"
)

type WatchPathReplacer interface {
	ReplaceWatchPaths(paths []string) error
}

type scannerReloadable struct {
	watcher WatchPathReplacer
}

func NewScannerReloadable(watcher WatchPathReplacer) Reloadable {
	return &scannerReloadable{watcher: watcher}
}

func (r *scannerReloadable) Name() string {
	return "scanner"
}

func (r *scannerReloadable) Prepare(ctx context.Context, oldCfg, newCfg *config.Config) (Commit, Rollback, error) {
	newPaths := append([]string{}, newCfg.Watch.Movies...)
	newPaths = append(newPaths, newCfg.Watch.TV...)
	oldPaths := append([]string{}, oldCfg.Watch.Movies...)
	oldPaths = append(oldPaths, oldCfg.Watch.TV...)

	commit := func() error {
		return r.watcher.ReplaceWatchPaths(newPaths)
	}
	rollback := func() {
		_ = r.watcher.ReplaceWatchPaths(oldPaths)
	}
	return commit, rollback, nil
}
