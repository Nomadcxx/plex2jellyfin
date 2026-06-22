package systemduser

import (
	"os"
	"strings"
	"testing"
)

func readUnitFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func TestPostmortemUserUnits(t *testing.T) {
	service := readUnitFile(t, "jellywatch-postmortem.service")
	timer := readUnitFile(t, "jellywatch-postmortem.timer")

	for _, want := range []string{
		"WorkingDirectory=/home/nomadx/Documents/jellywatch",
		"ExecStart=/usr/local/bin/jellywatch postmortem collect --since 96h",
		"ExecStartPost=/home/nomadx/Documents/jellywatch/scripts/jellywatch-postmortem-terminal.sh",
	} {
		if !strings.Contains(service, want) {
			t.Fatalf("service missing %q", want)
		}
	}
	for _, want := range []string{
		"OnUnitActiveSec=96h",
		"OnBootSec=20m",
		"Persistent=true",
	} {
		if !strings.Contains(timer, want) {
			t.Fatalf("timer missing %q", want)
		}
	}
}
