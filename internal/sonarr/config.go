package sonarr

import "fmt"

type MediaManagementConfig struct {
	ID                                        int    `json:"id"`
	AutoUnmonitorPreviouslyDownloadedEpisodes bool   `json:"autoUnmonitorPreviouslyDownloadedEpisodes"`
	RecycleBin                                string `json:"recycleBin"`
	RecycleBinCleanupDays                     int    `json:"recycleBinCleanupDays"`
	DownloadPropersAndRepacks                 string `json:"downloadPropersAndRepacks"`
	CreateEmptySeriesFolders                  bool   `json:"createEmptySeriesFolders"`
	DeleteEmptyFolders                        bool   `json:"deleteEmptyFolders"`
	FileDate                                  string `json:"fileDate"`
	RescanAfterRefreshType                    string `json:"rescanAfterRefreshType"`
	SetPermissionsLinux                       bool   `json:"setPermissionsLinux"`
	ChmodFolder                               string `json:"chmodFolder"`
	ChownGroup                                string `json:"chownGroup"`
	EpisodeTitleRequiredType                  string `json:"episodeTitleRequiredType"`
	SkipFreeSpaceCheckWhenImporting           bool   `json:"skipFreeSpaceCheckWhenImporting"`
	MinimumFreeSpaceWhenImporting             int    `json:"minimumFreeSpaceWhenImporting"`
	CopyUsingHardlinks                        bool   `json:"copyUsingHardlinks"`
	UseScriptImport                           bool   `json:"useScriptImport"`
	ScriptImportPath                          string `json:"scriptImportPath"`
	ImportExtraFiles                          bool   `json:"importExtraFiles"`
	ExtraFileExtensions                       string `json:"extraFileExtensions"`
}

// NamingConfig represents Sonarr's naming configuration
type NamingConfig struct {
	ID                    int    `json:"id"`
	SeasonFolderFormat    string `json:"seasonFolderFormat"`
	SeriesFolderFormat    string `json:"seriesFolderFormat"`
	SeasonFolderPattern   string `json:"seasonFolderPattern"`
	StandardEpisodeFormat string `json:"standardEpisodeFormat"`
	DailyEpisodeFormat    string `json:"dailyEpisodeFormat"`
	AnimeEpisodeFormat    string `json:"animeEpisodeFormat"`
	ReleaseGroup          bool   `json:"releaseGroup"`
	SceneSource           bool   `json:"sceneSource"`
	SeriesCleanup         bool   `json:"seriesCleanup"`
	EpisodeCleanup        bool   `json:"episodeCleanup"`
	ColonReplacement      string `json:"colonReplacement"`
	SpaceReplacement      string `json:"spaceReplacement"`
	WindowsFiles          bool   `json:"windowsFiles"`
	RenameEpisodes        bool   `json:"renameEpisodes"`
	ReplaceIllegalChars   bool   `json:"replaceIllegalChars"`
	MultiEpisodeStyle     int    `json:"multiEpisodeStyle"`
}

// RootFolder represents a Sonarr root folder
type RootFolder struct {
	ID              int              `json:"id"`
	Path            string           `json:"path"`
	FreeSpace       int64            `json:"freeSpace"`
	TotalSpace      int64            `json:"totalSpace"`
	UnmappedFolders []UnmappedFolder `json:"unmappedFolders"`
}

// UnmappedFolder represents a folder not yet mapped to a series
type UnmappedFolder struct {
	Name         string `json:"name"`
	Path         string `json:"path"`
	RelativePath string `json:"relativePath"`
}

// GetMediaManagementConfig retrieves Sonarr's media management settings
func (c *Client) GetMediaManagementConfig() (*MediaManagementConfig, error) {
	var config MediaManagementConfig
	if err := c.get("/api/v3/config/mediamanagement", &config); err != nil {
		return nil, err
	}
	return &config, nil
}

// UpdateMediaManagementConfig updates Sonarr's media management settings
func (c *Client) UpdateMediaManagementConfig(cfg *MediaManagementConfig) error {
	current, err := c.GetMediaManagementConfig()
	if err != nil {
		return err
	}

	cfg.ID = current.ID
	return c.put("/api/v3/config/mediamanagement", cfg, nil)
}

// GetNamingConfig retrieves Sonarr's naming configuration
func (c *Client) GetNamingConfig() (*NamingConfig, error) {
	var config NamingConfig
	if err := c.get("/api/v3/config/naming", &config); err != nil {
		return nil, err
	}
	return &config, nil
}

// UpdateNamingConfig updates Sonarr's naming configuration
func (c *Client) UpdateNamingConfig(cfg *NamingConfig) error {
	current, err := c.GetNamingConfig()
	if err != nil {
		return err
	}

	cfg.ID = current.ID
	return c.put("/api/v3/config/naming", cfg, nil)
}

// GetRootFolders retrieves all root folders from Sonarr
func (c *Client) GetRootFolders() ([]RootFolder, error) {
	var folders []RootFolder
	if err := c.get("/api/v3/rootfolder", &folders); err != nil {
		return nil, err
	}
	return folders, nil
}

// DeleteRootFolder removes a root folder from Sonarr by ID
func (c *Client) DeleteRootFolder(id int) error {
	return c.delete(fmt.Sprintf("/api/v3/rootfolder/%d", id))
}
