package mediamanager

import (
	"context"
	"fmt"

	"github.com/Nomadcxx/jellywatch/internal/sonarr"
)

// SonarrAdapter wraps the Sonarr client to implement MediaManager
type SonarrAdapter struct {
	id     string
	name   string
	client *sonarr.Client
}

// NewSonarrAdapter creates a new Sonarr adapter
func NewSonarrAdapter(id, name string, client *sonarr.Client) *SonarrAdapter {
	return &SonarrAdapter{
		id:     id,
		name:   name,
		client: client,
	}
}

func (s *SonarrAdapter) ID() string        { return s.id }
func (s *SonarrAdapter) Type() ManagerType { return ManagerTypeSonarr }
func (s *SonarrAdapter) Name() string      { return s.name }

func (s *SonarrAdapter) Info() ManagerInfo {
	return ManagerInfo{
		ID:           s.id,
		Type:         s.Type(),
		Name:         s.name,
		Enabled:      true,
		Capabilities: s.Capabilities(),
	}
}

func (s *SonarrAdapter) Ping(ctx context.Context) error {
	return s.client.Ping()
}

func (s *SonarrAdapter) Status(ctx context.Context) (*ServiceStatus, error) {
	status := &ServiceStatus{}

	sysStatus, err := s.client.GetSystemStatus()
	if err != nil {
		status.Online = false
		status.Error = err.Error()
		return status, nil
	}

	status.Online = true
	status.Version = sysStatus.Version

	queue, err := s.client.GetQueue(1, 1)
	if err == nil {
		status.QueueSize = queue.TotalRecords
	}

	stuck, err := s.client.GetStuckItems()
	if err == nil {
		status.StuckCount = len(stuck)
	}

	return status, nil
}

func (s *SonarrAdapter) GetQueue(ctx context.Context, filter QueueFilter) ([]QueueItem, error) {
	limit := filter.Limit
	if limit == 0 {
		limit = 50
	}

	queue, err := s.client.GetQueue(1, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get Sonarr queue: %w", err)
	}

	items := make([]QueueItem, 0, len(queue.Records))
	for _, r := range queue.Records {
		isStuck := r.TrackedDownloadStatus == "warning" || r.TrackedDownloadStatus == "error"

		if filter.IsStuck != nil && *filter.IsStuck != isStuck {
			continue
		}

		item := QueueItem{
			ID:             int64(r.ID),
			Title:          r.Title,
			Status:         r.Status,
			Progress:       calculateProgress(r.Size, r.Sizeleft),
			Size:           r.Size,
			SizeRemaining:  r.Sizeleft,
			TimeLeft:       r.Timeleft,
			IsStuck:        isStuck,
			DownloadClient: r.DownloadClient,
		}

		if len(r.StatusMessages) > 0 {
			item.ErrorMessage = r.StatusMessages[0].Title
		}

		items = append(items, item)
	}

	return items, nil
}

func (s *SonarrAdapter) GetStuckItems(ctx context.Context) ([]QueueItem, error) {
	stuck, err := s.client.GetStuckItems()
	if err != nil {
		return nil, fmt.Errorf("failed to get stuck items: %w", err)
	}

	items := make([]QueueItem, len(stuck))
	for i, r := range stuck {
		items[i] = QueueItem{
			ID:      int64(r.ID),
			Title:   r.Title,
			Status:  r.Status,
			IsStuck: true,
		}
		if len(r.StatusMessages) > 0 {
			items[i].ErrorMessage = r.StatusMessages[0].Title
		}
	}

	return items, nil
}

func (s *SonarrAdapter) GetQueueItem(ctx context.Context, id int64) (*QueueItem, error) {
	item, err := s.client.GetQueueItem(int(id))
	if err != nil {
		return nil, err
	}

	isStuck := item.TrackedDownloadStatus == "warning" || item.TrackedDownloadStatus == "error"

	return &QueueItem{
		ID:             int64(item.ID),
		Title:          item.Title,
		Status:         item.Status,
		Progress:       calculateProgress(item.Size, item.Sizeleft),
		Size:           item.Size,
		SizeRemaining:  item.Sizeleft,
		TimeLeft:       item.Timeleft,
		IsStuck:        isStuck,
		DownloadClient: item.DownloadClient,
	}, nil
}

func (s *SonarrAdapter) ClearItem(ctx context.Context, id int64, blocklist bool) error {
	return s.client.RemoveFromQueue(int(id), blocklist, false)
}

func (s *SonarrAdapter) ClearStuckItems(ctx context.Context, blocklist bool) (int, error) {
	return s.client.ClearStuckItems(blocklist)
}

func (s *SonarrAdapter) RetryItem(ctx context.Context, id int64) error {
	return s.client.GrabQueueItem(int(id))
}

func (s *SonarrAdapter) TriggerImportScan(ctx context.Context, path string) error {
	_, err := s.client.TriggerDownloadedEpisodesScan(path)
	return err
}

func (s *SonarrAdapter) ForceSync(ctx context.Context) error {
	_, err := s.client.RefreshAllSeries()
	return err
}

func (s *SonarrAdapter) GetRootFolders(ctx context.Context) ([]RootFolder, error) {
	folders, err := s.client.GetRootFolders()
	if err != nil {
		return nil, err
	}

	result := make([]RootFolder, len(folders))
	for i, f := range folders {
		result[i] = RootFolder{
			ID:        int64(f.ID),
			Path:      f.Path,
			FreeSpace: f.FreeSpace,
		}
	}

	return result, nil
}

func (s *SonarrAdapter) GetQualityProfiles(ctx context.Context) ([]QualityProfile, error) {
	// Not implemented in current Sonarr client
	return nil, nil
}

func (s *SonarrAdapter) GetHistory(ctx context.Context, opts HistoryOptions) ([]HistoryItem, error) {
	// Use the client's GetHistory method
	history, err := s.client.GetHistory(opts.Offset/opts.Limit+1, opts.Limit)
	if err != nil {
		return nil, err
	}

	items := make([]HistoryItem, len(history.Records))
	for i, r := range history.Records {
		items[i] = HistoryItem{
			ID:        int64(r.ID),
			Title:     r.SourceTitle,
			EventType: r.EventType,
			Date:      r.Date,
		}
	}

	return items, nil
}

func (s *SonarrAdapter) Capabilities() ManagerCapabilities {
	return ManagerCapabilities{
		SupportsRetry:       true,
		SupportsPriority:    false,
		SupportsHistory:     true,
		SupportsQualityEdit: false,
		SupportsBulkActions: true,
	}
}

func calculateProgress(total, remaining int64) float64 {
	if total == 0 {
		return 0
	}
	return float64(total-remaining) / float64(total) * 100
}
