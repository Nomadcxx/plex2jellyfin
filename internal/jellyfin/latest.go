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

const latestItemFields = "DateCreated,SeriesId,SeriesName,ImageTags,Overview,Genres,MediaSources"

// GetLatestItems returns recently added library items for the default admin
// user, dropping Virtual placeholders Jellyfin sometimes includes.
func (c *Client) GetLatestItems(ctx context.Context, limit int) ([]Item, error) {
	if limit <= 0 {
		limit = 24
	}
	userID, err := c.DefaultUserID(ctx)
	if err != nil {
		return nil, err
	}

	query := url.Values{}
	query.Set("Limit", strconv.Itoa(limit))
	query.Set("Fields", latestItemFields)

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

	out := make([]Item, 0, len(items))
	for _, item := range items {
		if item.LocationType == "Virtual" {
			continue
		}
		out = append(out, item)
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
