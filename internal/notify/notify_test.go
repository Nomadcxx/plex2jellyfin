package notify

import (
	"testing"
	"time"
)

type mockNotifier struct {
	name        string
	enabled     bool
	pingErr     error
	notifyErr   error
	notifyCalls int
}

func (m *mockNotifier) Name() string  { return m.name }
func (m *mockNotifier) Enabled() bool { return m.enabled }
func (m *mockNotifier) Ping() error   { return m.pingErr }
func (m *mockNotifier) Notify(event OrganizationEvent) *NotifyResult {
	m.notifyCalls++
	return &NotifyResult{
		Service:   m.name,
		Success:   m.notifyErr == nil,
		CommandID: 123,
		Error:     m.notifyErr,
		Duration:  time.Millisecond,
	}
}

func TestManagerRegister(t *testing.T) {
	mgr := NewManager(false)
	defer mgr.Close()

	enabledNotifier := &mockNotifier{name: "enabled", enabled: true}
	disabledNotifier := &mockNotifier{name: "disabled", enabled: false}

	mgr.Register(enabledNotifier)
	mgr.Register(disabledNotifier)

	if mgr.NotifierCount() != 1 {
		t.Errorf("expected 1 notifier, got %d", mgr.NotifierCount())
	}
}

func TestManagerNotifySync(t *testing.T) {
	mgr := NewManager(false)
	defer mgr.Close()

	notifier := &mockNotifier{name: "test", enabled: true}
	mgr.Register(notifier)

	event := OrganizationEvent{
		MediaType:  MediaTypeMovie,
		SourcePath: "/src/movie.mkv",
		TargetPath: "/dst/Movie (2025)/Movie (2025).mkv",
	}

	results := mgr.Notify(event)

	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
	if !results[0].Success {
		t.Error("expected success")
	}
	if notifier.notifyCalls != 1 {
		t.Errorf("expected 1 notify call, got %d", notifier.notifyCalls)
	}
}

func TestManagerNotifyAsync(t *testing.T) {
	mgr := NewManager(true)
	defer mgr.Close()

	notifier := &mockNotifier{name: "async", enabled: true}
	mgr.Register(notifier)

	event := OrganizationEvent{
		MediaType:  MediaTypeTVEpisode,
		SourcePath: "/src/show.mkv",
		TargetPath: "/dst/Show/Season 01/Show S01E01.mkv",
	}

	results := mgr.Notify(event)
	if results != nil {
		t.Error("async notify should return nil")
	}

	select {
	case result := <-mgr.Results():
		if !result.Success {
			t.Fatalf("async notification failed: %v", result.Error)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for async notification")
	}

	if notifier.notifyCalls != 1 {
		t.Errorf("expected 1 notify call, got %d", notifier.notifyCalls)
	}
}

func TestMediaTypeString(t *testing.T) {
	tests := []struct {
		mediaType MediaType
		expected  string
	}{
		{MediaTypeMovie, "movie"},
		{MediaTypeTVEpisode, "tv"},
		{MediaType(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.mediaType.String(); got != tt.expected {
			t.Errorf("MediaType(%d).String() = %s, want %s", tt.mediaType, got, tt.expected)
		}
	}
}

func TestFormatEventSummary(t *testing.T) {
	tests := []struct {
		event    OrganizationEvent
		expected string
	}{
		{
			OrganizationEvent{MediaType: MediaTypeMovie, Title: "Inception", Year: "2010"},
			"Movie: Inception (2010)",
		},
		{
			OrganizationEvent{MediaType: MediaTypeTVEpisode, Title: "Breaking Bad", Season: 1, Episode: 5},
			"TV: Breaking Bad S01E05",
		},
	}

	for _, tt := range tests {
		if got := FormatEventSummary(tt.event); got != tt.expected {
			t.Errorf("FormatEventSummary() = %s, want %s", got, tt.expected)
		}
	}
}

func TestManagerPingAll(t *testing.T) {
	mgr := NewManager(false)
	defer mgr.Close()

	mgr.Register(&mockNotifier{name: "healthy", enabled: true, pingErr: nil})

	results := mgr.PingAll()

	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
	if results["healthy"] != nil {
		t.Error("expected nil error for healthy notifier")
	}
}
