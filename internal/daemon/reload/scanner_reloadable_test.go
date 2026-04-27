package reload

import (
	"context"
	"reflect"
	"testing"

	"github.com/Nomadcxx/jellywatch/internal/config"
)

type fakeWatchPathReplacer struct {
	paths []string
}

func (f *fakeWatchPathReplacer) ReplaceWatchPaths(paths []string) error {
	f.paths = append([]string(nil), paths...)
	return nil
}

func TestScannerReloadableReplacesWatchPathsOnCommit(t *testing.T) {
	replacer := &fakeWatchPathReplacer{}
	r := NewScannerReloadable(replacer)

	oldCfg := &config.Config{
		Watch: config.WatchConfig{
			Movies: []string{"/old/movies"},
			TV:     []string{"/old/tv"},
		},
	}
	newCfg := &config.Config{
		Watch: config.WatchConfig{
			Movies: []string{"/new/movies"},
			TV:     []string{"/new/tv"},
		},
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

	want := []string{"/new/movies", "/new/tv"}
	if !reflect.DeepEqual(replacer.paths, want) {
		t.Fatalf("paths = %#v, want %#v", replacer.paths, want)
	}
}
