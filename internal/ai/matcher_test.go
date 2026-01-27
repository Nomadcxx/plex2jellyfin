package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Nomadcxx/jellywatch/internal/config"
)

func TestMatcher_ParseMovie(t *testing.T) {
	// Create mock Ollama server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/generate" {
			response := GenerateResponse{
				Model: "test-model",
				Response: `{
					"title": "The Matrix",
					"year": 1999,
					"type": "movie",
					"confidence": 0.98
				}`,
				Done: true,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}
	}))
	defer mockServer.Close()

	cfg := config.AIConfig{
		Enabled:           true,
		Model:             "test-model",
		OllamaEndpoint:    mockServer.URL,
		ConfidenceThreshold: 0.8,
		TimeoutSeconds:    30,
	}

	matcher, err := NewMatcher(cfg)
	if err != nil {
		t.Fatalf("Failed to create matcher: %v", err)
	}

	ctx := context.Background()
	result, err := matcher.Parse(ctx, "The.Matrix.1999.1080p.BluRay.x264-Group.mkv")

	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if result.Title != "The Matrix" {
		t.Errorf("Expected title 'The Matrix', got '%s'", result.Title)
	}

	if result.Year == nil || *result.Year != 1999 {
		t.Errorf("Expected year 1999, got %v", result.Year)
	}

	if result.Type != "movie" {
		t.Errorf("Expected type 'movie', got '%s'", result.Type)
	}

	if result.Confidence < 0.95 {
		t.Errorf("Expected high confidence, got %f", result.Confidence)
	}
}

func TestMatcher_ParseTVEpisode(t *testing.T) {
	// Create mock Ollama server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/generate" {
			response := GenerateResponse{
				Model: "test-model",
				Response: `{
					"title": "Breaking Bad",
					"type": "tv",
					"season": 1,
					"episodes": [1],
					"confidence": 0.95
				}`,
				Done: true,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}
	}))
	defer mockServer.Close()

	cfg := config.AIConfig{
		Enabled:           true,
		Model:             "test-model",
		OllamaEndpoint:    mockServer.URL,
		ConfidenceThreshold: 0.8,
		TimeoutSeconds:    30,
	}

	matcher, err := NewMatcher(cfg)
	if err != nil {
		t.Fatalf("Failed to create matcher: %v", err)
	}

	ctx := context.Background()
	result, err := matcher.Parse(ctx, "Breaking.Bad.S01E01.720p.HDTV.x264.mkv")

	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if result.Title != "Breaking Bad" {
		t.Errorf("Expected title 'Breaking Bad', got '%s'", result.Title)
	}

	if result.Type != "tv" {
		t.Errorf("Expected type 'tv', got '%s'", result.Type)
	}

	if result.Season == nil || *result.Season != 1 {
		t.Errorf("Expected season 1, got %v", result.Season)
	}

	if len(result.Episodes) != 1 || result.Episodes[0] != 1 {
		t.Errorf("Expected episodes [1], got %v", result.Episodes)
	}
}

func TestMatcher_ParseWithMarkdownCodeBlock(t *testing.T) {
	// Create mock Ollama server that returns markdown code block
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/generate" {
			response := GenerateResponse{
				Model: "test-model",
				Response: "```json\n{\n  \"title\": \"Inception\",\n  \"year\": 2010,\n  \"type\": \"movie\",\n  \"confidence\": 0.97\n}\n```",
				Done:  true,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}
	}))
	defer mockServer.Close()

	cfg := config.AIConfig{
		Enabled:           true,
		Model:             "test-model",
		OllamaEndpoint:    mockServer.URL,
		ConfidenceThreshold: 0.8,
		TimeoutSeconds:    30,
	}

	matcher, err := NewMatcher(cfg)
	if err != nil {
		t.Fatalf("Failed to create matcher: %v", err)
	}

	ctx := context.Background()
	result, err := matcher.Parse(ctx, "Inception.2010.1080p.BluRay.mkv")

	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if result.Title != "Inception" {
		t.Errorf("Expected title 'Inception', got '%s'", result.Title)
	}

	if result.Year == nil || *result.Year != 2010 {
		t.Errorf("Expected year 2010, got %v", result.Year)
	}
}

func TestMatcher_ParseWithAbsoluteEpisode(t *testing.T) {
	// Create mock Ollama server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/generate" {
			response := GenerateResponse{
				Model: "test-model",
				Response: `{
					"title": "One Piece",
					"type": "tv",
					"absolute_episode": 243,
					"confidence": 0.85
				}`,
				Done: true,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}
	}))
	defer mockServer.Close()

	cfg := config.AIConfig{
		Enabled:           true,
		Model:             "test-model",
		OllamaEndpoint:    mockServer.URL,
		ConfidenceThreshold: 0.8,
		TimeoutSeconds:    30,
	}

	matcher, err := NewMatcher(cfg)
	if err != nil {
		t.Fatalf("Failed to create matcher: %v", err)
	}

	ctx := context.Background()
	result, err := matcher.Parse(ctx, "One.Piece.E243.1080p.mkv")

	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if result.Title != "One Piece" {
		t.Errorf("Expected title 'One Piece', got '%s'", result.Title)
	}

	if result.AbsoluteEpisode == nil || *result.AbsoluteEpisode != 243 {
		t.Errorf("Expected absolute episode 243, got %v", result.AbsoluteEpisode)
	}

	if result.Type != "tv" {
		t.Errorf("Expected type 'tv', got '%s'", result.Type)
	}
}

func TestMatcher_ParseWithAirDate(t *testing.T) {
	// Create mock Ollama server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/generate" {
			response := GenerateResponse{
				Model: "test-model",
				Response: `{
					"title": "The Daily Show",
					"type": "tv",
					"air_date": "2024-01-09",
					"confidence": 0.88
				}`,
				Done: true,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}
	}))
	defer mockServer.Close()

	cfg := config.AIConfig{
		Enabled:           true,
		Model:             "test-model",
		OllamaEndpoint:    mockServer.URL,
		ConfidenceThreshold: 0.8,
		TimeoutSeconds:    30,
	}

	matcher, err := NewMatcher(cfg)
	if err != nil {
		t.Fatalf("Failed to create matcher: %v", err)
	}

	ctx := context.Background()
	result, err := matcher.Parse(ctx, "The.Daily.Show.2024.01.09.mkv")

	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if result.Title != "The Daily Show" {
		t.Errorf("Expected title 'The Daily Show', got '%s'", result.Title)
	}

	if result.AirDate != "2024-01-09" {
		t.Errorf("Expected air date '2024-01-09', got '%s'", result.AirDate)
	}

	if result.Type != "tv" {
		t.Errorf("Expected type 'tv', got '%s'", result.Type)
	}
}

func TestMatcher_ParseWithMultiEpisode(t *testing.T) {
	// Create mock Ollama server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/generate" {
			response := GenerateResponse{
				Model: "test-model",
				Response: `{
					"title": "Game of Thrones",
					"type": "tv",
					"season": 8,
					"episodes": [5, 6],
					"confidence": 0.90
				}`,
				Done: true,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}
	}))
	defer mockServer.Close()

	cfg := config.AIConfig{
		Enabled:           true,
		Model:             "test-model",
		OllamaEndpoint:    mockServer.URL,
		ConfidenceThreshold: 0.8,
		TimeoutSeconds:    30,
	}

	matcher, err := NewMatcher(cfg)
	if err != nil {
		t.Fatalf("Failed to create matcher: %v", err)
	}

	ctx := context.Background()
	result, err := matcher.Parse(ctx, "Game.of.Thrones.S08E05-E06.1080p.mkv")

	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if result.Title != "Game of Thrones" {
		t.Errorf("Expected title 'Game of Thrones', got '%s'", result.Title)
	}

	if result.Season == nil || *result.Season != 8 {
		t.Errorf("Expected season 8, got %v", result.Season)
	}

	if len(result.Episodes) != 2 || result.Episodes[0] != 5 || result.Episodes[1] != 6 {
		t.Errorf("Expected episodes [5, 6], got %v", result.Episodes)
	}
}

func TestMatcher_NewMatcherValidation(t *testing.T) {
	// Test missing model
	cfg := config.AIConfig{
		Enabled:        true,
		OllamaEndpoint: "http://localhost:11434",
	}
	_, err := NewMatcher(cfg)
	if err == nil {
		t.Error("Expected error for missing model")
	}

	// Test missing endpoint
	cfg = config.AIConfig{
		Enabled: true,
		Model:   "test-model",
	}
	_, err = NewMatcher(cfg)
	if err == nil {
		t.Error("Expected error for missing endpoint")
	}

	// Test invalid confidence threshold
	cfg = config.AIConfig{
		Enabled:             true,
		Model:               "test-model",
		OllamaEndpoint:      "http://localhost:11434",
		ConfidenceThreshold: 1.5,
	}
	_, err = NewMatcher(cfg)
	if err == nil {
		t.Error("Expected error for invalid confidence threshold")
	}

	// Test invalid timeout
	cfg = config.AIConfig{
		Enabled:             true,
		Model:               "test-model",
		OllamaEndpoint:      "http://localhost:11434",
		ConfidenceThreshold: 0.8,
		TimeoutSeconds:      0,
	}
	_, err = NewMatcher(cfg)
	if err == nil {
		t.Error("Expected error for invalid timeout")
	}

	// Test valid config
	cfg = config.AIConfig{
		Enabled:             true,
		Model:               "test-model",
		OllamaEndpoint:      "http://localhost:11434",
		ConfidenceThreshold: 0.8,
		TimeoutSeconds:      30,
	}
	matcher, err := NewMatcher(cfg)
	if err != nil {
		t.Errorf("Expected no error for valid config, got: %v", err)
	}
	if matcher == nil {
		t.Error("Expected matcher to be non-nil")
	}
}

func TestMatcher_ServerError(t *testing.T) {
	// Create mock server that returns error
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/generate" {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Internal Server Error"))
		}
	}))
	defer mockServer.Close()

	cfg := config.AIConfig{
		Enabled:           true,
		Model:             "test-model",
		OllamaEndpoint:    mockServer.URL,
		ConfidenceThreshold: 0.8,
		TimeoutSeconds:    30,
	}

	matcher, err := NewMatcher(cfg)
	if err != nil {
		t.Fatalf("Failed to create matcher: %v", err)
	}

	ctx := context.Background()
	_, err = matcher.Parse(ctx, "Some.Movie.2024.mkv")

	if err == nil {
		t.Error("Expected error for server error response")
	}
}

func TestMatcher_InvalidJSONResponse(t *testing.T) {
	// Create mock server that returns invalid JSON
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/generate" {
			response := GenerateResponse{
				Model:    "test-model",
				Response: "This is not valid JSON",
				Done:     true,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}
	}))
	defer mockServer.Close()

	cfg := config.AIConfig{
		Enabled:           true,
		Model:             "test-model",
		OllamaEndpoint:    mockServer.URL,
		ConfidenceThreshold: 0.8,
		TimeoutSeconds:    30,
	}

	matcher, err := NewMatcher(cfg)
	if err != nil {
		t.Fatalf("Failed to create matcher: %v", err)
	}

	ctx := context.Background()
	_, err = matcher.Parse(ctx, "Some.Movie.2024.mkv")

	if err == nil {
		t.Error("Expected error for invalid JSON response")
	}
}

func TestMatcher_IsAvailable(t *testing.T) {
	// Create mock server for tags endpoint
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"models": []}`))
		} else if r.URL.Path == "/api/generate" {
			response := GenerateResponse{
				Model: "test-model",
				Response: `{
					"title": "Test",
					"type": "movie",
					"confidence": 0.9
				}`,
				Done: true,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}
	}))
	defer mockServer.Close()

	cfg := config.AIConfig{
		Enabled:           true,
		Model:             "test-model",
		OllamaEndpoint:    mockServer.URL,
		ConfidenceThreshold: 0.8,
		TimeoutSeconds:    30,
	}

	matcher, err := NewMatcher(cfg)
	if err != nil {
		t.Fatalf("Failed to create matcher: %v", err)
	}

	ctx := context.Background()
	if !matcher.IsAvailable(ctx) {
		t.Error("Expected matcher to be available")
	}

	// GetConfig should return the config
	returnedCfg := matcher.GetConfig()
	if returnedCfg.Model != cfg.Model {
		t.Error("GetConfig returned incorrect config")
	}
}

func TestMatcher_IsAvailableNegative(t *testing.T) {
	// Create mock server that returns 404 for tags
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockServer.Close()

	cfg := config.AIConfig{
		Enabled:           true,
		Model:             "test-model",
		OllamaEndpoint:    mockServer.URL,
		ConfidenceThreshold: 0.8,
		TimeoutSeconds:    30,
	}

	matcher, err := NewMatcher(cfg)
	if err != nil {
		t.Fatalf("Failed to create matcher: %v", err)
	}

	ctx := context.Background()
	if matcher.IsAvailable(ctx) {
		t.Error("Expected matcher to be unavailable when tags returns 404")
	}
}

func TestMatcher_YearWithoutValue(t *testing.T) {
	// Create mock Ollama server that returns null year
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/generate" {
			response := GenerateResponse{
				Model: "test-model",
				Response: `{
					"title": "Some Show",
					"year": null,
					"type": "tv",
					"season": 1,
					"episodes": [1],
					"confidence": 0.85
				}`,
				Done: true,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		}
	}))
	defer mockServer.Close()

	cfg := config.AIConfig{
		Enabled:           true,
		Model:             "test-model",
		OllamaEndpoint:    mockServer.URL,
		ConfidenceThreshold: 0.8,
		TimeoutSeconds:    30,
	}

	matcher, err := NewMatcher(cfg)
	if err != nil {
		t.Fatalf("Failed to create matcher: %v", err)
	}

	ctx := context.Background()
	result, err := matcher.Parse(ctx, "Some.Show.S01E01.mkv")

	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if result.Title != "Some Show" {
		t.Errorf("Expected title 'Some Show', got '%s'", result.Title)
	}

	if result.Year != nil {
		t.Errorf("Expected nil year, got %v", *result.Year)
	}
}
