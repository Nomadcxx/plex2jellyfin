package main

import (
	"fmt"
	"strings"
	"testing"

	setuppkg "github.com/Nomadcxx/plex2jellyfin/internal/setup"
)

func TestHandleTaskComplete_EmptyLibrariesStillRunsPostScanTasks(t *testing.T) {
	m := model{
		uninstallMode:     false,
		updateMode:        false,
		tvLibraryPaths:    "",
		movieLibraryPaths: "",
		tasks: []installTask{
			{name: "Write config", status: statusRunning},
		},
		currentTaskIndex: 0,
		postScanTasks: []installTask{
			{name: "Setup systemd", status: statusPending},
			{name: "Start service", status: statusPending},
		},
	}

	next, _ := m.handleTaskComplete(taskCompleteMsg{index: 0, success: true})
	got := next.(model)

	if got.step != stepInstalling {
		t.Fatalf("step = %v, want stepInstalling so systemd still runs", got.step)
	}
	if len(got.tasks) != 2 || got.tasks[0].name != "Setup systemd" {
		t.Fatalf("tasks = %v, want postScanTasks promoted", taskNames(got.tasks))
	}
	if got.postScanTasks != nil {
		t.Fatalf("postScanTasks should be cleared, got %v", taskNames(got.postScanTasks))
	}
}

func TestGenerateConfigString_SetupStartsIncomplete(t *testing.T) {
	m := &model{
		watchFolders:   []WatchFolder{{Type: "tv", Paths: "/watch/tv"}},
		tvLibraryPaths: "/lib/tv",
		serviceEnabled: true,
	}
	configStr, err := m.generateConfigString()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(configStr, "\ncompleted = false\n") {
		t.Fatalf("fresh write must leave setup incomplete, got:\n%s", configStr)
	}
	if strings.Contains(configStr, "\ncompleted = true\n") {
		t.Fatal("completed = true must not appear before listeners start")
	}
	if !strings.Contains(configStr, fmt.Sprintf("version = %d", setuppkg.CurrentVersion)) {
		t.Fatalf("expected setup version, got:\n%s", configStr)
	}
}

func TestMediaPathsValid_RequiresCompletePair(t *testing.T) {
	cases := []struct {
		name    string
		watchTV string
		libTV   string
		ok      bool
	}{
		{"empty", "", "", false},
		{"watch only", "/w/tv", "", false},
		{"lib only", "", "/l/tv", false},
		{"complete tv", "/w/tv", "/l/tv", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := model{
				watchFolders:   []WatchFolder{{Type: "tv", Paths: tc.watchTV}, {Type: "movies", Paths: ""}},
				tvLibraryPaths: tc.libTV,
			}
			err := m.mediaPathsError()
			if tc.ok && err != "" {
				t.Fatalf("want ok, got %q", err)
			}
			if !tc.ok && err == "" {
				t.Fatal("want error for incomplete media paths")
			}
		})
	}
}

func TestRenderComplete_HidesWebStartWhenUnitNotInstalled(t *testing.T) {
	m := model{
		webEnabled:   true,
		serviceState: &serviceRunState{daemonUnitWritten: true, daemonStarted: true},
	}
	out := m.renderComplete()
	if strings.Contains(out, "systemctl start plex2jellyfin-web") {
		t.Fatalf("must not advertise web start when web unit was not written:\n%s", out)
	}
	if strings.Contains(out, "The daemon is running and watching") {
		t.Fatal("must not claim daemon is watching unconditionally")
	}
}
