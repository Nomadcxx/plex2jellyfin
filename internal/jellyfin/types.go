package jellyfin

// SystemInfo from GET /System/Info.
type SystemInfo struct {
	ServerName      string `json:"ServerName"`
	Version         string `json:"Version"`
	ID              string `json:"Id"`
	OperatingSystem string `json:"OperatingSystem"`
}

// PublicSystemInfo from GET /System/Info/Public.
type PublicSystemInfo struct {
	ServerName   string `json:"ServerName"`
	Version      string `json:"Version"`
	ID           string `json:"Id"`
	LocalAddress string `json:"LocalAddress"`
}

// VirtualFolder from GET /Library/VirtualFolders.
type VirtualFolder struct {
	Name           string          `json:"Name"`
	Locations      []string        `json:"Locations"`
	CollectionType string          `json:"CollectionType"`
	ItemID         string          `json:"ItemId"`
	LibraryOptions *LibraryOptions `json:"LibraryOptions,omitempty"`
}

type LibraryOptions struct {
	EnableRealtimeMonitor         bool `json:"EnableRealtimeMonitor"`
	EnableAutomaticSeriesGrouping bool `json:"EnableAutomaticSeriesGrouping"`
}

// Item from GET /Items and GET /Items/{id}.
type Item struct {
	ID                string            `json:"Id"`
	Name              string            `json:"Name"`
	Path              string            `json:"Path"`
	Type              string            `json:"Type"`
	ProductionYear    int               `json:"ProductionYear"`
	ProviderIDs       map[string]string `json:"ProviderIds"`
	ParentID          string            `json:"ParentId"`
	SeriesID          string            `json:"SeriesId,omitempty"`
	SeriesName        string            `json:"SeriesName,omitempty"`
	SeasonName        string            `json:"SeasonName,omitempty"`
	IndexNumber       *int              `json:"IndexNumber,omitempty"`
	ParentIndexNumber *int              `json:"ParentIndexNumber,omitempty"`
}

// ItemsResponse from GET /Items.
type ItemsResponse struct {
	Items            []Item `json:"Items"`
	TotalRecordCount int    `json:"TotalRecordCount"`
}

// Session from GET /Sessions.
type Session struct {
	ID             string      `json:"Id"`
	UserName       string      `json:"UserName"`
	Client         string      `json:"Client"`
	DeviceName     string      `json:"DeviceName"`
	NowPlayingItem *NowPlaying `json:"NowPlayingItem,omitempty"`
	PlayState      *PlayState  `json:"PlayState,omitempty"`
}

type NowPlaying struct {
	ID   string `json:"Id"`
	Name string `json:"Name"`
	Path string `json:"Path"`
	Type string `json:"Type"`
}

type PlayState struct {
	IsPaused      bool   `json:"IsPaused"`
	PositionTicks *int64 `json:"PositionTicks,omitempty"`
}
