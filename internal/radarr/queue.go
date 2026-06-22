package radarr

import (
	"context"
	"fmt"
	"strings"
)

func (c *Client) GetQueue(page, pageSize int) (*QueueResponse, error) {
	return c.GetQueueContext(context.Background(), page, pageSize)
}

func (c *Client) GetQueueContext(ctx context.Context, page, pageSize int) (*QueueResponse, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 25
	}

	endpoint := fmt.Sprintf("/api/v3/queue?page=%d&pageSize=%d&includeMovie=true", page, pageSize)
	var response QueueResponse
	if err := c.getContext(ctx, endpoint, &response); err != nil {
		return nil, fmt.Errorf("getting queue: %w", err)
	}
	return &response, nil
}

func (c *Client) GetAllQueueItems() ([]QueueItem, error) {
	return c.GetAllQueueItemsContext(context.Background())
}

func (c *Client) GetAllQueueItemsContext(ctx context.Context) ([]QueueItem, error) {
	var allItems []QueueItem
	page := 1
	pageSize := 100

	for {
		response, err := c.GetQueueContext(ctx, page, pageSize)
		if err != nil {
			return nil, err
		}

		allItems = append(allItems, response.Records...)

		if len(allItems) >= response.TotalRecords {
			break
		}
		page++
	}

	return allItems, nil
}

func (c *Client) GetQueueItem(id int) (*QueueItem, error) {
	return c.GetQueueItemContext(context.Background(), id)
}

func (c *Client) GetQueueItemContext(ctx context.Context, id int) (*QueueItem, error) {
	endpoint := fmt.Sprintf("/api/v3/queue/%d", id)
	var item QueueItem
	if err := c.getContext(ctx, endpoint, &item); err != nil {
		return nil, fmt.Errorf("getting queue item %d: %w", id, err)
	}
	return &item, nil
}

func (c *Client) RemoveFromQueue(id int, blocklist, removeFromClient bool) error {
	return c.RemoveFromQueueContext(context.Background(), id, blocklist, removeFromClient)
}

func (c *Client) RemoveFromQueueContext(ctx context.Context, id int, blocklist, removeFromClient bool) error {
	endpoint := fmt.Sprintf("/api/v3/queue/%d?removeFromClient=%t&blocklist=%t",
		id, removeFromClient, blocklist)
	if err := c.deleteContext(ctx, endpoint); err != nil {
		return fmt.Errorf("removing queue item %d: %w", id, err)
	}
	return nil
}

func (c *Client) BulkRemoveFromQueue(ids []int, blocklist, removeFromClient bool) error {
	endpoint := fmt.Sprintf("/api/v3/queue/bulk?removeFromClient=%t&blocklist=%t",
		removeFromClient, blocklist)

	payload := BulkQueueRequest{
		IDs:              ids,
		RemoveFromClient: removeFromClient,
		Blocklist:        blocklist,
	}

	if err := c.post(endpoint, payload, nil); err != nil {
		return fmt.Errorf("bulk removing queue items: %w", err)
	}
	return nil
}

func (c *Client) GrabQueueItem(id int) error {
	endpoint := fmt.Sprintf("/api/v3/queue/grab/%d", id)
	if err := c.post(endpoint, nil, nil); err != nil {
		return fmt.Errorf("grabbing queue item %d: %w", id, err)
	}
	return nil
}

func (c *Client) GetStuckItems() ([]QueueItem, error) {
	return c.GetStuckItemsContext(context.Background())
}

func (c *Client) GetStuckItemsContext(ctx context.Context) ([]QueueItem, error) {
	allItems, err := c.GetAllQueueItemsContext(ctx)
	if err != nil {
		return nil, err
	}

	var stuck []QueueItem
	for _, item := range allItems {
		status := strings.ToLower(item.TrackedDownloadStatus)
		if status == "warning" || status == "error" {
			stuck = append(stuck, item)
		}
	}

	return stuck, nil
}

func (c *Client) ClearStuckItems(blocklist bool) (int, error) {
	stuck, err := c.GetStuckItems()
	if err != nil {
		return 0, err
	}

	if len(stuck) == 0 {
		return 0, nil
	}

	ids := make([]int, len(stuck))
	for i, item := range stuck {
		ids[i] = item.ID
	}

	if err := c.BulkRemoveFromQueue(ids, blocklist, false); err != nil {
		return 0, err
	}

	return len(stuck), nil
}

func (c *Client) GetItemsWithImportErrors() ([]QueueItem, error) {
	allItems, err := c.GetAllQueueItems()
	if err != nil {
		return nil, err
	}

	var errored []QueueItem
	for _, item := range allItems {
		for _, msg := range item.StatusMessages {
			msgLower := strings.ToLower(msg.Title)
			if strings.Contains(msgLower, "import") ||
				strings.Contains(msgLower, "permission") ||
				strings.Contains(msgLower, "error") {
				errored = append(errored, item)
				break
			}
		}
	}

	return errored, nil
}
