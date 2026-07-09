package reload

import (
	"context"
	"errors"
	"testing"

	"github.com/Nomadcxx/plex2jellyfin/internal/config"
)

type fakeReloadable struct {
	name       string
	prepErr    error
	commitErr  error
	committed  bool
	rolledBack bool
}

func (f *fakeReloadable) Name() string { return f.name }

func (f *fakeReloadable) Prepare(ctx context.Context, oldCfg, newCfg *config.Config) (Commit, Rollback, error) {
	if f.prepErr != nil {
		return nil, nil, f.prepErr
	}
	return func() error {
			if f.commitErr != nil {
				return f.commitErr
			}
			f.committed = true
			return nil
		}, func() {
			f.rolledBack = true
		}, nil
}

func TestSupervisorAllSucceed(t *testing.T) {
	a := &fakeReloadable{name: "a"}
	b := &fakeReloadable{name: "b"}
	sup := NewSupervisor()
	sup.Register(a)
	sup.Register(b)

	res := sup.Reload(context.Background(), &config.Config{}, &config.Config{})
	if !res.OK {
		t.Fatalf("expected OK, got %+v", res)
	}
	if !a.committed || !b.committed {
		t.Errorf("subsystems not committed")
	}
}

func TestSupervisorRollbackOnPrepareFailure(t *testing.T) {
	a := &fakeReloadable{name: "a"}
	b := &fakeReloadable{name: "b", prepErr: errors.New("nope")}
	sup := NewSupervisor()
	sup.Register(a)
	sup.Register(b)

	res := sup.Reload(context.Background(), &config.Config{}, &config.Config{})
	if res.OK {
		t.Fatal("expected failure")
	}
	if a.committed {
		t.Error("a should not have committed")
	}
	if !a.rolledBack {
		t.Error("a should have rolled back")
	}
}

func TestSupervisorReportsCommitFailure(t *testing.T) {
	a := &fakeReloadable{name: "a", commitErr: errors.New("commit failed")}
	sup := NewSupervisor()
	sup.Register(a)

	res := sup.Reload(context.Background(), &config.Config{}, &config.Config{})
	if res.OK {
		t.Fatal("expected failure")
	}
	if len(res.Failed) != 1 || res.Failed[0].Name != "a" {
		t.Fatalf("unexpected failures: %+v", res.Failed)
	}
}
