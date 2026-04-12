package notify

import (
	"path/filepath"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/radarr"
)

type RadarrNotifier struct {
	client  *radarr.Client
	enabled bool
}

func NewRadarrNotifier(client *radarr.Client, enabled bool) *RadarrNotifier {
	return &RadarrNotifier{
		client:  client,
		enabled: enabled && client != nil,
	}
}

func (n *RadarrNotifier) Name() string {
	return "radarr"
}

func (n *RadarrNotifier) Enabled() bool {
	return n.enabled
}

func (n *RadarrNotifier) Ping() error {
	if !n.enabled {
		return nil
	}
	return n.client.Ping()
}

func (n *RadarrNotifier) Notify(event OrganizationEvent) *NotifyResult {
	start := time.Now()
	result := &NotifyResult{
		Service: n.Name(),
	}

	if event.MediaType != MediaTypeMovie {
		result.Skipped = true
		result.Duration = time.Since(start)
		return result
	}

	targetDir := filepath.Dir(event.TargetPath)
	resp, err := n.client.TriggerDownloadedMoviesScan(targetDir)
	if err != nil {
		result.Error = err
		result.Duration = time.Since(start)
		return result
	}

	result.Success = true
	result.CommandID = resp.ID
	result.Duration = time.Since(start)
	return result
}
