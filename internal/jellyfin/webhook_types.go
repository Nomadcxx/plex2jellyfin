package jellyfin

import "encoding/json"

// WebhookEvent is the normalized payload received from Jellyfin Webhook plugin.
type WebhookEvent struct {
	NotificationType string `json:"NotificationType"`

	ServerName string `json:"ServerName,omitempty"`
	ServerID   string `json:"ServerId,omitempty"`

	ItemID   string `json:"ItemId,omitempty"`
	ItemName string `json:"Name,omitempty"`
	ItemType string `json:"ItemType,omitempty"`
	ItemPath string `json:"ItemPath,omitempty"`
	Year     int    `json:"Year,omitempty"`

	SeriesName    string `json:"SeriesName,omitempty"`
	SeasonNumber  *int   `json:"SeasonNumber,omitempty"`
	EpisodeNumber *int   `json:"EpisodeNumber,omitempty"`

	ProviderTmdb string `json:"Provider_tmdb,omitempty"`
	ProviderTvdb string `json:"Provider_tvdb,omitempty"`
	ProviderImdb string `json:"Provider_imdb,omitempty"`

	UserName   string `json:"NotificationUsername,omitempty"`
	DeviceName string `json:"DeviceName,omitempty"`
	ClientName string `json:"ClientName,omitempty"`

	TaskName string `json:"TaskName,omitempty"`

	Timestamp string `json:"Timestamp,omitempty"`
}

const (
	EventItemAdded      = "ItemAdded"
	EventPlaybackStart  = "PlaybackStart"
	EventPlaybackStop   = "PlaybackStop"
	EventLibraryChanged = "LibraryChanged"
	EventTaskCompleted  = "TaskCompleted"
)

// UnmarshalJSON normalizes both the flat Jellyfin Webhook plugin format and the
// nested companion plugin format ({"eventType":"...","payload":{"item":{...}}})
// into a single WebhookEvent representation.
func (e *WebhookEvent) UnmarshalJSON(data []byte) error {
type Alias WebhookEvent
flat := (*Alias)(e)
if err := json.Unmarshal(data, flat); err != nil {
return err
}

if e.NotificationType != "" {
return nil
}

var nested struct {
EventType string `json:"eventType"`
Timestamp string `json:"timestamp"`
Payload   struct {
Item struct {
ID          string            `json:"id"`
Path        string            `json:"path"`
Name        string            `json:"name"`
Type        string            `json:"type"`
ProviderIDs map[string]string `json:"providerIds"`
} `json:"item"`
} `json:"payload"`
}
if err := json.Unmarshal(data, &nested); err != nil {
return err
}

if nested.EventType != "" {
e.NotificationType = nested.EventType
if e.Timestamp == "" {
e.Timestamp = nested.Timestamp
}
if e.ItemID == "" {
e.ItemID = nested.Payload.Item.ID
}
if e.ItemPath == "" {
e.ItemPath = nested.Payload.Item.Path
}
if e.ItemName == "" {
e.ItemName = nested.Payload.Item.Name
}
if e.ItemType == "" {
e.ItemType = nested.Payload.Item.Type
}
if e.ProviderImdb == "" {
e.ProviderImdb = nested.Payload.Item.ProviderIDs["Imdb"]
}
if e.ProviderTmdb == "" {
e.ProviderTmdb = nested.Payload.Item.ProviderIDs["Tmdb"]
}
if e.ProviderTvdb == "" {
e.ProviderTvdb = nested.Payload.Item.ProviderIDs["Tvdb"]
}
}

return nil
}
