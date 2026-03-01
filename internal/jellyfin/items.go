package jellyfin

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

// SearchItems searches Jellyfin's library by name.
func (c *Client) SearchItems(searchTerm string, itemTypes ...string) (*ItemsResponse, error) {
	query := url.Values{}
	query.Set("Recursive", "true")
	if searchTerm != "" {
		query.Set("SearchTerm", searchTerm)
	}
	if len(itemTypes) > 0 {
		query.Set("IncludeItemTypes", strings.Join(itemTypes, ","))
	}

	var resp ItemsResponse
	if err := c.get("/Items?"+query.Encode(), &resp); err != nil {
		return nil, fmt.Errorf("searching items: %w", err)
	}
	return &resp, nil
}

// GetItem returns full metadata for a specific item.
func (c *Client) GetItem(itemID string) (*Item, error) {
	var item Item
	if err := c.get("/Items/"+itemID, &item); err != nil {
		return nil, fmt.Errorf("getting item %s: %w", itemID, err)
	}
	return &item, nil
}

// GetItemByPath finds an item matching a specific filesystem path.
func (c *Client) GetItemByPath(path string) (*Item, error) {
	nameHint := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	items, err := c.SearchItems(nameHint)
	if err != nil {
		return nil, err
	}

	for i := range items.Items {
		if items.Items[i].Path == path {
			return &items.Items[i], nil
		}
	}

	return nil, nil
}

// GetItemsByParent returns child items under a parent.
func (c *Client) GetItemsByParent(parentID string) (*ItemsResponse, error) {
	query := url.Values{}
	query.Set("ParentId", parentID)
	query.Set("Recursive", "true")

	var resp ItemsResponse
	if err := c.get("/Items?"+query.Encode(), &resp); err != nil {
		return nil, fmt.Errorf("getting items by parent: %w", err)
	}
	return &resp, nil
}

func (c *Client) GetOrphanedEpisodes() ([]Item, error) {
	query := url.Values{}
	query.Set("IncludeItemTypes", "Episode")
	query.Set("Recursive", "true")
	query.Set("Fields", "Path,ProviderIds,ParentId,SeriesId")
	query.Set("Limit", "10000")

	var resp ItemsResponse
	if err := c.get("/Items?"+query.Encode(), &resp); err != nil {
		return nil, fmt.Errorf("querying episodes: %w", err)
	}

	orphans := make([]Item, 0, len(resp.Items))
	for _, item := range resp.Items {
		if item.SeriesID == "" {
			orphans = append(orphans, item)
		}
	}

	return orphans, nil
}
