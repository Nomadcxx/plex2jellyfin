package main

import "testing"

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
