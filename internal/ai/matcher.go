// Package ai provides experimental AI-powered title matching for media files.
//
// ⚠️  EXPERIMENTAL - NOT YET INTEGRATED
// This package contains prototype code for AI-based title matching but is not
// currently integrated into the main workflow. All parsing currently uses
// regex-based methods in internal/naming.
//
// To integrate: Pass OptionalMatcher to scanner/organizer and enable in config.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/config"
)

// Result represents the AI's parsed output
type Result struct {
	Title           string  `json:"title"`
	Year            *int    `json:"year,omitempty"`
	Type            string  `json:"type"` // "movie" or "tv"
	Season          *int    `json:"season,omitempty"`
	Episodes        []int   `json:"episodes,omitempty"`
	AbsoluteEpisode *int    `json:"absolute_episode,omitempty"`
	AirDate         string  `json:"air_date,omitempty"`
	Confidence      float64 `json:"confidence"`
}

// Matcher handles AI-based title matching
type Matcher struct {
	config       config.AIConfig
	client       *http.Client
	systemPrompt string
}

// NewMatcher creates a new AI matcher
func NewMatcher(cfg config.AIConfig) (*Matcher, error) {
	if cfg.Enabled && cfg.Model == "" {
		return nil, fmt.Errorf("AI enabled but no model specified")
	}
	if cfg.Enabled && cfg.OllamaEndpoint == "" {
		return nil, fmt.Errorf("AI enabled but no Ollama endpoint specified")
	}
	if cfg.ConfidenceThreshold < 0 || cfg.ConfidenceThreshold > 1 {
		return nil, fmt.Errorf("confidence threshold must be between 0 and 1")
	}
	if cfg.TimeoutSeconds < 1 {
		return nil, fmt.Errorf("timeout must be at least 1 second")
	}

	return &Matcher{
		config: cfg,
		client: &http.Client{
			Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second,
		},
		systemPrompt: getSystemPrompt(),
	}, nil
}

// Parse sends a filename to Ollama and returns parsed metadata
func (m *Matcher) Parse(ctx context.Context, filename string) (*Result, error) {
	return m.parseWithModel(ctx, filename, m.config.Model)
}

// ParseWithCloud sends a filename to Ollama cloud model and returns parsed metadata
func (m *Matcher) ParseWithCloud(ctx context.Context, filename string) (*Result, error) {
	if m.config.CloudModel == "" {
		return nil, fmt.Errorf("no cloud model configured")
	}
	return m.parseWithModel(ctx, filename, m.config.CloudModel)
}

// parseWithModel sends a filename to a specific model
func (m *Matcher) parseWithModel(ctx context.Context, filename, model string) (*Result, error) {
	// Construct full prompt
	fullPrompt := m.systemPrompt + "\n" + filename

	reqBody := GenerateRequest{
		Model:  model,
		Prompt: fullPrompt,
		Stream: false,
	}

	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request with context
	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(m.config.TimeoutSeconds)*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, "POST", m.config.OllamaEndpoint+"/api/generate", bytes.NewReader(reqJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Send request
	startTime := time.Now()
	resp, err := m.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to Ollama: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama returned %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var genResp GenerateResponse
	if err := json.NewDecoder(resp.Body).Decode(&genResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	latency := time.Since(startTime)

	// Strip markdown code blocks if present
	responseText := strings.TrimSpace(genResp.Response)
	if strings.HasPrefix(responseText, "```json") {
		responseText = strings.TrimPrefix(responseText, "```json")
	} else if strings.HasPrefix(responseText, "```") {
		responseText = strings.TrimPrefix(responseText, "```")
	}
	if strings.HasSuffix(responseText, "```") {
		responseText = strings.TrimSuffix(responseText, "```")
	}
	responseText = strings.TrimSpace(responseText)

	// Parse AI's JSON response
	var result Result
	if err := json.Unmarshal([]byte(responseText), &result); err != nil {
		return nil, fmt.Errorf("failed to parse AI response: %w (response: %s)", err, genResp.Response)
	}

	// Log latency for debugging
	if os.Getenv("DEBUG_AI") == "1" {
		fmt.Printf("[AI] Parsed '%s' in %v with %.2f confidence\n", filename, latency, result.Confidence)
	}

	return &result, nil
}

// IsAvailable checks if Ollama is running
func (m *Matcher) IsAvailable(ctx context.Context) bool {
	reqCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, "GET", m.config.OllamaEndpoint+"/api/tags", nil)
	if err != nil {
		return false
	}

	resp, err := m.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// GetConfig returns the matcher's configuration
func (m *Matcher) GetConfig() config.AIConfig {
	return m.config
}

// generateRequest is the request structure for Ollama API
type GenerateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

// generateResponse is the response structure from Ollama API
type GenerateResponse struct {
	Model    string `json:"model"`
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

// getSystemPrompt returns the optimized system prompt for title extraction
func getSystemPrompt() string {
	return `You are a media filename parser. Your job is to extract clean metadata from messy release filenames.

## Rules
1. Remove release group tags (e.g., "-GROUP", "-RARBG", "-YTS")
2. Remove quality indicators (e.g., "1080p", "720p", "2160p", "4K", "UHD")
3. Remove source indicators (e.g., "BluRay", "WEB-DL", "REMUX", "HDTV")
4. Remove codec markers (e.g., "x264", "x265", "HEVC", "H264", "AVC")
5. Remove audio markers (e.g., "DTS", "AAC", "AC3", "TrueHD", "Atmos", "DDP5.1")
6. Remove HDR markers (e.g., "HDR", "DoVi", "Dolby.Vision", "HDR10+")
7. Remove streaming sources (e.g., "AMZN", "NF", "DSNP", "ATVP", "HULU")
8. Remove special edition markers (e.g., "EXTENDED", "REMASTERED", "Directors Cut")
9. Preserve proper punctuation in titles (add back apostrophes, colons, commas where appropriate)
10. Extract year from anywhere in filename (prefer earliest year if multiple)
11. Return confidence score (0.0-1.0) based on certainty
12. For TV shows: extract season, episode(s), absolute episode, or air date

## Output Format
Return ONLY valid JSON with this exact structure:

For movies:
{
  "title": "The Matrix",
  "year": 1999,
  "type": "movie",
  "confidence": 0.98
}

For TV shows:
{
  "title": "Breaking Bad",
  "year": null,
  "type": "tv",
  "season": 1,
  "episodes": [1],
  "confidence": 0.95
}

For multi-episode files:
{
  "title": "Game of Thrones",
  "type": "tv",
  "season": 8,
  "episodes": [5, 6],
  "confidence": 0.90
}

For absolute episode numbering:
{
  "title": "One Piece",
  "type": "tv",
  "absolute_episode": 243,
  "confidence": 0.85
}

For date-based episodes:
{
  "title": "The Daily Show",
  "type": "tv",
  "air_date": "2024-01-09",
  "confidence": 0.85
}

## Confidence Scoring
- 0.95-1.0: Very confident (standard format, clear markers)
- 0.85-0.94: Confident (some ambiguity but likely correct)
- 0.75-0.84: Moderate (multiple interpretations possible)
- 0.0-0.74: Low (highly ambiguous, may need user confirmation)

Lower confidence when:
- Ambiguous titles (e.g., "The Office" without US/UK indicator)
- Multiple years present
- Unusual numbering schemes
- Foreign language content
- Very short/generic titles

Now parse this filename:`
}
