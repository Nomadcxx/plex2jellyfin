package mediamanager

import (
	"context"
	"fmt"

	"github.com/Nomadcxx/jellywatch/internal/radarr"
)

// RadarrAdapter wraps the Radarr client to implement MediaManager
type RadarrAdapter struct {
	id     string
	name   string
	client *radarr.Client
}

// NewRadarrAdapter creates a new Radarr adapter
func NewRadarrAdapter(id, name string, client *radarr.Client) *RadarrAdapter {
	return &RadarrAdapter{
		id:     id,
		name:   name,
		client: client,
	}
}

func (r *RadarrAdapter) ID() string        { return r.id }
func (r *RadarrAdapter) Type() ManagerType { return ManagerTypeRadarr }
func (r *RadarrAdapter) Name() string      { return r.name }

func (r *RadarrAdapter) Info() ManagerInfo {
	return ManagerInfo{
		ID:           r.id,
		Type:         r.Type(),
		Name:         r.name,
		Enabled:      true,
		Capabilities: r.Capabilities(),
	}
}

func (r *RadarrAdapter) Ping(ctx context.Context) error {
	return r.client.Ping()
}

func (r *RadarrAdapter) Status(ctx context.Context) (*ServiceStatus, error) {
	status := &ServiceStatus{}

	sysStatus, err := r.client.GetSystemStatus()
	if err != nil {
		status.Online = false
		status.Error = err.Error()
		return status, nil
	}

	status.Online = true
	status.Version = sysStatus.Version

	queue, err := r.client.GetQueue(1, 1)
	if err == nil {
		status.QueueSize = queue.TotalRecords
	}

	stuck, err := r.client.GetStuckItems()
	if err == nil {
		status.StuckCount = len(stuck)
	}

	return status, nil
}

func (r *RadarrAdapter) GetQueue(ctx context.Context, filter QueueFilter) ([]QueueItem, error) {
	limit := filter.Limit
	if limit == 0 {
		limit = 50
	}

	queue, err := r.client.GetQueue(1, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get Radarr queue: %w", err)
	}

	items := make([]QueueItem, 0, len(queue.Records))
	for _, rec := range queue.Records {
		isStuck := rec.TrackedDownloadStatus == "warning" || rec.TrackedDownloadStatus == "error"

		if filter.IsStuck != nil && *filter.IsStuck != isStuck {
			continue
		}

		item := QueueItem{
			ID:             int64(rec.ID),
			Title:          rec.Title,
			Status:         rec.Status,
			Progress:       calculateProgress(rec.Size, rec.Sizeleft),
			Size:           rec.Size,
			SizeRemaining:  rec.Sizeleft,
			TimeLeft:       rec.Timeleft,
			IsStuck:        isStuck,
			DownloadClient: rec.DownloadClient,
		}

		if len(rec.StatusMessages) > 0 {
			item.ErrorMessage = rec.StatusMessages[0].Title
		}

		items = append(items, item)
	}

	return items, nil
}

func (r *RadarrAdapter) GetStuckItems(ctx context.Context) ([]QueueItem, error) {
	stuck, err := r.client.GetStuckItems()
	if err != nil {
		return nil, fmt.Errorf("failed to get stuck items: %w", err)
	}

	items := make([]QueueItem, len(stuck))
	for i, rec := range stuck {
		items[i] = QueueItem{
			ID:      int64(rec.ID),
			Title:   rec.Title,
			Status:  rec.Status,
			IsStuck: true,
		}
		if len(rec.StatusMessages) > 0 {
			items[i].ErrorMessage = rec.StatusMessages[0].Title
		}
	}

	return items, nil
}

func (r *RadarrAdapter) GetQueueItem(ctx context.Context, id int64) (*QueueItem, error) {
	item, err := r.client.GetQueueItem(int(id))
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

func (r *RadarrAdapter) ClearItem(ctx context.Context, id int64, blocklist bool) error {
	return r.client.RemoveFromQueue(int(id), blocklist, false)
}

func (r *RadarrAdapter) ClearStuckItems(ctx context.Context, blocklist bool) (int, error) {
	return r.client.ClearStuckItems(blocklist)
}

func (r *RadarrAdapter) RetryItem(ctx context.Context, id int64) error {
	return r.client.GrabQueueItem(int(id))
}

func (r *RadarrAdapter) TriggerImportScan(ctx context.Context, path string) error {
	_, err := r.client.TriggerDownloadedMoviesScan(path)
	return err
}

func (r *RadarrAdapter) ForceSync(ctx context.Context) error {
	_, err := r.client.RefreshAllMovies()
	return err
}

func (r *RadarrAdapter) GetRootFolders(ctx context.Context) ([]RootFolder, error) {
	folders, err := r.client.GetRootFolders()
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

func (r *RadarrAdapter) GetQualityProfiles(ctx context.Context) ([]QualityProfile, error) {
	return nil, nil
}

func (r *RadarrAdapter) GetHistory(ctx context.Context, opts HistoryOptions) ([]HistoryItem, error) {
	return nil, nil
}

func (r *RadarrAdapter) Capabilities() ManagerCapabilities {
	return ManagerCapabilities{
		SupportsRetry:       true,
		SupportsPriority:    false,
		SupportsHistory:     false,
		SupportsQualityEdit: false,
		SupportsBulkActions: true,
	}
}
