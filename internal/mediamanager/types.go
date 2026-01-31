package mediamanager

import "time"

// ManagerType represents different media manager implementations
type ManagerType string

const (
	ManagerTypeSonarr          ManagerType = "sonarr"
	ManagerTypeRadarr          ManagerType = "radarr"
	ManagerTypeMediaDownloader ManagerType = "mediadownloader"
	ManagerTypeCustom          ManagerType = "custom"
)

// ServiceStatus represents the health and status of a media manager
type ServiceStatus struct {
	Online       bool      `json:"online"`
	Version      string    `json:"version"`
	QueueSize    int       `json:"queueSize"`
	StuckCount   int       `json:"stuckCount"`
	LastSyncedAt time.Time `json:"lastSyncedAt,omitempty"`
	Error        string    `json:"error,omitempty"`
}

// QueueItem represents a download queue item
type QueueItem struct {
	ID                    int64     `json:"id"`
	Title                 string    `json:"title"`
	Status                string    `json:"status"`
	Progress              float64   `json:"progress"`
	Size                  int64     `json:"size"`
	SizeRemaining         int64     `json:"sizeRemaining"`
	TimeLeft              string    `json:"timeLeft,omitempty"`
	EstimatedCompletionAt time.Time `json:"estimatedCompletionAt,omitempty"`
	IsStuck               bool      `json:"isStuck"`
	ErrorMessage          string    `json:"errorMessage,omitempty"`
	DownloadClient        string    `json:"downloadClient,omitempty"`
	Indexer               string    `json:"indexer,omitempty"`
}

// QueueFilter defines filtering options for queue queries
type QueueFilter struct {
	Status  string `json:"status,omitempty"`
	IsStuck *bool  `json:"isStuck,omitempty"`
	Limit   int    `json:"limit,omitempty"`
	Offset  int    `json:"offset,omitempty"`
}

// RootFolder represents a library root folder
type RootFolder struct {
	ID         int64  `json:"id"`
	Path       string `json:"path"`
	FreeSpace  int64  `json:"freeSpace"`
	TotalSpace int64  `json:"totalSpace,omitempty"`
}

// QualityProfile represents a quality profile
type QualityProfile struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// HistoryItem represents a completed/failed import
type HistoryItem struct {
	ID          int64     `json:"id"`
	Title       string    `json:"title"`
	EventType   string    `json:"eventType"`
	Date        time.Time `json:"date"`
	SourceTitle string    `json:"sourceTitle,omitempty"`
	Quality     string    `json:"quality,omitempty"`
	Success     bool      `json:"success"`
}

// HistoryOptions defines options for history queries
type HistoryOptions struct {
	Limit     int    `json:"limit,omitempty"`
	Offset    int    `json:"offset,omitempty"`
	EventType string `json:"eventType,omitempty"`
}

// ManagerCapabilities indicates what a manager supports
type ManagerCapabilities struct {
	SupportsRetry       bool `json:"supportsRetry"`
	SupportsPriority    bool `json:"supportsPriority"`
	SupportsHistory     bool `json:"supportsHistory"`
	SupportsQualityEdit bool `json:"supportsQualityEdit"`
	SupportsBulkActions bool `json:"supportsBulkActions"`
}

// ManagerInfo represents summary info about a media manager
type ManagerInfo struct {
	ID           string              `json:"id"`
	Type         ManagerType         `json:"type"`
	Name         string              `json:"name"`
	URL          string              `json:"url"`
	Enabled      bool                `json:"enabled"`
	Capabilities ManagerCapabilities `json:"capabilities"`
}
