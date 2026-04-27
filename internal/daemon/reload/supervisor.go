package reload

import (
	"context"
	"sync"

	"github.com/Nomadcxx/jellywatch/internal/config"
)

type Commit func() error
type Rollback func()

type Reloadable interface {
	Name() string
	Prepare(ctx context.Context, oldCfg, newCfg *config.Config) (Commit, Rollback, error)
}

type Result struct {
	OK       bool              `json:"ok"`
	Reloaded []string          `json:"reloaded"`
	Failed   []FailedSubsystem `json:"failed,omitempty"`
}

type FailedSubsystem struct {
	Name  string `json:"name"`
	Error string `json:"error"`
}

type Supervisor struct {
	mu  sync.Mutex
	all []Reloadable
}

func NewSupervisor() *Supervisor {
	return &Supervisor{}
}

func (s *Supervisor) Register(r Reloadable) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.all = append(s.all, r)
}

func (s *Supervisor) Reload(ctx context.Context, oldCfg, newCfg *config.Config) Result {
	s.mu.Lock()
	subs := append([]Reloadable(nil), s.all...)
	s.mu.Unlock()

	type prepared struct {
		name     string
		commit   Commit
		rollback Rollback
	}
	var preparedSubs []prepared
	for _, sub := range subs {
		commit, rollback, err := sub.Prepare(ctx, oldCfg, newCfg)
		if err != nil {
			for i := len(preparedSubs) - 1; i >= 0; i-- {
				if preparedSubs[i].rollback != nil {
					preparedSubs[i].rollback()
				}
			}
			return Result{
				OK:     false,
				Failed: []FailedSubsystem{{Name: sub.Name(), Error: err.Error()}},
			}
		}
		preparedSubs = append(preparedSubs, prepared{name: sub.Name(), commit: commit, rollback: rollback})
	}

	res := Result{OK: true}
	for _, sub := range preparedSubs {
		if sub.commit == nil {
			res.Reloaded = append(res.Reloaded, sub.name)
			continue
		}
		if err := sub.commit(); err != nil {
			res.OK = false
			res.Failed = append(res.Failed, FailedSubsystem{Name: sub.name, Error: err.Error()})
			continue
		}
		res.Reloaded = append(res.Reloaded, sub.name)
	}
	return res
}
