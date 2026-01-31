package mediamanager_test

import (
	"context"
	"testing"

	"github.com/Nomadcxx/jellywatch/internal/mediamanager"
)

// mockManager is a test implementation of MediaManager
type mockManager struct {
	id   string
	name string
}

func (m *mockManager) ID() string                     { return m.id }
func (m *mockManager) Type() mediamanager.ManagerType { return mediamanager.ManagerTypeSonarr }
func (m *mockManager) Name() string                   { return m.name }
func (m *mockManager) Info() mediamanager.ManagerInfo {
	return mediamanager.ManagerInfo{
		ID:      m.id,
		Type:    m.Type(),
		Name:    m.name,
		Enabled: true,
	}
}
func (m *mockManager) Ping(ctx context.Context) error { return nil }
func (m *mockManager) Status(ctx context.Context) (*mediamanager.ServiceStatus, error) {
	return &mediamanager.ServiceStatus{Online: true, Version: "1.0.0"}, nil
}
func (m *mockManager) GetQueue(ctx context.Context, filter mediamanager.QueueFilter) ([]mediamanager.QueueItem, error) {
	return nil, nil
}
func (m *mockManager) GetStuckItems(ctx context.Context) ([]mediamanager.QueueItem, error) {
	return nil, nil
}
func (m *mockManager) GetQueueItem(ctx context.Context, id int64) (*mediamanager.QueueItem, error) {
	return nil, nil
}
func (m *mockManager) ClearItem(ctx context.Context, id int64, blocklist bool) error { return nil }
func (m *mockManager) ClearStuckItems(ctx context.Context, blocklist bool) (int, error) {
	return 0, nil
}
func (m *mockManager) RetryItem(ctx context.Context, id int64) error            { return nil }
func (m *mockManager) TriggerImportScan(ctx context.Context, path string) error { return nil }
func (m *mockManager) ForceSync(ctx context.Context) error                      { return nil }
func (m *mockManager) GetRootFolders(ctx context.Context) ([]mediamanager.RootFolder, error) {
	return nil, nil
}
func (m *mockManager) GetQualityProfiles(ctx context.Context) ([]mediamanager.QualityProfile, error) {
	return nil, nil
}
func (m *mockManager) GetHistory(ctx context.Context, opts mediamanager.HistoryOptions) ([]mediamanager.HistoryItem, error) {
	return nil, nil
}
func (m *mockManager) Capabilities() mediamanager.ManagerCapabilities {
	return mediamanager.ManagerCapabilities{
		SupportsRetry:       true,
		SupportsBulkActions: true,
	}
}

func TestMediaManagerInterface(t *testing.T) {
	// Compile-time check that mockManager implements MediaManager
	var _ mediamanager.MediaManager = (*mockManager)(nil)
	t.Log("MediaManager interface compiles successfully")
}

func TestRegistry(t *testing.T) {
	registry := mediamanager.NewRegistry()

	mock1 := &mockManager{id: "sonarr-1", name: "Sonarr Main"}
	mock2 := &mockManager{id: "radarr-1", name: "Radarr Main"}

	registry.Register(mock1)
	registry.Register(mock2)

	// Test Get
	m, ok := registry.Get("sonarr-1")
	if !ok {
		t.Fatal("Expected to find sonarr-1")
	}
	if m.Name() != "Sonarr Main" {
		t.Errorf("Expected name 'Sonarr Main', got '%s'", m.Name())
	}

	// Test All
	all := registry.All()
	if len(all) != 2 {
		t.Errorf("Expected 2 managers, got %d", len(all))
	}

	// Test AllInfo
	info := registry.AllInfo()
	if len(info) != 2 {
		t.Errorf("Expected 2 info items, got %d", len(info))
	}
}
