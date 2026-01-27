package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func (m model) testSonarr() (tea.Model, tea.Cmd) {
	m.sonarrTesting = true
	// Get current values from inputs (not the saved model fields)
	url := m.sonarrURL
	apiKey := m.sonarrAPIKey
	if len(m.inputs) >= 2 {
		url = m.inputs[0].Value()
		apiKey = m.inputs[1].Value()
	}
	return m, testSonarrCmd(url, apiKey)
}

func testSonarrCmd(url, apiKey string) tea.Cmd {
	return func() tea.Msg {
		client := &http.Client{Timeout: 10 * time.Second}
		req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/v3/system/status", url), nil)
		if err != nil {
			return apiTestResultMsg{service: "sonarr", success: false, err: err}
		}
		req.Header.Set("X-Api-Key", apiKey)

		resp, err := client.Do(req)
		if err != nil {
			return apiTestResultMsg{service: "sonarr", success: false, err: err}
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			return apiTestResultMsg{service: "sonarr", success: false, err: fmt.Errorf("HTTP %d", resp.StatusCode)}
		}

		body, _ := io.ReadAll(resp.Body)
		var result struct {
			Version string `json:"version"`
		}
		json.Unmarshal(body, &result)

		return apiTestResultMsg{service: "sonarr", success: true, version: result.Version}
	}
}

func (m model) testRadarr() (tea.Model, tea.Cmd) {
	m.radarrTesting = true
	// Get current values from inputs (not the saved model fields)
	url := m.radarrURL
	apiKey := m.radarrAPIKey
	if len(m.inputs) >= 2 {
		url = m.inputs[0].Value()
		apiKey = m.inputs[1].Value()
	}
	return m, testRadarrCmd(url, apiKey)
}

func testRadarrCmd(url, apiKey string) tea.Cmd {
	return func() tea.Msg {
		client := &http.Client{Timeout: 10 * time.Second}
		req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/v3/system/status", url), nil)
		if err != nil {
			return apiTestResultMsg{service: "radarr", success: false, err: err}
		}
		req.Header.Set("X-Api-Key", apiKey)

		resp, err := client.Do(req)
		if err != nil {
			return apiTestResultMsg{service: "radarr", success: false, err: err}
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			return apiTestResultMsg{service: "radarr", success: false, err: fmt.Errorf("HTTP %d", resp.StatusCode)}
		}

		body, _ := io.ReadAll(resp.Body)
		var result struct {
			Version string `json:"version"`
		}
		json.Unmarshal(body, &result)

		return apiTestResultMsg{service: "radarr", success: true, version: result.Version}
	}
}

func (m model) detectOllama() (tea.Model, tea.Cmd) {
	m.aiTesting = true
	m.aiState = aiStateUnknown
	return m, detectOllamaCmd(m.aiOllamaURL)
}

func detectOllamaCmd(url string) tea.Cmd {
	return func() tea.Msg {
		if !isOllamaInstalled() {
			return apiTestResultMsg{service: "ollama", success: false, err: fmt.Errorf("not installed")}
		}

		if !isOllamaRunning(url) {
			return apiTestResultMsg{service: "ollama", success: false, err: fmt.Errorf("not running")}
		}

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Get(fmt.Sprintf("%s/api/tags", url))
		if err != nil {
			return apiTestResultMsg{service: "ollama", success: false, err: err}
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		var result struct {
			Models []struct {
				Name string `json:"name"`
			} `json:"models"`
		}
		json.Unmarshal(body, &result)

		models := make([]string, len(result.Models))
		for i, m := range result.Models {
			models[i] = m.Name
		}

		if len(models) == 0 {
			return aiModelsMsg{models: nil, err: fmt.Errorf("no models")}
		}

		return aiModelsMsg{models: models, err: nil}
	}
}

func (m model) testOllamaConnection() (tea.Model, tea.Cmd) {
	m.aiTesting = true
	return m, testOllamaConnectionCmd(m.aiOllamaURL)
}

func testOllamaConnectionCmd(url string) tea.Cmd {
	return func() tea.Msg {
		start := time.Now()
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Get(fmt.Sprintf("%s/api/tags", url))
		duration := time.Since(start)

		if err != nil {
			return apiTestResultMsg{service: "ollama", success: false, err: err}
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			return apiTestResultMsg{service: "ollama", success: false, err: fmt.Errorf("HTTP %d", resp.StatusCode)}
		}

		return apiTestResultMsg{
			service: "ollama",
			success: true,
			version: fmt.Sprintf("Connected (%dms)", duration.Milliseconds()),
		}
	}
}

func (m model) testOllamaPrompt() (tea.Model, tea.Cmd) {
	m.aiPromptTesting = true
	return m, testOllamaPromptCmd(m.aiOllamaURL, m.aiModel)
}

func testOllamaPromptCmd(url, modelName string) tea.Cmd {
	return func() tea.Msg {
		start := time.Now()
		client := &http.Client{Timeout: 60 * time.Second}

		prompt := "Parse this filename and return JSON with title, year, type (movie or tv), and confidence (0-1):\nThe.Matrix.1999.2160p.UHD.BluRay.x265-GROUP\nReturn ONLY valid JSON, no explanation."

		// Use proper JSON marshaling to handle escaping
		reqData := map[string]interface{}{
			"model":  modelName,
			"prompt": prompt,
			"stream": false,
		}
		reqBody, _ := json.Marshal(reqData)

		resp, err := client.Post(
			fmt.Sprintf("%s/api/generate", url),
			"application/json",
			strings.NewReader(string(reqBody)),
		)

		duration := time.Since(start)

		if err != nil {
			return aiPromptTestMsg{success: false, err: err, duration: duration}
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		var result struct {
			Response string `json:"response"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			return aiPromptTestMsg{success: false, result: "Failed to parse API response", duration: duration}
		}

		valid, summary := validateTestResponse(result.Response)
		return aiPromptTestMsg{
			success:  valid,
			result:   summary,
			duration: duration,
			err:      nil,
		}
	}
}

func validateTestResponse(response string) (bool, string) {
	cleaned := strings.TrimSpace(response)

	// Handle markdown code blocks
	if strings.HasPrefix(cleaned, "```") {
		lines := strings.Split(cleaned, "\n")
		if len(lines) > 2 {
			// Remove first line (```json) and last line (```)
			cleaned = strings.Join(lines[1:len(lines)-1], "\n")
		}
	}

	// Try to extract JSON from response - some models output thinking/reasoning first
	// Look for JSON object pattern
	jsonStart := strings.Index(cleaned, "{")
	jsonEnd := strings.LastIndex(cleaned, "}")
	if jsonStart != -1 && jsonEnd != -1 && jsonEnd > jsonStart {
		cleaned = cleaned[jsonStart : jsonEnd+1]
	}

	// Clean up any remaining whitespace
	cleaned = strings.TrimSpace(cleaned)

	// Use interface{} for year to handle both int and string
	var rawResult map[string]interface{}
	if err := json.Unmarshal([]byte(cleaned), &rawResult); err != nil {
		return false, "Invalid JSON response"
	}

	// Extract title (try both cases)
	title := ""
	if t, ok := rawResult["title"].(string); ok {
		title = t
	} else if t, ok := rawResult["Title"].(string); ok {
		title = t
	}

	if title == "" {
		return false, "No title extracted"
	}

	// Extract year (handle int, float64 from JSON, or string)
	var year int
	if y, ok := rawResult["year"].(float64); ok {
		year = int(y)
	} else if y, ok := rawResult["Year"].(float64); ok {
		year = int(y)
	} else if y, ok := rawResult["year"].(string); ok {
		fmt.Sscanf(y, "%d", &year)
	} else if y, ok := rawResult["Year"].(string); ok {
		fmt.Sscanf(y, "%d", &year)
	}

	// Extract type
	mediaType := ""
	if t, ok := rawResult["type"].(string); ok {
		mediaType = strings.ToLower(t)
	} else if t, ok := rawResult["Type"].(string); ok {
		mediaType = strings.ToLower(t)
	}

	if mediaType != "movie" && mediaType != "tv" {
		return false, "Invalid type field"
	}

	// Extract confidence
	var confidence float64
	if c, ok := rawResult["confidence"].(float64); ok {
		confidence = c
	} else if c, ok := rawResult["Confidence"].(float64); ok {
		confidence = c
	}

	yearStr := ""
	if year > 0 {
		yearStr = fmt.Sprintf(" (%d)", year)
	}

	return true, fmt.Sprintf("%s%s - %.0f%% confidence",
		title, yearStr, confidence*100)
}

func (m model) handleAPITestResult(msg apiTestResultMsg) (tea.Model, tea.Cmd) {
	switch msg.service {
	case "sonarr":
		m.sonarrTesting = false
		m.sonarrTested = msg.success
		if msg.success {
			m.sonarrVersion = msg.version
		} else if msg.err != nil {
			m.sonarrVersion = msg.err.Error()
		}
	case "radarr":
		m.radarrTesting = false
		m.radarrTested = msg.success
		if msg.success {
			m.radarrVersion = msg.version
		} else if msg.err != nil {
			m.radarrVersion = msg.err.Error()
		}
	case "ollama":
		m.aiTesting = false
		if msg.success {
			m.aiTestResult = fmt.Sprintf("[OK] %s", msg.version)
		} else {
			if msg.err != nil && msg.err.Error() == "not installed" {
				m.aiState = aiStateNotInstalled
			} else if msg.err != nil && msg.err.Error() == "not running" {
				m.aiState = aiStateNotRunning
			}
			m.aiTestResult = fmt.Sprintf("[FAIL] %v", msg.err)
		}
	}
	return m, nil
}

func (m model) handleAIModels(msg aiModelsMsg) (tea.Model, tea.Cmd) {
	m.aiTesting = false
	if msg.err != nil {
		if msg.err.Error() == "no models" {
			m.aiState = aiStateNoModels
		}
		return m, nil
	}
	m.aiModels = msg.models
	m.aiState = aiStateReady
	if len(msg.models) > 0 {
		m.aiModelIndex = 0
		m.aiModel = msg.models[0]
	}
	return m, nil
}

func (m model) handleAIPromptTest(msg aiPromptTestMsg) (tea.Model, tea.Cmd) {
	m.aiPromptTesting = false
	if msg.success {
		m.aiPromptResult = fmt.Sprintf("[OK] %s (%.1fs)", msg.result, msg.duration.Seconds())
	} else {
		m.aiPromptResult = fmt.Sprintf("[FAIL] %s", msg.result)
	}
	return m, nil
}

func (m model) handleTaskComplete(msg taskCompleteMsg) (tea.Model, tea.Cmd) {
	if msg.success {
		m.tasks[msg.index].status = statusComplete
	} else {
		if m.tasks[msg.index].optional {
			m.tasks[msg.index].status = statusSkipped
			m.errors = append(m.errors, fmt.Sprintf("%s (skipped): %s", m.tasks[msg.index].name, msg.err))
		} else {
			m.tasks[msg.index].status = statusFailed
			m.errors = append(m.errors, fmt.Sprintf("%s: %s", m.tasks[msg.index].name, msg.err))
			m.step = stepComplete
			return m, nil
		}
	}

	m.currentTaskIndex++
	if m.currentTaskIndex >= len(m.tasks) {
		// If install mode (not uninstall) and we have libraries, scan BEFORE systemd setup
		if !m.uninstallMode && !m.updateMode && (m.tvLibraryPaths != "" || m.movieLibraryPaths != "") && len(m.postScanTasks) > 0 {
			m.step = stepScanning
			return m, tea.Batch(m.spinner.Tick, m.runInitialScan())
		}
		// No scan needed or uninstall - go to complete
		m.step = stepComplete
		return m, nil
	}

	m.tasks[m.currentTaskIndex].status = statusRunning
	return m, executeTaskCmd(m.currentTaskIndex, &m)
}

func executeTaskCmd(index int, m *model) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(m.inputDelay)
		err := m.tasks[index].execute(m)
		if err != nil {
			return taskCompleteMsg{index: index, success: false, err: err.Error()}
		}
		return taskCompleteMsg{index: index, success: true}
	}
}
