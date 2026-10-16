package daemon

import (
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/Nomadcxx/plex2jellyfin/internal/config"
	"github.com/Nomadcxx/plex2jellyfin/internal/naming"
	"github.com/Nomadcxx/plex2jellyfin/internal/organizer"
)

type sourcePolicySnapshot struct {
	watch        config.WatchConfig
	deleteSource bool
	allPaths     []string
}

type sourcePolicy struct {
	current atomic.Pointer[sourcePolicySnapshot]
}

func newSourcePolicy(watch config.WatchConfig, deleteSource bool) *sourcePolicy {
	policy := &sourcePolicy{}
	policy.Update(watch, deleteSource)
	return policy
}

func (p *sourcePolicy) Update(watch config.WatchConfig, deleteSource bool) {
	watch.TV = cleanUniquePaths(watch.TV)
	watch.Movies = cleanUniquePaths(watch.Movies)
	watch.SeedingTV = cleanUniquePaths(watch.SeedingTV)
	watch.SeedingMovies = cleanUniquePaths(watch.SeedingMovies)
	p.current.Store(&sourcePolicySnapshot{
		watch:        watch,
		deleteSource: deleteSource,
		allPaths:     cleanUniquePaths(watch.AllPaths()),
	})
}

func (p *sourcePolicy) Resolve(sourcePath string) organizer.SourceTransferMode {
	snapshot := p.current.Load()
	if snapshot == nil {
		return organizer.TransferCopy
	}
	if pathInRoots(sourcePath, snapshot.watch.SeedingTV) || pathInRoots(sourcePath, snapshot.watch.SeedingMovies) {
		return organizer.TransferSeed
	}
	if pathInRoots(sourcePath, snapshot.watch.TV) || pathInRoots(sourcePath, snapshot.watch.Movies) {
		if snapshot.deleteSource {
			return organizer.TransferMove
		}
		return organizer.TransferCopy
	}
	return organizer.TransferCopy
}

func (p *sourcePolicy) SourceHint(sourcePath string) naming.SourceHint {
	snapshot := p.current.Load()
	if snapshot == nil {
		return naming.SourceUnknown
	}
	if pathInRoots(sourcePath, snapshot.watch.TV) || pathInRoots(sourcePath, snapshot.watch.SeedingTV) {
		return naming.SourceTV
	}
	if pathInRoots(sourcePath, snapshot.watch.Movies) || pathInRoots(sourcePath, snapshot.watch.SeedingMovies) {
		return naming.SourceMovie
	}
	return naming.SourceUnknown
}

func (p *sourcePolicy) AllPaths() []string {
	snapshot := p.current.Load()
	if snapshot == nil {
		return nil
	}
	return append([]string(nil), snapshot.allPaths...)
}

func cleanUniquePaths(paths []string) []string {
	cleaned := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			continue
		}
		path = filepath.Clean(path)
		if absolute, err := filepath.Abs(path); err == nil {
			path = absolute
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		cleaned = append(cleaned, path)
	}
	return cleaned
}

func pathInRoots(path string, roots []string) bool {
	for _, root := range roots {
		if config.PathContains(root, path) {
			return true
		}
	}
	return false
}
