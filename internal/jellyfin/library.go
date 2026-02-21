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
