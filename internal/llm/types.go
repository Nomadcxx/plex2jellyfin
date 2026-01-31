package llm

// ProviderType represents different LLM provider implementations
type ProviderType string

const (
	ProviderTypeOllama    ProviderType = "ollama"
	ProviderTypeOpenAI    ProviderType = "openai"
	ProviderTypeAnthropic ProviderType = "anthropic"
	ProviderTypeLMStudio  ProviderType = "lmstudio"
	ProviderTypeCustom    ProviderType = "custom"
)

// ProviderStatus represents the health and status of an LLM provider
type ProviderStatus struct {
	Online    bool    `json:"online"`
	Model     string  `json:"model"`
	ModelList []Model `json:"modelList,omitempty"`
	Error     string  `json:"error,omitempty"`
}

// Model represents an available LLM model
type Model struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Size         string `json:"size,omitempty"`
	Quantization string `json:"quantization,omitempty"`
	Family       string `json:"family,omitempty"`
}

// CompletionOptions represents options for an LLM completion request
type CompletionOptions struct {
	Temperature float64 `json:"temperature,omitempty"`
	MaxTokens   int     `json:"maxTokens,omitempty"`
	TopP        float64 `json:"topP,omitempty"`
	Stream      bool    `json:"stream,omitempty"`
}

// Completion represents an LLM response
type Completion struct {
	Text         string `json:"text"`
	Model        string `json:"model"`
	UsedTokens   int    `json:"usedTokens"`
	FinishReason string `json:"finishReason,omitempty"`
}

// ProviderCapabilities indicates what a provider supports
type ProviderCapabilities struct {
	SupportsStreaming   bool `json:"supportsStreaming"`
	SupportsVision      bool `json:"supportsVision"`
	SupportsModelSwitch bool `json:"supportsModelSwitch"`
	LocalOnly           bool `json:"localOnly"`
}

// ProviderInfo represents summary info about an LLM provider
type ProviderInfo struct {
	ID           string               `json:"id"`
	Type         ProviderType         `json:"type"`
	Name         string               `json:"name"`
	Endpoint     string               `json:"endpoint"`
	Enabled      bool                 `json:"enabled"`
	CurrentModel string               `json:"currentModel"`
	Capabilities ProviderCapabilities `json:"capabilities"`
}

// AISettings represents AI configuration
type AISettings struct {
	Enabled             bool    `json:"enabled"`
	DefaultProvider     string  `json:"defaultProvider"`
	ConfidenceThreshold float64 `json:"confidenceThreshold"`
	AutoApply           bool    `json:"autoApply"`
}

// AuditSuggestion represents an AI-generated suggestion for fixing a parse
type AuditSuggestion struct {
	ID             int64   `json:"id"`
	FilePath       string  `json:"filePath"`
	CurrentParse   string  `json:"currentParse"`
	SuggestedParse string  `json:"suggestedParse"`
	Confidence     float64 `json:"confidence"`
	Reasoning      string  `json:"reasoning"`
	Status         string  `json:"status"`
	CreatedAt      string  `json:"createdAt"`
}
