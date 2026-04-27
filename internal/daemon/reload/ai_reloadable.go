package reload

import (
	"context"

	"github.com/Nomadcxx/jellywatch/internal/config"
)

type AIReconfigurer interface {
	Reconfigure(cfg config.AIConfig) error
}

type aiReloadable struct {
	matcher AIReconfigurer
}

func NewAIReloadable(matcher AIReconfigurer) Reloadable {
	return &aiReloadable{matcher: matcher}
}

func (r *aiReloadable) Name() string {
	return "ai"
}

func (r *aiReloadable) Prepare(ctx context.Context, oldCfg, newCfg *config.Config) (Commit, Rollback, error) {
	oldAI := oldCfg.AI
	newAI := newCfg.AI
	commit := func() error {
		return r.matcher.Reconfigure(newAI)
	}
	rollback := func() {
		_ = r.matcher.Reconfigure(oldAI)
	}
	return commit, rollback, nil
}
