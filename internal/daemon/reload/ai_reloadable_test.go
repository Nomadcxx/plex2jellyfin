package reload

import (
	"context"
	"testing"

	"github.com/Nomadcxx/jellywatch/internal/config"
)

type fakeAIReconfigurer struct {
	cfg config.AIConfig
}

func (f *fakeAIReconfigurer) Reconfigure(cfg config.AIConfig) error {
	f.cfg = cfg
	return nil
}

func TestAIReloadableReconfiguresMatcherOnCommit(t *testing.T) {
	matcher := &fakeAIReconfigurer{}
	r := NewAIReloadable(matcher)

	next := &config.Config{
		AI: config.AIConfig{
			Enabled:        true,
			Model:          "llama3.1",
			OllamaEndpoint: "http://localhost:11434",
			TimeoutSeconds: 20,
		},
	}
	commit, rollback, err := r.Prepare(context.Background(), &config.Config{}, next)
	if err != nil {
		t.Fatal(err)
	}
	if rollback == nil {
		t.Fatal("rollback is nil")
	}
	if err := commit(); err != nil {
		t.Fatal(err)
	}

	if matcher.cfg.Model != next.AI.Model {
		t.Fatalf("model = %q, want %q", matcher.cfg.Model, next.AI.Model)
	}
}
