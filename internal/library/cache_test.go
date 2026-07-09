package library

import (
	"testing"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/sonarr"
)

// MockSonarrClient implements basic Sonarr client methods for testing
type MockSonarrClient struct {
	series []sonarr.Series
	err    error
}

func (m *MockSonarrClient) GetAllSeries() ([]sonarr.Series, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.series, nil
}

func TestSeriesCache_FindSeries(t *testing.T) {
	mockClient := &MockSonarrClient{
		series: []sonarr.Series{
			{ID: 1, Title: "Fallout", Year: 2024, Path: "/mnt/STORAGE2/TVSHOWS/Fallout (2024)"},
			{ID: 2, Title: "For All Mankind", Year: 2019, Path: "/mnt/STORAGE5/TVSHOWS/For All Mankind (2019)"},
			{ID: 3, Title: "The White Lotus", Year: 2021, Path: "/mnt/STORAGE3/TVSHOWS/The White Lotus (2021)"},
			{ID: 4, Title: "Breaking Bad", Year: 2008, Path: "/mnt/STORAGE1/TVSHOWS/Breaking Bad (2008)"},
		},
	}

	cache := NewSeriesCache((*sonarr.Client)(nil), 5*time.Minute)
	// Replace client with mock
	cache.client = (*sonarr.Client)(nil)

	// Manually populate cache for testing (bypass actual API call)
	cache.mu.Lock()
	cache.series = make(map[string]*sonarr.Series)
	for i := range mockClient.series {
		s := &mockClient.series[i]
		key := normalizeTitle(s.Title)
		cache.series[key] = s
		if s.Year > 0 {
			keyWithYear := key + time.Now().Format("2006") // Use current year format
			cache.series[keyWithYear] = s
		}
	}
	cache.expiry = time.Now().Add(5 * time.Minute)
	cache.mu.Unlock()

	tests := []struct {
		showName  string
		year      string
		wantFound bool
		wantID    int
		wantPath  string
	}{
		{"Fallout", "2024", true, 1, "/mnt/STORAGE2/TVSHOWS/Fallout (2024)"},
		{"For All Mankind", "2019", true, 2, "/mnt/STORAGE5/TVSHOWS/For All Mankind (2019)"},
		{"The White Lotus", "2021", true, 3, "/mnt/STORAGE3/TVSHOWS/The White Lotus (2021)"},
		{"Breaking Bad", "2008", true, 4, "/mnt/STORAGE1/TVSHOWS/Breaking Bad (2008)"},
		{"Nonexistent Show", "2024", false, 0, ""},
		// Test case sensitivity
		{"FALLOUT", "2024", true, 1, "/mnt/STORAGE2/TVSHOWS/Fallout (2024)"},
		// Test without year
		{"Fallout", "", true, 1, "/mnt/STORAGE2/TVSHOWS/Fallout (2024)"},
	}

	for _, tt := range tests {
		t.Run(tt.showName, func(t *testing.T) {
			series := cache.FindSeries(tt.showName, tt.year)
			found := series != nil

			if found != tt.wantFound {
				t.Errorf("FindSeries(%q, %q) found=%v, want=%v", tt.showName, tt.year, found, tt.wantFound)
				return
			}

			if found {
				if series.ID != tt.wantID {
					t.Errorf("FindSeries(%q, %q) ID=%d, want=%d", tt.showName, tt.year, series.ID, tt.wantID)
				}
				if series.Path != tt.wantPath {
					t.Errorf("FindSeries(%q, %q) Path=%q, want=%q", tt.showName, tt.year, series.Path, tt.wantPath)
				}
			}
		})
	}
}

func TestSeriesCache_Expiry(t *testing.T) {
	mockClient := &MockSonarrClient{
		series: []sonarr.Series{
			{ID: 1, Title: "Test Show", Year: 2024, Path: "/mnt/STORAGE1/TVSHOWS/Test Show (2024)"},
		},
	}

	// Create cache with very short TTL
	cache := NewSeriesCache((*sonarr.Client)(nil), 100*time.Millisecond)
	cache.client = (*sonarr.Client)(nil)

	// Manually populate cache
	cache.mu.Lock()
	cache.series = make(map[string]*sonarr.Series)
	for i := range mockClient.series {
		s := &mockClient.series[i]
		key := normalizeTitle(s.Title)
		cache.series[key] = s
	}
	cache.expiry = time.Now().Add(100 * time.Millisecond)
	cache.mu.Unlock()

	// Should find series immediately
	series := cache.FindSeries("Test Show", "2024")
	if series == nil {
		t.Error("FindSeries should find series before expiry")
	}

	// Wait for expiry
	time.Sleep(150 * time.Millisecond)

	// Cache should still return stale data (since refresh will fail with nil client)
	series = cache.FindSeries("Test Show", "2024")
	if series == nil {
		t.Error("FindSeries should return stale data when refresh fails")
	}
}

func TestSeriesCache_ForceRefresh(t *testing.T) {
	cache := NewSeriesCache((*sonarr.Client)(nil), 5*time.Minute)

	// Manually set expiry to future
	cache.mu.Lock()
	cache.expiry = time.Now().Add(5 * time.Minute)
	initialExpiry := cache.expiry
	cache.mu.Unlock()

	// Force refresh should expire immediately
	cache.ForceRefresh()

	cache.mu.RLock()
	newExpiry := cache.expiry
	cache.mu.RUnlock()

	// Expiry should have been reset (will be zero time or new time depending on refresh outcome)
	if newExpiry == initialExpiry {
		// Give it a moment for the goroutine to start
		time.Sleep(50 * time.Millisecond)

		cache.mu.RLock()
		finalExpiry := cache.expiry
		cache.mu.RUnlock()

		if finalExpiry == initialExpiry {
			t.Error("ForceRefresh should update expiry time")
		}
	}
}
