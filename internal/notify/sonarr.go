package notify

import (
	"path/filepath"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/sonarr"
)

type SonarrNotifier struct {
	client  *sonarr.Client
	enabled bool
}

func NewSonarrNotifier(client *sonarr.Client, enabled bool) *SonarrNotifier {
	return &SonarrNotifier{
		client:  client,
		enabled: enabled && client != nil,
	}
}

func (n *SonarrNotifier) Name() string {
	return "sonarr"
}

func (n *SonarrNotifier) Enabled() bool {
	return n.enabled
}

func (n *SonarrNotifier) Ping() error {
	if !n.enabled {
		return nil
	}
	return n.client.Ping()
}

func (n *SonarrNotifier) Notify(event OrganizationEvent) *NotifyResult {
	start := time.Now()
	result := &NotifyResult{
		Service: n.Name(),
	}

	if event.MediaType != MediaTypeTVEpisode {
		result.Skipped = true
		result.Duration = time.Since(start)
		return result
	}

	targetDir := filepath.Dir(event.TargetPath)
	resp, err := n.client.TriggerDownloadedEpisodesScan(targetDir)
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
