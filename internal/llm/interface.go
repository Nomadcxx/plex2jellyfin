package llm

import (
	"context"
	"fmt"
)

// LLMProvider defines the interface for LLM provider integrations.
// Implementations wrap specific providers (Ollama, OpenAI, etc.) behind
// a unified interface.
type LLMProvider interface {
	// Identity
	ID() string
	Type() ProviderType
	Name() string
	Info() ProviderInfo

	// Health
	Ping(ctx context.Context) error
	Status(ctx context.Context) (*ProviderStatus, error)

	// Models
	ListModels(ctx context.Context) ([]Model, error)
	CurrentModel() string
	SetModel(model string) error

	// Core operations
	Complete(ctx context.Context, prompt string, opts CompletionOptions) (*Completion, error)

	// Capabilities
	Capabilities() ProviderCapabilities
}

// ProviderRegistry manages LLM provider instances
type ProviderRegistry struct {
	providers       map[string]LLMProvider
	defaultProvider string
}

// NewProviderRegistry creates a new provider registry
func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		providers: make(map[string]LLMProvider),
	}
}

// Register adds a provider to the registry
func (r *ProviderRegistry) Register(provider LLMProvider) {
	r.providers[provider.ID()] = provider
	if r.defaultProvider == "" {
		r.defaultProvider = provider.ID()
	}
}

// Get returns a provider by ID
func (r *ProviderRegistry) Get(id string) (LLMProvider, bool) {
	p, ok := r.providers[id]
	return p, ok
}

// Default returns the default provider
func (r *ProviderRegistry) Default() (LLMProvider, bool) {
	return r.Get(r.defaultProvider)
}

// SetDefault sets the default provider
func (r *ProviderRegistry) SetDefault(id string) error {
	if _, ok := r.providers[id]; !ok {
		return fmt.Errorf("provider %s not found", id)
	}
	r.defaultProvider = id
	return nil
}

// All returns all registered providers
func (r *ProviderRegistry) All() []LLMProvider {
	result := make([]LLMProvider, 0, len(r.providers))
	for _, p := range r.providers {
		result = append(result, p)
	}
	return result
}

// AllInfo returns info for all registered providers
func (r *ProviderRegistry) AllInfo() []ProviderInfo {
	result := make([]ProviderInfo, 0, len(r.providers))
	for _, p := range r.providers {
		result = append(result, p.Info())
	}
	return result
}
