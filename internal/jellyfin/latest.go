package jellyfin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

const latestItemFields = "DateCreated,SeriesId,SeriesName,ImageTags,Overview,Genres,MediaSources,IndexNumber,ParentIndexNumber"

// Latest keeps movies + TV only. Sports/homevideos land as Type=Video in
// Jellyfin and would otherwise dominate Items/Latest.
var latestAllowedTypes = map[string]bool{
	"Movie":   true,
	"Episode": true,
	"Series":  true,
}

// GetLatestItems returns recently added library items for the default admin
// user, dropping Virtual placeholders and non movie/TV types (sports, etc.).
func (c *Client) GetLatestItems(ctx context.Context, limit int) ([]Item, error) {
	if limit <= 0 {
		limit = 24
	}
	userID, err := c.DefaultUserID(ctx)
	if err != nil {
		return nil, err
	}

	// Over-fetch: Jellyfin's IncludeItemTypes is soft on Latest (Series still
	// appears when asking for Movie,Episode), and Sports Video items crowd
	// the unfiltered feed.
	fetchLimit := limit * 3
	if fetchLimit < limit {
		fetchLimit = limit
	}
	if fetchLimit > 100 {
		fetchLimit = 100
	}

	query := url.Values{}
	query.Set("Limit", strconv.Itoa(fetchLimit))
	query.Set("Fields", latestItemFields)
	query.Set("IncludeItemTypes", "Movie,Episode,Series")

	path := "/Users/" + url.PathEscape(userID) + "/Items/Latest?" + query.Encode()
	resp, err := c.requestCtx(ctx, http.MethodGet, path, nil, true)
	if err != nil {
		return nil, fmt.Errorf("getting latest items: %w", err)
	}
	defer resp.Body.Close()

	var items []Item
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, fmt.Errorf("decoding latest items: %w", err)
	}

	out := make([]Item, 0, limit)
	for _, item := range items {
		if item.LocationType == "Virtual" {
			continue
		}
		if !latestAllowedTypes[item.Type] {
			continue
		}
		out = append(out, item)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// GetPrimaryImage fetches a resized Primary image for an item.
func (c *Client) GetPrimaryImage(ctx context.Context, itemID string, fillHeight, fillWidth, quality int) (string, []byte, error) {
	if itemID == "" {
		return "", nil, fmt.Errorf("item id required")
	}
	query := url.Values{}
	query.Set("fillHeight", strconv.Itoa(fillHeight))
	query.Set("fillWidth", strconv.Itoa(fillWidth))
	query.Set("quality", strconv.Itoa(quality))

	path := "/Items/" + url.PathEscape(itemID) + "/Images/Primary?" + query.Encode()
	resp, err := c.requestCtx(ctx, http.MethodGet, path, nil, true)
	if err != nil {
		return "", nil, fmt.Errorf("getting primary image: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", nil, fmt.Errorf("reading primary image: %w", err)
	}
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "image/jpeg"
	}
	return ct, body, nil
}
