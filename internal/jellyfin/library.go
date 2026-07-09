package jellyfin

import "fmt"

// RefreshLibrary triggers a full library scan.
func (c *Client) RefreshLibrary() error {
	if err := c.post("/Library/Refresh", nil, nil); err != nil {
		return fmt.Errorf("refreshing library: %w", err)
	}
	return nil
}

// RefreshItem triggers a metadata refresh for a specific item.
func (c *Client) RefreshItem(itemID string) error {
	payload := map[string]interface{}{
		"Recursive":          true,
		"ReplaceAllMetadata": false,
		"ReplaceAllImages":   false,
	}
	if err := c.post("/Items/"+itemID+"/Refresh", payload, nil); err != nil {
		return fmt.Errorf("refreshing item %s: %w", itemID, err)
	}
	return nil
}

// RefreshItemFullMetadata triggers a full metadata and image refresh for a
// specific item.
func (c *Client) RefreshItemFullMetadata(itemID string) error {
	query := "?Recursive=false&MetadataRefreshMode=FullRefresh&ImageRefreshMode=FullRefresh&ReplaceAllMetadata=true&ReplaceAllImages=true"
	if err := c.post("/Items/"+itemID+"/Refresh"+query, nil, nil); err != nil {
		return fmt.Errorf("refreshing item full metadata %s: %w", itemID, err)
	}
	return nil
}

// RefreshItemFullMetadataRecursive triggers a full metadata and image refresh
// for an item and its children. This is used for series-level repair because
// refreshing only a stale episode cannot identify an unidentified parent
// series.
func (c *Client) RefreshItemFullMetadataRecursive(itemID string) error {
	query := "?Recursive=true&MetadataRefreshMode=FullRefresh&ImageRefreshMode=FullRefresh&ReplaceAllMetadata=true&ReplaceAllImages=true"
	if err := c.post("/Items/"+itemID+"/Refresh"+query, nil, nil); err != nil {
		return fmt.Errorf("refreshing item recursive full metadata %s: %w", itemID, err)
	}
	return nil
}

// RefreshItemMetadataRecursive asks Jellyfin to re-read item metadata for a
// series and children without forcing artwork replacement.
func (c *Client) RefreshItemMetadataRecursive(itemID string) error {
	query := "?Recursive=true&MetadataRefreshMode=FullRefresh&ImageRefreshMode=Default&ReplaceAllMetadata=true&ReplaceAllImages=false"
	if err := c.post("/Items/"+itemID+"/Refresh"+query, nil, nil); err != nil {
		return fmt.Errorf("refreshing item recursive metadata %s: %w", itemID, err)
	}
	return nil
}

// GetVirtualFolders returns all configured libraries with their disk paths.
func (c *Client) GetVirtualFolders() ([]VirtualFolder, error) {
	var folders []VirtualFolder
	if err := c.get("/Library/VirtualFolders", &folders); err != nil {
		return nil, fmt.Errorf("getting virtual folders: %w", err)
	}
	return folders, nil
}

// GetPhysicalPaths returns all monitored disk paths.
func (c *Client) GetPhysicalPaths() ([]string, error) {
	var paths []string
	if err := c.get("/Library/PhysicalPaths", &paths); err != nil {
		return nil, fmt.Errorf("getting physical paths: %w", err)
	}
	return paths, nil
}
