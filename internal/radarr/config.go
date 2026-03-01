package radarr

import "fmt"

// DownloadClientConfig represents Radarr's download client configuration.
type DownloadClientConfig struct {
	ID                               int64 `json:"id"`
	EnableCompletedDownloadHandling  bool  `json:"enableCompletedDownloadHandling"`
	AutoRedownloadFailed             bool  `json:"autoRedownloadFailed"`
	CheckForFinishedDownloadInterval int64 `json:"checkForFinishedDownloadInterval"`
}

// MediaManagementConfig represents Radarr's media management settings
type MediaManagementConfig struct {
	ID                              int    `json:"id"`
	RecycleBin                      string `json:"recycleBin"`
	RecycleBinCleanupDays           int    `json:"recycleBinCleanupDays"`
	DownloadPropersAndRepacks       string `json:"downloadPropersAndRepacks"`
	CreateEmptyMovieFolders         bool   `json:"createEmptyMovieFolders"`
	DeleteEmptyFolders              bool   `json:"deleteEmptyFolders"`
	FileDate                        string `json:"fileDate"`
	RescanAfterRefreshType          string `json:"rescanAfterRefreshType"`
	SetPermissionsLinux             bool   `json:"setPermissionsLinux"`
	ChmodFolder                     string `json:"chmodFolder"`
	ChownGroup                      string `json:"chownGroup"`
	SkipFreeSpaceCheckWhenImporting bool   `json:"skipFreeSpaceCheckWhenImporting"`
	MinimumFreeSpaceWhenImporting   int    `json:"minimumFreeSpaceWhenImporting"`
	CopyUsingHardlinks              bool   `json:"copyUsingHardlinks"`
	UseScriptImport                 bool   `json:"useScriptImport"`
	ScriptImportPath                string `json:"scriptImportPath"`
	ImportExtraFiles                bool   `json:"importExtraFiles"`
	ExtraFileExtensions             string `json:"extraFileExtensions"`
}

// NamingConfig represents Radarr's naming configuration
type NamingConfig struct {
	ID                  int    `json:"id"`
	StandardMovieFormat string `json:"standardMovieFormat"`
	MovieFolderFormat   string `json:"movieFolderFormat"`
	ReleaseGroup        bool   `json:"releaseGroup"`
	SceneSource         bool   `json:"sceneSource"`
	MovieCleanup        bool   `json:"movieCleanup"`
	ColonReplacement    string `json:"colonReplacement"`
	SpaceReplacement    string `json:"spaceReplacement"`
	WindowsFiles        bool   `json:"windowsFiles"`
	RenameMovies        bool   `json:"renameMovies"`
	ReplaceIllegalChars bool   `json:"replaceIllegalChars"`
}

// RootFolder represents a Radarr root folder
type RootFolder struct {
	ID              int              `json:"id"`
	Path            string           `json:"path"`
	FreeSpace       int64            `json:"freeSpace"`
	TotalSpace      int64            `json:"totalSpace"`
	UnmappedFolders []UnmappedFolder `json:"unmappedFolders"`
}

// UnmappedFolder represents a folder not yet mapped to a movie
type UnmappedFolder struct {
	Name         string `json:"name"`
	Path         string `json:"path"`
	RelativePath string `json:"relativePath"`
}

// GetMediaManagementConfig retrieves Radarr's media management settings
func (c *Client) GetMediaManagementConfig() (*MediaManagementConfig, error) {
	var config MediaManagementConfig
	if err := c.get("/api/v3/config/mediamanagement", &config); err != nil {
		return nil, err
	}
	return &config, nil
}

// UpdateMediaManagementConfig updates Radarr's media management settings
func (c *Client) UpdateMediaManagementConfig(cfg *MediaManagementConfig) error {
	current, err := c.GetMediaManagementConfig()
	if err != nil {
		return err
	}

	cfg.ID = current.ID
	return c.put("/api/v3/config/mediamanagement", cfg, nil)
}

// GetNamingConfig retrieves Radarr's naming configuration
func (c *Client) GetNamingConfig() (*NamingConfig, error) {
	var config NamingConfig
	if err := c.get("/api/v3/config/naming", &config); err != nil {
		return nil, err
	}
	return &config, nil
}

// UpdateNamingConfig updates Radarr's naming configuration
func (c *Client) UpdateNamingConfig(cfg *NamingConfig) error {
	current, err := c.GetNamingConfig()
	if err != nil {
		return err
	}

	cfg.ID = current.ID
	return c.put("/api/v3/config/naming", cfg, nil)
}

// GetRootFolders retrieves all root folders from Radarr
func (c *Client) GetRootFolders() ([]RootFolder, error) {
	var folders []RootFolder
	if err := c.get("/api/v3/rootfolder", &folders); err != nil {
		return nil, err
	}
	return folders, nil
}

// DeleteRootFolder removes a root folder from Radarr by ID
func (c *Client) DeleteRootFolder(id int) error {
	return c.delete(fmt.Sprintf("/api/v3/rootfolder/%d", id))
}

// GetDownloadClientConfig retrieves download client configuration.
func (c *Client) GetDownloadClientConfig() (*DownloadClientConfig, error) {
	var cfg DownloadClientConfig
	if err := c.get("/api/v3/config/downloadClient", &cfg); err != nil {
		return nil, fmt.Errorf("getting download client config: %w", err)
	}
	return &cfg, nil
}

// UpdateDownloadClientConfig updates download client configuration.
func (c *Client) UpdateDownloadClientConfig(cfg DownloadClientConfig) (*DownloadClientConfig, error) {
	var result DownloadClientConfig
	if err := c.put(fmt.Sprintf("/api/v3/config/downloadClient/%d", cfg.ID), cfg, &result); err != nil {
		return nil, fmt.Errorf("updating download client config: %w", err)
	}
	return &result, nil
}
