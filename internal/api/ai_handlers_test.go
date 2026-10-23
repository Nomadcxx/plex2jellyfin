package api

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Nomadcxx/plex2jellyfin/internal/config"
)

func newTestAIHandler(t *testing.T, cfg *config.Config) http.Handler {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SUDO_USER", "")
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	return (&Server{cfg: cfg, ipc: &stubIPC{}}).apiRouter()
}

func TestAISettingsGetReturnsConfiguredValues(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.AI.Enabled = true
	cfg.AI.OllamaEndpoint = "http://ollama.local:11434"
	cfg.AI.Model = "qwen2.5:14b"
	cfg.AI.FallbackModel = "llama3.1:8b"
	cfg.AI.ConfidenceThreshold = 0.72
	cfg.AI.AutoResolveRisky = true
	handler := newTestAIHandler(t, cfg)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/ai/settings", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("GET /ai/settings status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	confidenceThreshold, ok := got["confidenceThreshold"].(float64)
	if got["enabled"] != true || got["defaultProvider"] != "ollama" || !ok || math.Abs(confidenceThreshold-cfg.AI.ConfidenceThreshold) > 1e-6 || got["autoApply"] != true || got["endpoint"] != cfg.AI.OllamaEndpoint || got["primaryModel"] != cfg.AI.Model || got["fallbackModel"] != cfg.AI.FallbackModel {
		t.Fatalf("GET /ai/settings = %#v", got)
	}
}

func TestAISettingsPutRoundTripsThroughGet(t *testing.T) {
	cfg := config.DefaultConfig()
	handler := newTestAIHandler(t, cfg)
	body := `{"enabled":true,"endpoint":"http://ollama.example:11434","primaryModel":"qwen2.5:14b","fallbackModel":"llama3.1:8b"}`

	putResponse := httptest.NewRecorder()
	handler.ServeHTTP(putResponse, httptest.NewRequest(http.MethodPut, "/ai/settings", strings.NewReader(body)))
	if putResponse.Code != http.StatusOK {
		t.Fatalf("PUT /ai/settings status = %d, want %d: %s", putResponse.Code, http.StatusOK, putResponse.Body.String())
	}

	getResponse := httptest.NewRecorder()
	handler.ServeHTTP(getResponse, httptest.NewRequest(http.MethodGet, "/ai/settings", nil))
	var got map[string]any
	if err := json.Unmarshal(getResponse.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if getResponse.Code != http.StatusOK || got["enabled"] != true || got["endpoint"] != "http://ollama.example:11434" || got["primaryModel"] != "qwen2.5:14b" || got["fallbackModel"] != "llama3.1:8b" {
		t.Fatalf("GET after PUT status=%d body=%#v", getResponse.Code, got)
	}
}

func TestAISettingsPutRestoresPreviousConfigWhenReloadFails(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SUDO_USER", "")
	cfg := config.DefaultConfig()
	cfg.AI.Model = "previous-model"
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	server := &Server{
		cfg: cfg,
		ipc: &stubIPC{result: json.RawMessage(`{"ok":false,"failed":[{"name":"ai","error":"invalid model"}]}`)},
	}
	response := httptest.NewRecorder()
	server.apiRouter().ServeHTTP(response, httptest.NewRequest(
		http.MethodPut,
		"/ai/settings",
		strings.NewReader(`{"primaryModel":"new-model"}`),
	))

	if response.Code != http.StatusBadGateway {
		t.Fatalf("PUT /ai/settings status = %d, want %d: %s", response.Code, http.StatusBadGateway, response.Body.String())
	}
	if !server.ipc.(*stubIPC).called {
		t.Fatal("expected daemon reload")
	}
	if cfg.AI.Model != "previous-model" {
		t.Fatalf("in-memory model = %q, want previous-model", cfg.AI.Model)
	}
	disk, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if disk.AI.Model != "previous-model" {
		t.Fatalf("saved model = %q, want previous-model", disk.AI.Model)
	}
}

func TestLLMProviderEndpointsReportConfiguredOllama(t *testing.T) {
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"models":[{"name":"qwen2.5:14b","size":4096,"details":{"family":"qwen","quantization_level":"Q4_K_M"}}]}`))
	}))
	defer ollama.Close()

	cfg := config.DefaultConfig()
	cfg.AI.Enabled = true
	cfg.AI.OllamaEndpoint = ollama.URL
	cfg.AI.Model = "qwen2.5:14b"
	handler := newTestAIHandler(t, cfg)

	listResponse := httptest.NewRecorder()
	handler.ServeHTTP(listResponse, httptest.NewRequest(http.MethodGet, "/llm-providers", nil))
	var providers []map[string]any
	if err := json.Unmarshal(listResponse.Body.Bytes(), &providers); err != nil {
		t.Fatal(err)
	}
	if listResponse.Code != http.StatusOK || len(providers) != 1 {
		t.Fatalf("GET /llm-providers status=%d providers=%#v", listResponse.Code, providers)
	}
	provider := providers[0]
	if provider["id"] != "ollama" || provider["type"] != "ollama" || provider["enabled"] != true || provider["currentModel"] != cfg.AI.Model || provider["endpoint"] != ollama.URL {
		t.Fatalf("GET /llm-providers provider = %#v", provider)
	}

	statusResponse := httptest.NewRecorder()
	handler.ServeHTTP(statusResponse, httptest.NewRequest(http.MethodGet, "/llm-providers/ollama/status", nil))
	var status map[string]any
	if err := json.Unmarshal(statusResponse.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	models, _ := status["modelList"].([]any)
	if statusResponse.Code != http.StatusOK || status["online"] != true || status["model"] != cfg.AI.Model || len(models) != 1 {
		t.Fatalf("GET /llm-providers/ollama/status status=%d body=%#v", statusResponse.Code, status)
	}
}

func TestLLMProviderStatusReportsOllamaHTTPFailure(t *testing.T) {
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"unavailable"}`, http.StatusServiceUnavailable)
	}))
	defer ollama.Close()

	cfg := config.DefaultConfig()
	cfg.AI.OllamaEndpoint = ollama.URL
	handler := newTestAIHandler(t, cfg)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/llm-providers/ollama/status", nil))

	var status map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	errorMessage, _ := status["error"].(string)
	if response.Code != http.StatusOK || status["online"] != false || errorMessage == "" {
		t.Fatalf("GET /llm-providers/ollama/status status=%d body=%#v", response.Code, status)
	}
}
