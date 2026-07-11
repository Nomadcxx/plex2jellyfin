package main

import (
	"strings"
	"testing"
)

func TestJellyfinInputs_IncludeAndSaveWebhookSecret(t *testing.T) {
	m := model{
		jellyfinURL:     "http://localhost:8096",
		jellyfinAPIKey:  "abc123",
		webhookSecret:   "initial-secret",
		jellyfinEnabled: true,
	}

	m.initJellyfinInputs()

	if len(m.inputs) != 3 {
		t.Fatalf("expected 3 jellyfin inputs (url, api key, webhook secret), got %d", len(m.inputs))
	}

	m.inputs[0].SetValue("http://jf.local:8096")
	m.inputs[1].SetValue("new-api-key")
	m.inputs[2].SetValue("new-webhook-secret")
	m.saveJellyfinInputs()

	if m.jellyfinURL != "http://jf.local:8096" {
		t.Fatalf("unexpected jellyfinURL %q", m.jellyfinURL)
	}
	if m.jellyfinAPIKey != "new-api-key" {
		t.Fatalf("unexpected jellyfinAPIKey %q", m.jellyfinAPIKey)
	}
	if m.webhookSecret != "new-webhook-secret" {
		t.Fatalf("unexpected webhookSecret %q", m.webhookSecret)
	}
}

func TestDefaultCallbackURL(t *testing.T) {
	m := &model{webEnabled: true, webPort: "5522"}
	url := m.defaultCallbackURL()
	if !strings.HasPrefix(url, "http://") || !strings.HasSuffix(url, ":5522") {
		t.Errorf("web enabled: got %q, want http://<host>:5522", url)
	}

	m.webPort = "18080"
	if url := m.defaultCallbackURL(); !strings.HasSuffix(url, ":18080") {
		t.Errorf("custom port: got %q, want suffix :18080", url)
	}

	m.webEnabled = false
	if url := m.defaultCallbackURL(); !strings.HasSuffix(url, ":8686") {
		t.Errorf("web disabled: got %q, want daemon health port :8686", url)
	}
}

func TestInitWebInputs_CallbackFieldGatedOnPluginConsent(t *testing.T) {
	m := &model{webEnabled: true, webPort: "5522", jellyfinEnabled: true, pluginInstall: true}
	m.initWebInputs()
	if len(m.inputs) != 2 {
		t.Fatalf("expected port + callback URL inputs, got %d", len(m.inputs))
	}
	if v := m.inputs[1].Value(); !strings.HasSuffix(v, ":5522") {
		t.Errorf("callback input should pre-fill derived default, got %q", v)
	}

	m2 := &model{webEnabled: true, webPort: "5522", jellyfinEnabled: true, pluginInstall: false}
	m2.initWebInputs()
	if len(m2.inputs) != 1 {
		t.Fatalf("expected only port input when plugin install declined, got %d", len(m2.inputs))
	}
}

func TestSaveWebInputs_CapturesCallbackURL(t *testing.T) {
	m := &model{webEnabled: true, webPort: "5522", jellyfinEnabled: true, pluginInstall: true}
	m.initWebInputs()
	m.inputs[1].SetValue("  http://10.0.0.5:9000  ")
	m.saveWebInputs()
	if m.pluginDaemonURL != "http://10.0.0.5:9000" {
		t.Errorf("pluginDaemonURL = %q, want trimmed URL", m.pluginDaemonURL)
	}
}
