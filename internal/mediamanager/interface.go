package mediamanager

import "context"

// MediaManager defines the interface for media manager integrations.
// Implementations wrap specific services (Sonarr, Radarr, etc.) behind
// a unified interface to allow the frontend to work with any backend.
type MediaManager interface {
	// Identity returns information about this manager instance
	ID() string
	Type() ManagerType
	Name() string
	Info() ManagerInfo

	// Health checks connectivity and returns current status
	Ping(ctx context.Context) error
	Status(ctx context.Context) (*ServiceStatus, error)

	// Queue Operations - view and manage download queue
	GetQueue(ctx context.Context, filter QueueFilter) ([]QueueItem, error)
	GetStuckItems(ctx context.Context) ([]QueueItem, error)
	GetQueueItem(ctx context.Context, id int64) (*QueueItem, error)
	ClearItem(ctx context.Context, id int64, blocklist bool) error
	ClearStuckItems(ctx context.Context, blocklist bool) (int, error)
	RetryItem(ctx context.Context, id int64) error

	// Sync Operations - trigger imports and syncs
	TriggerImportScan(ctx context.Context, path string) error
	ForceSync(ctx context.Context) error

	// Configuration - read-only access to manager config
	GetRootFolders(ctx context.Context) ([]RootFolder, error)
	GetQualityProfiles(ctx context.Context) ([]QualityProfile, error)

	// History - view past imports
	GetHistory(ctx context.Context, opts HistoryOptions) ([]HistoryItem, error)

	// Capabilities indicates what features this manager supports
	Capabilities() ManagerCapabilities
}

// Registry manages multiple MediaManager instances
type Registry struct {
	managers map[string]MediaManager
}

// NewRegistry creates a new manager registry
func NewRegistry() *Registry {
	return &Registry{
		managers: make(map[string]MediaManager),
	}
}

// Register adds a manager to the registry
func (r *Registry) Register(manager MediaManager) {
	r.managers[manager.ID()] = manager
}

// Get returns a manager by ID
func (r *Registry) Get(id string) (MediaManager, bool) {
	m, ok := r.managers[id]
	return m, ok
}

// All returns all registered managers
func (r *Registry) All() []MediaManager {
	result := make([]MediaManager, 0, len(r.managers))
	for _, m := range r.managers {
		result = append(result, m)
	}
	return result
}

// AllInfo returns info for all registered managers
func (r *Registry) AllInfo() []ManagerInfo {
	result := make([]ManagerInfo, 0, len(r.managers))
	for _, m := range r.managers {
		result = append(result, m.Info())
	}
	return result
}
