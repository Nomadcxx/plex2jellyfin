package api

import (
	"fmt"
	"strings"

	"github.com/Nomadcxx/jellywatch/api"
	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/radarr"
	"github.com/Nomadcxx/jellywatch/internal/sonarr"
)

// ManagerClient provides a unified interface for media manager operations
type ManagerClient interface {
	GetSystemStatus() (version string, err error)
	GetAllQueueItems() ([]QueueItemInfo, error)
	GetStuckItems() ([]QueueItemInfo, error)
	RemoveFromQueue(id int, blocklist bool) error
	ClearStuckItems(blocklist bool) (int, error)
}

// QueueItemInfo is a unified queue item representation
type QueueItemInfo struct {
	ID                    int64
	Title                 string
	Status                string
	TrackedDownloadStatus string
	Size                  int64
	SizeRemaining         int64
	TimeLeft              string
	DownloadClient        string
	ErrorMessage          string
}

// SonarrClientWrapper wraps Sonarr client to implement ManagerClient
type SonarrClientWrapper struct {
	client *sonarr.Client
}

func (w *SonarrClientWrapper) GetSystemStatus() (string, error) {
	status, err := w.client.GetSystemStatus()
	if err != nil {
		return "", err
	}
	return status.Version, nil
}

func (w *SonarrClientWrapper) GetAllQueueItems() ([]QueueItemInfo, error) {
	items, err := w.client.GetAllQueueItems()
	if err != nil {
		return nil, err
	}
	return convertSonarrQueueItems(items), nil
}

func (w *SonarrClientWrapper) GetStuckItems() ([]QueueItemInfo, error) {
	items, err := w.client.GetStuckItems()
	if err != nil {
		return nil, err
	}
	return convertSonarrQueueItems(items), nil
}

func (w *SonarrClientWrapper) RemoveFromQueue(id int, blocklist bool) error {
	return w.client.RemoveFromQueue(id, blocklist, false)
}

func (w *SonarrClientWrapper) ClearStuckItems(blocklist bool) (int, error) {
	return w.client.ClearStuckItems(blocklist)
}

// RadarrClientWrapper wraps Radarr client to implement ManagerClient
type RadarrClientWrapper struct {
	client *radarr.Client
}

func (w *RadarrClientWrapper) GetSystemStatus() (string, error) {
	status, err := w.client.GetSystemStatus()
	if err != nil {
		return "", err
	}
	return status.Version, nil
}

func (w *RadarrClientWrapper) GetAllQueueItems() ([]QueueItemInfo, error) {
	items, err := w.client.GetAllQueueItems()
	if err != nil {
		return nil, err
	}
	return convertRadarrQueueItems(items), nil
}

func (w *RadarrClientWrapper) GetStuckItems() ([]QueueItemInfo, error) {
	items, err := w.client.GetStuckItems()
	if err != nil {
		return nil, err
	}
	return convertRadarrQueueItems(items), nil
}

func (w *RadarrClientWrapper) RemoveFromQueue(id int, blocklist bool) error {
	return w.client.RemoveFromQueue(id, blocklist, false)
}

func (w *RadarrClientWrapper) ClearStuckItems(blocklist bool) (int, error) {
	return w.client.ClearStuckItems(blocklist)
}

// convertSonarrQueueItems converts Sonarr queue items to unified format
func convertSonarrQueueItems(items []sonarr.QueueItem) []QueueItemInfo {
	result := make([]QueueItemInfo, len(items))
	for i, item := range items {
		result[i] = QueueItemInfo{
			ID:                    int64(item.ID),
			Title:                 item.Title,
			Status:                item.Status,
			TrackedDownloadStatus: item.TrackedDownloadStatus,
			Size:                  item.Size,
			SizeRemaining:         item.Sizeleft,
			TimeLeft:              item.Timeleft,
			DownloadClient:        item.DownloadClient,
			ErrorMessage:          item.ErrorMessage,
		}
	}
	return result
}

// convertRadarrQueueItems converts Radarr queue items to unified format
func convertRadarrQueueItems(items []radarr.QueueItem) []QueueItemInfo {
	result := make([]QueueItemInfo, len(items))
	for i, item := range items {
		result[i] = QueueItemInfo{
			ID:                    int64(item.ID),
			Title:                 item.Title,
			Status:                item.Status,
			TrackedDownloadStatus: item.TrackedDownloadStatus,
			Size:                  item.Size,
			SizeRemaining:         item.Sizeleft,
			TimeLeft:              item.Timeleft,
			DownloadClient:        item.DownloadClient,
			ErrorMessage:          item.ErrorMessage,
		}
	}
	return result
}

// convertToAPIQueueItem converts unified QueueItemInfo to API QueueItem
func convertToAPIQueueItem(item QueueItemInfo) api.QueueItem {
	isStuck := isItemStuck(item.TrackedDownloadStatus)
	return api.QueueItem{
		Id:             &item.ID,
		Title:          &item.Title,
		Status:         &item.Status,
		Size:           &item.Size,
		SizeRemaining:  &item.SizeRemaining,
		TimeLeft:       &item.TimeLeft,
		DownloadClient: &item.DownloadClient,
		ErrorMessage:   &item.ErrorMessage,
		IsStuck:        &isStuck,
	}
}

// isItemStuck determines if a queue item is stuck based on tracked download status
func isItemStuck(status string) bool {
	s := strings.ToLower(status)
	return s == "warning" || s == "error"
}

// getManagerClient returns a client for the given manager ID
func getManagerClient(cfg *config.Config, managerId string) (ManagerClient, error) {
	switch managerId {
	case "sonarr":
		if !cfg.Sonarr.Enabled || cfg.Sonarr.URL == "" {
			return nil, fmt.Errorf("sonarr not configured")
		}
		client := sonarr.NewClient(sonarr.Config{
			URL:    cfg.Sonarr.URL,
			APIKey: cfg.Sonarr.APIKey,
		})
		return &SonarrClientWrapper{client: client}, nil
	case "radarr":
		if !cfg.Radarr.Enabled || cfg.Radarr.URL == "" {
			return nil, fmt.Errorf("radarr not configured")
		}
		client := radarr.NewClient(radarr.Config{
			URL:    cfg.Radarr.URL,
			APIKey: cfg.Radarr.APIKey,
		})
		return &RadarrClientWrapper{client: client}, nil
	default:
		return nil, fmt.Errorf("unknown manager: %s", managerId)
	}
}

// isManagerConfigured checks if a manager is configured and enabled
func isManagerConfigured(cfg *config.Config, managerId string) bool {
	switch managerId {
	case "sonarr":
		return cfg.Sonarr.Enabled && cfg.Sonarr.URL != ""
	case "radarr":
		return cfg.Radarr.Enabled && cfg.Radarr.URL != ""
	default:
		return false
	}
}