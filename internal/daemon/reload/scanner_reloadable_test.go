package reload

import (
	"context"
	"reflect"
	"testing"

	"github.com/Nomadcxx/plex2jellyfin/internal/config"
)

type fakeWatchPathReplacer struct {
	paths []string
}

func (f *fakeWatchPathReplacer) ReplaceWatchPaths(paths []string) error {
	f.paths = append([]string(nil), paths...)
	return nil
}

type fakePeriodicPathReplacer struct {
	paths []string
}

func (f *fakePeriodicPathReplacer) ReplaceWatchPaths(paths []string) {
	f.paths = append([]string(nil), paths...)
}

type fakeSourcePolicyReplacer struct {
	watch        config.WatchConfig
	deleteSource bool
}

func (f *fakeSourcePolicyReplacer) ReplaceSourcePolicy(watch config.WatchConfig, deleteSource bool) {
	f.watch = watch
	f.deleteSource = deleteSource
}

func TestScannerReloadableReplacesWatchPathsOnCommit(t *testing.T) {
	replacer := &fakeWatchPathReplacer{}
	periodic := &fakePeriodicPathReplacer{}
	policy := &fakeSourcePolicyReplacer{}
	r := NewScannerReloadable(replacer, periodic, policy)

	oldCfg := &config.Config{
		Watch: config.WatchConfig{
			Movies:        []string{"/old/movies"},
			TV:            []string{"/old/tv"},
			SeedingMovies: []string{"/old/seed-movies"},
			SeedingTV:     []string{"/old/seed-tv"},
		},
		Options: config.OptionsConfig{DeleteSource: true},
	}
	newCfg := &config.Config{
		Watch: config.WatchConfig{
			Movies:        []string{"/new/movies"},
			TV:            []string{"/new/tv"},
			SeedingMovies: []string{"/new/seed-movies"},
			SeedingTV:     []string{"/new/seed-tv"},
		},
		Options: config.OptionsConfig{DeleteSource: false},
	}

	commit, rollback, err := r.Prepare(context.Background(), oldCfg, newCfg)
	if err != nil {
		t.Fatal(err)
	}
	if rollback == nil {
		t.Fatal("rollback is nil")
	}
	if err := commit(); err != nil {
		t.Fatal(err)
	}

	want := newCfg.Watch.AllPaths()
	if !reflect.DeepEqual(replacer.paths, want) {
		t.Fatalf("paths = %#v, want %#v", replacer.paths, want)
	}
	if !reflect.DeepEqual(periodic.paths, want) {
		t.Fatalf("periodic paths = %#v, want %#v", periodic.paths, want)
	}
	if !reflect.DeepEqual(policy.watch, newCfg.Watch) || policy.deleteSource {
		t.Fatalf("policy = %+v delete=%v, want new policy with delete=false", policy.watch, policy.deleteSource)
	}

	rollback()
	if !reflect.DeepEqual(replacer.paths, oldCfg.Watch.AllPaths()) {
		t.Fatalf("watcher rollback paths = %#v, want %#v", replacer.paths, oldCfg.Watch.AllPaths())
	}
	if !reflect.DeepEqual(periodic.paths, oldCfg.Watch.AllPaths()) {
		t.Fatalf("periodic rollback paths = %#v, want %#v", periodic.paths, oldCfg.Watch.AllPaths())
	}
	if !reflect.DeepEqual(policy.watch, oldCfg.Watch) || !policy.deleteSource {
		t.Fatalf("policy rollback = %+v delete=%v, want old policy with delete=true", policy.watch, policy.deleteSource)
	}
}

func TestScannerReloadableRejectsInvalidWatchLayoutDuringPrepare(t *testing.T) {
	replacer := &fakeWatchPathReplacer{}
	periodic := &fakePeriodicPathReplacer{}
	policy := &fakeSourcePolicyReplacer{}
	r := NewScannerReloadable(replacer, periodic, policy)
	oldCfg := &config.Config{}
	newCfg := &config.Config{
		Watch:     config.WatchConfig{SeedingTV: []string{"/media/tv/incoming"}},
		Libraries: config.LibrariesConfig{TV: []string{"/media/tv"}},
	}

	if _, _, err := r.Prepare(context.Background(), oldCfg, newCfg); err == nil {
		t.Fatal("expected invalid watch layout error")
	}
	if len(replacer.paths) != 0 || len(periodic.paths) != 0 || len(policy.watch.AllPaths()) != 0 {
		t.Fatal("prepare mutated runtime state")
	}
}
