package config

import (
	"encoding/json"
	"testing"
)

func TestSectionsListsAllKnown(t *testing.T) {
	want := []string{
		"paths", "libraries", "sonarr", "radarr", "jellyfin",
		"ai", "daemon", "logging", "options", "permissions",
	}
	got := SectionNames()
	gotMap := make(map[string]bool)
	for _, s := range got {
		gotMap[s] = true
	}
	for _, w := range want {
		if !gotMap[w] {
			t.Errorf("missing section %q", w)
		}
	}
}

func TestGetSectionUsesMapstructureNames(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AI.OllamaEndpoint = "http://ollama"
	raw, err := GetSection(cfg, "ai")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got["ollama_endpoint"] != "http://ollama" {
		t.Fatalf("missing mapstructure key in %s", string(raw))
	}
	if _, ok := got["OllamaEndpoint"]; ok {
		t.Fatalf("unexpected Go field key in %s", string(raw))
	}
}

func TestSetSectionUsesMapstructureNames(t *testing.T) {
	cfg := DefaultConfig()
	raw := []byte(`{"enabled":true,"ollama_endpoint":"http://x","model":"m"}`)
	if err := SetSection(cfg, "ai", raw); err != nil {
		t.Fatal(err)
	}
	if !cfg.AI.Enabled || cfg.AI.OllamaEndpoint != "http://x" || cfg.AI.Model != "m" {
		t.Fatalf("config not updated: %+v", cfg.AI)
	}
}

func TestSetWatchSectionUsesSeedingPathNames(t *testing.T) {
	cfg := DefaultConfig()
	raw := []byte(`{"tv":["/ordinary/tv"],"movies":["/ordinary/movies"],"seeding_tv":["/seed/tv"],"seeding_movies":["/seed/movies"]}`)
	if err := SetSection(cfg, "paths", raw); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Watch.SeedingTV) != 1 || cfg.Watch.SeedingTV[0] != "/seed/tv" {
		t.Fatalf("seeding TV paths not decoded: %+v", cfg.Watch)
	}
	if len(cfg.Watch.SeedingMovies) != 1 || cfg.Watch.SeedingMovies[0] != "/seed/movies" {
		t.Fatalf("seeding movie paths not decoded: %+v", cfg.Watch)
	}
}

func TestSetSectionUnknown(t *testing.T) {
	if err := SetSection(DefaultConfig(), "bogus", []byte("{}")); err == nil {
		t.Fatal("expected error")
	}
}
