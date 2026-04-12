// Package notify provides a unified interface for post-organization notifications
// to media management systems like Sonarr, Radarr, and Jellyfin.
package notify

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// MediaType represents the type of media that was organized
type MediaType int

const (
	MediaTypeMovie MediaType = iota
	MediaTypeTVEpisode
)

func (m MediaType) String() string {
	switch m {
	case MediaTypeMovie:
		return "movie"
	case MediaTypeTVEpisode:
		return "tv"
	default:
		return "unknown"
	}
}

// OrganizationEvent contains information about a successfully organized file
type OrganizationEvent struct {
	MediaType   MediaType
	SourcePath  string
	TargetPath  string
	TargetDir   string
	Title       string
	Year        string
	Season      int
	Episode     int
	BytesCopied int64
	Duration    time.Duration
}

// NotifyResult represents the result of a notification attempt
type NotifyResult struct {
	Service   string
	Success   bool
	Skipped   bool // True when notifier was not applicable (e.g. Radarr for TV content)
	CommandID int
	Error     error
	Duration  time.Duration
}

// Notifier is the interface that notification providers must implement
type Notifier interface {
	// Name returns the name of the notification service
	Name() string

	// Notify sends a notification about the organization event
	Notify(event OrganizationEvent) *NotifyResult

	// Ping checks if the service is reachable
	Ping() error

	// Enabled returns whether this notifier is enabled
	Enabled() bool
}

// Manager handles multiple notification providers
type Manager struct {
	notifiers []Notifier
	mu        sync.RWMutex
	async     bool
	results   chan *NotifyResult
}

// NewManager creates a new notification manager
func NewManager(async bool) *Manager {
	m := &Manager{
		notifiers: make([]Notifier, 0),
		async:     async,
	}
	if async {
		m.results = make(chan *NotifyResult, 100)
	}
	return m
}

// Register adds a notifier to the manager
func (m *Manager) Register(n Notifier) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if n.Enabled() {
		m.notifiers = append(m.notifiers, n)
		log.Printf("Registered notifier: %s", n.Name())
	}
}

// Notify sends notifications to all registered providers
func (m *Manager) Notify(event OrganizationEvent) []*NotifyResult {
	m.mu.RLock()
	notifiers := make([]Notifier, len(m.notifiers))
	copy(notifiers, m.notifiers)
	m.mu.RUnlock()

	if len(notifiers) == 0 {
		return nil
	}

	if m.async {
		go m.notifyAsync(notifiers, event)
		return nil
	}

	return m.notifySync(notifiers, event)
}

func (m *Manager) notifySync(notifiers []Notifier, event OrganizationEvent) []*NotifyResult {
	results := make([]*NotifyResult, 0, len(notifiers))

	for _, n := range notifiers {
		result := n.Notify(event)
		results = append(results, result)

		if result.Skipped {
			log.Printf("[%s] Notification skipped (wrong media type)", n.Name())
		} else if result.Success {
			log.Printf("[%s] Notification sent successfully (command ID: %d)", n.Name(), result.CommandID)
		} else {
			log.Printf("[%s] Notification failed: %v", n.Name(), result.Error)
		}
	}

	return results
}

func (m *Manager) notifyAsync(notifiers []Notifier, event OrganizationEvent) {
	var wg sync.WaitGroup

	for _, n := range notifiers {
		wg.Add(1)
		go func(notifier Notifier) {
			defer wg.Done()

			result := notifier.Notify(event)
			if m.results != nil {
				select {
				case m.results <- result:
				default:
					// Channel full, log and discard
					log.Printf("[%s] Result channel full, discarding", notifier.Name())
				}
			}

			if result.Skipped {
				log.Printf("[%s] Notification skipped (wrong media type)", notifier.Name())
			} else if result.Success {
				log.Printf("[%s] Notification sent successfully (command ID: %d)", notifier.Name(), result.CommandID)
			} else {
				log.Printf("[%s] Notification failed: %v", notifier.Name(), result.Error)
			}
		}(n)
	}

	wg.Wait()
}

// Results returns the async results channel (nil if sync mode)
func (m *Manager) Results() <-chan *NotifyResult {
	return m.results
}

// PingAll checks connectivity to all registered notifiers
func (m *Manager) PingAll() map[string]error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	results := make(map[string]error)
	for _, n := range m.notifiers {
		results[n.Name()] = n.Ping()
	}
	return results
}

// NotifierCount returns the number of registered notifiers
func (m *Manager) NotifierCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.notifiers)
}

// Close cleans up the manager
func (m *Manager) Close() {
	if m.results != nil {
		close(m.results)
	}
}

// FormatEventSummary returns a human-readable summary of the event
func FormatEventSummary(event OrganizationEvent) string {
	switch event.MediaType {
	case MediaTypeMovie:
		return fmt.Sprintf("Movie: %s (%s)", event.Title, event.Year)
	case MediaTypeTVEpisode:
		return fmt.Sprintf("TV: %s S%02dE%02d", event.Title, event.Season, event.Episode)
	default:
		return fmt.Sprintf("Unknown: %s", event.Title)
	}
}
