package notify

import (
	"time"

	"github.com/Nomadcxx/jellywatch/internal/jellyfin"
)

type JellyfinNotifier struct {
	client  *jellyfin.Client
	enabled bool
}

func NewJellyfinNotifier(client *jellyfin.Client, enabled bool) *JellyfinNotifier {
	return &JellyfinNotifier{
		client:  client,
		enabled: enabled && client != nil,
	}
}

func (n *JellyfinNotifier) Name() string {
	return "jellyfin"
}

func (n *JellyfinNotifier) Enabled() bool {
	return n.enabled
}

func (n *JellyfinNotifier) Ping() error {
	if !n.enabled {
		return nil
	}
	return n.client.Ping()
}

func (n *JellyfinNotifier) Notify(event OrganizationEvent) *NotifyResult {
	start := time.Now()
	result := &NotifyResult{Service: n.Name()}

	if !n.enabled {
		result.Success = true
		result.Duration = time.Since(start)
		return result
	}

	err := n.client.RefreshLibrary()
	result.Success = err == nil
	result.Error = err
	result.Duration = time.Since(start)
	return result
}
