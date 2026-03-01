package main

import (
	"regexp"
	"strings"
	"testing"
)

func TestStartInstallation_WebUIDisabledSkipsWebTasks(t *testing.T) {
	m := model{
		serviceEnabled: true,
		webEnabled:     false,
	}

	next, _ := m.startInstallation()
	got := next.(model)

	for _, task := range got.postScanTasks {
		if strings.Contains(strings.ToLower(task.name), "web service") {
			t.Fatalf("expected no web service tasks when web UI disabled, found task %q", task.name)
		}
	}
}

func TestBuildWebServiceUnit_UsesConfiguredPort(t *testing.T) {
	unit := buildWebServiceUnit("nomadx", "18080")
	if !strings.Contains(unit, "ExecStart=/usr/local/bin/jellyweb --host 0.0.0.0 --port 18080") {
		t.Fatalf("expected configured port in service unit, got:\n%s", unit)
	}
	if !strings.Contains(unit, "Environment=SUDO_USER=nomadx") {
		t.Fatalf("expected SUDO_USER in service unit, got:\n%s", unit)
	}
}

func TestGenerateConfigString_IncludesJellyfinWebhookSecret(t *testing.T) {
	m := &model{
		watchFolders: []WatchFolder{
			{Type: "movies", Paths: "/watch/movies"},
			{Type: "tv", Paths: "/watch/tv"},
		},
		movieLibraryPaths: "/lib/movies",
		tvLibraryPaths:    "/lib/tv",
		serviceEnabled:    true,
		scanFrequency:     0,
		permUser:          "nomadx",
		permGroup:         "media",
		permFileMode:      "0664",
		permDirMode:       "0775",
		jellyfinEnabled:   true,
		jellyfinURL:       "http://localhost:8096",
		jellyfinAPIKey:    "jf-api",
		webhookSecret:     "my-explicit-secret",
	}

	configStr, err := m.generateConfigString()
	if err != nil {
		t.Fatalf("generateConfigString() error = %v", err)
	}

	if !strings.Contains(configStr, `webhook_secret = "my-explicit-secret"`) {
		t.Fatalf("expected explicit webhook secret in config, got:\n%s", configStr)
	}
}

func TestGenerateConfigString_AutoGeneratesWebhookSecret(t *testing.T) {
	m := &model{
		watchFolders: []WatchFolder{
			{Type: "movies", Paths: "/watch/movies"},
			{Type: "tv", Paths: "/watch/tv"},
		},
		movieLibraryPaths: "/lib/movies",
		tvLibraryPaths:    "/lib/tv",
		serviceEnabled:    true,
		scanFrequency:     0,
		permUser:          "nomadx",
		permGroup:         "media",
		permFileMode:      "0664",
		permDirMode:       "0775",
		jellyfinEnabled:   true,
		jellyfinURL:       "http://localhost:8096",
		jellyfinAPIKey:    "jf-api",
		webhookSecret:     "",
	}

	configStr, err := m.generateConfigString()
	if err != nil {
		t.Fatalf("generateConfigString() error = %v", err)
	}

	if strings.Contains(configStr, `webhook_secret = ""`) {
		t.Fatalf("expected generated webhook secret, got empty value:\n%s", configStr)
	}

	re := regexp.MustCompile(`webhook_secret = "([a-f0-9]{64})"`)
	matches := re.FindStringSubmatch(configStr)
	if len(matches) != 2 {
		t.Fatalf("expected generated 64-char hex webhook secret, got:\n%s", configStr)
	}
	if m.webhookSecret != matches[1] {
		t.Fatalf("expected model webhookSecret to match generated value")
	}
}
