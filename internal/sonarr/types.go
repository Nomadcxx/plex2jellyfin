package sonarr

import "time"

// QueueItem represents an item in Sonarr's download queue
type QueueItem struct {
	ID                      int             `json:"id"`
	SeriesID                int             `json:"seriesId"`
	EpisodeID               int             `json:"episodeId"`
	SeasonNumber            int             `json:"seasonNumber"`
	Title                   string          `json:"title"`
	Status                  string          `json:"status"`
	TrackedDownloadStatus   string          `json:"trackedDownloadStatus"`
	TrackedDownloadState    string          `json:"trackedDownloadState"`
	StatusMessages          []StatusMessage `json:"statusMessages"`
	DownloadID              string          `json:"downloadId"`
	Protocol                string          `json:"protocol"`
	DownloadClient          string          `json:"downloadClient"`
	OutputPath              string          `json:"outputPath"`
	ErrorMessage            string          `json:"errorMessage"`
	Size                    int64           `json:"size"`
	Sizeleft                int64           `json:"sizeleft"`
	Timeleft                string          `json:"timeleft"`
	EstimatedCompletionTime *time.Time      `json:"estimatedCompletionTime"`
	Added                   time.Time       `json:"added"`
}

// StatusMessage contains error/warning info for queue items
type StatusMessage struct {
	Title    string   `json:"title"`
	Messages []string `json:"messages"`
}

// QueueResponse is the paginated queue response
type QueueResponse struct {
	Page          int         `json:"page"`
	PageSize      int         `json:"pageSize"`
	SortKey       string      `json:"sortKey"`
	SortDirection string      `json:"sortDirection"`
	TotalRecords  int         `json:"totalRecords"`
	Records       []QueueItem `json:"records"`
}

// Series represents a TV series in Sonarr
type Series struct {
	ID               int               `json:"id"`
	Title            string            `json:"title"`
	TitleSlug        string            `json:"titleSlug"`
	SortTitle        string            `json:"sortTitle"`
	Path             string            `json:"path"`
	Year             int               `json:"year"`
	SeasonCount      int               `json:"seasonCount"`
	TvdbID           int               `json:"tvdbId"`
	TvRageID         int               `json:"tvRageId"`
	TvMazeID         int               `json:"tvMazeId"`
	ImdbID           string            `json:"imdbId"`
	Overview         string            `json:"overview"`
	Status           string            `json:"status"`
	SeriesType       string            `json:"seriesType"`
	Network          string            `json:"network"`
	Runtime          int               `json:"runtime"`
	QualityProfileID int               `json:"qualityProfileId"`
	Monitored        bool              `json:"monitored"`
	SeasonFolder     bool              `json:"seasonFolder"`
	Added            time.Time         `json:"added"`
	FirstAired       string            `json:"firstAired"`
	Statistics       *SeriesStatistics `json:"statistics,omitempty"`
}

// SeriesStatistics contains episode/file statistics for a series
type SeriesStatistics struct {
	SeasonCount       int     `json:"seasonCount"`
	EpisodeFileCount  int     `json:"episodeFileCount"`
	EpisodeCount      int     `json:"episodeCount"`
	TotalEpisodeCount int     `json:"totalEpisodeCount"`
	SizeOnDisk        int64   `json:"sizeOnDisk"`
	PercentOfEpisodes float64 `json:"percentOfEpisodes"`
}

// Episode represents a TV episode
type Episode struct {
	ID                    int          `json:"id"`
	SeriesID              int          `json:"seriesId"`
	TvdbID                int          `json:"tvdbId"`
	SeasonNumber          int          `json:"seasonNumber"`
	EpisodeNumber         int          `json:"episodeNumber"`
	Title                 string       `json:"title"`
	Overview              string       `json:"overview"`
	AirDate               string       `json:"airDate"`
	AirDateUtc            *time.Time   `json:"airDateUtc"`
	HasFile               bool         `json:"hasFile"`
	Monitored             bool         `json:"monitored"`
	EpisodeFileID         int          `json:"episodeFileId"`
	AbsoluteEpisodeNumber *int         `json:"absoluteEpisodeNumber"`
	SceneSeasonNumber     *int         `json:"sceneSeasonNumber"`
	SceneEpisodeNumber    *int         `json:"sceneEpisodeNumber"`
	Runtime               int          `json:"runtime"`
	Series                *Series      `json:"series,omitempty"`
	EpisodeFile           *EpisodeFile `json:"episodeFile,omitempty"`
}

// EpisodeFile represents a downloaded episode file
type EpisodeFile struct {
	ID           int        `json:"id"`
	SeriesID     int        `json:"seriesId"`
	SeasonNumber int        `json:"seasonNumber"`
	RelativePath string     `json:"relativePath"`
	Path         string     `json:"path"`
	Size         int64      `json:"size"`
	DateAdded    time.Time  `json:"dateAdded"`
	SceneName    string     `json:"sceneName"`
	ReleaseGroup string     `json:"releaseGroup"`
	Quality      Quality    `json:"quality"`
	MediaInfo    *MediaInfo `json:"mediaInfo,omitempty"`
}

// Quality represents video quality information
type Quality struct {
	Quality  QualityInfo `json:"quality"`
	Revision Revision    `json:"revision"`
}

// QualityInfo contains quality details
type QualityInfo struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Source     string `json:"source"`
	Resolution int    `json:"resolution"`
}

// Revision contains version info for a quality
type Revision struct {
	Version  int  `json:"version"`
	Real     int  `json:"real"`
	IsRepack bool `json:"isRepack"`
}

// MediaInfo contains technical media information
type MediaInfo struct {
	AudioBitrate     int     `json:"audioBitrate"`
	AudioChannels    float64 `json:"audioChannels"`
	AudioCodec       string  `json:"audioCodec"`
	AudioLanguages   string  `json:"audioLanguages"`
	AudioStreamCount int     `json:"audioStreamCount"`
	VideoBitDepth    int     `json:"videoBitDepth"`
	VideoBitrate     int     `json:"videoBitrate"`
	VideoCodec       string  `json:"videoCodec"`
	VideoFps         float64 `json:"videoFps"`
	Resolution       string  `json:"resolution"`
	RunTime          string  `json:"runTime"`
	ScanType         string  `json:"scanType"`
	Subtitles        string  `json:"subtitles"`
}

// Command represents a Sonarr command request
type Command struct {
	Name         string `json:"name"`
	SeriesID     int    `json:"seriesId,omitempty"`
	SeasonNumber int    `json:"seasonNumber,omitempty"`
	SeriesIDs    []int  `json:"seriesIds,omitempty"`
	EpisodeIDs   []int  `json:"episodeIds,omitempty"`
	Files        []int  `json:"files,omitempty"`
	Path         string `json:"path,omitempty"`
	ImportMode   string `json:"importMode,omitempty"`
}

// CommandResponse is returned after executing a command
type CommandResponse struct {
	ID          int        `json:"id"`
	Name        string     `json:"name"`
	CommandName string     `json:"commandName"`
	Message     string     `json:"message"`
	Status      string     `json:"status"`
	Queued      time.Time  `json:"queued"`
	Started     *time.Time `json:"started"`
	Ended       *time.Time `json:"ended"`
	StateChange *time.Time `json:"stateChangeTime"`
	Duration    string     `json:"duration"`
	Trigger     string     `json:"trigger"`
	Priority    string     `json:"priority"`
}

// HistoryRecord represents a history entry
type HistoryRecord struct {
	ID                  int                    `json:"id"`
	EpisodeID           int                    `json:"episodeId"`
	SeriesID            int                    `json:"seriesId"`
	SourceTitle         string                 `json:"sourceTitle"`
	Quality             Quality                `json:"quality"`
	QualityCutoffNotMet bool                   `json:"qualityCutoffNotMet"`
	Date                time.Time              `json:"date"`
	DownloadID          string                 `json:"downloadId"`
	EventType           string                 `json:"eventType"`
	Data                map[string]interface{} `json:"data"`
	Series              *Series                `json:"series,omitempty"`
	Episode             *Episode               `json:"episode,omitempty"`
}

// HistoryResponse is the paginated history response
type HistoryResponse struct {
	Page          int             `json:"page"`
	PageSize      int             `json:"pageSize"`
	SortKey       string          `json:"sortKey"`
	SortDirection string          `json:"sortDirection"`
	TotalRecords  int             `json:"totalRecords"`
	Records       []HistoryRecord `json:"records"`
}

// SystemStatus represents Sonarr system status
type SystemStatus struct {
	AppName                string `json:"appName"`
	InstanceName           string `json:"instanceName"`
	Version                string `json:"version"`
	BuildTime              string `json:"buildTime"`
	IsDebug                bool   `json:"isDebug"`
	IsProduction           bool   `json:"isProduction"`
	IsAdmin                bool   `json:"isAdmin"`
	IsUserInteractive      bool   `json:"isUserInteractive"`
	StartupPath            string `json:"startupPath"`
	AppData                string `json:"appData"`
	OsName                 string `json:"osName"`
	OsVersion              string `json:"osVersion"`
	IsNetCore              bool   `json:"isNetCore"`
	IsLinux                bool   `json:"isLinux"`
	IsOsx                  bool   `json:"isOsx"`
	IsWindows              bool   `json:"isWindows"`
	IsDocker               bool   `json:"isDocker"`
	Branch                 string `json:"branch"`
	DatabaseType           string `json:"databaseType"`
	DatabaseVersion        string `json:"databaseVersion"`
	Authentication         string `json:"authentication"`
	MigrationVersion       int    `json:"migrationVersion"`
	UrlBase                string `json:"urlBase"`
	RuntimeVersion         string `json:"runtimeVersion"`
	RuntimeName            string `json:"runtimeName"`
	StartTime              string `json:"startTime"`
	PackageVersion         string `json:"packageVersion"`
	PackageAuthor          string `json:"packageAuthor"`
	PackageUpdateMechanism string `json:"packageUpdateMechanism"`
}

// BulkQueueRequest is used for bulk queue operations
type BulkQueueRequest struct {
	IDs              []int `json:"ids"`
	RemoveFromClient bool  `json:"removeFromClient,omitempty"`
	Blocklist        bool  `json:"blocklist,omitempty"`
}
