package jellyfin

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
