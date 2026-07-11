package main

import (
	"strings"
	"testing"
)

func jellyfinTestModel() model {
	m := model{
		step:            stepIntegrationsJellyfin,
		jellyfinEnabled: true,
		jellyfinTested:  true,
		jellyfinVersion: "10.11.11",
		pluginInstall:   true,
		pluginRestart:   true,
	}
	m.initJellyfinInputs()
	return m
}

func TestRenderJellyfin_PluginTogglesVisibleAfterSuccessfulTest(t *testing.T) {
	m := jellyfinTestModel()

	out := m.renderJellyfin()
	for _, want := range []string{
		"Install companion plugin: Yes",
		"Restart Jellyfin after install: Yes",
		"(recommended)",
		"closes the feedback loop",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in Jellyfin screen, got:\n%s", want, out)
		}
	}
}

func TestRenderJellyfin_TogglesHiddenBeforeTest(t *testing.T) {
	m := jellyfinTestModel()
	m.jellyfinTested = false

	if out := m.renderJellyfin(); strings.Contains(out, "Install companion plugin") {
		t.Fatalf("toggles should not render before a successful connection test:\n%s", out)
	}
}

func TestHandleJellyfinKeys_TogglesPluginConsent(t *testing.T) {
	m := jellyfinTestModel()
	m.focusedInput = len(m.inputs) + 1 // install toggle row

	next, _ := m.handleJellyfinKeys("up")
	got := next.(model)
	if got.pluginInstall {
		t.Error("expected up on install row to toggle pluginInstall off")
	}

	got.focusedInput = len(got.inputs) + 2 // restart toggle row
	next, _ = got.handleJellyfinKeys("down")
	got = next.(model)
	if got.pluginRestart {
		t.Error("expected down on restart row to toggle pluginRestart off")
	}
}

func TestNextJellyfinInput_CyclesThroughToggleRows(t *testing.T) {
	m := jellyfinTestModel()
	// rows: 0 enable, 1-3 inputs, 4 install toggle, 5 restart toggle
	m.focusedInput = 5
	next, _ := m.nextJellyfinInput()
	if got := next.(model); got.focusedInput != 0 {
		t.Errorf("expected focus to wrap 5 -> 0, got %d", got.focusedInput)
	}

	m.jellyfinTested = false // toggles hidden: old arithmetic
	m.focusedInput = 3
	next, _ = m.nextJellyfinInput()
	if got := next.(model); got.focusedInput != 0 {
		t.Errorf("expected focus to wrap 3 -> 0 when toggles hidden, got %d", got.focusedInput)
	}
}
