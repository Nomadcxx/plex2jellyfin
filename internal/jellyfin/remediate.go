package jellyfin

import "log/slog"

type RemediationResult struct {
	ItemID   string
	ItemName string
	ItemPath string
	Action   string
	Error    error
	SeriesID string
}

func (c *Client) RemediateOrphans(orphans []Item, dryRun bool) ([]RemediationResult, error) {
	results := make([]RemediationResult, 0, len(orphans))

	for _, orphan := range orphans {
		result := RemediationResult{
			ItemID:   orphan.ID,
			ItemName: orphan.Name,
			ItemPath: orphan.Path,
		}

		if dryRun {
			result.Action = "skipped"
			slog.Info("dry-run orphan remediation", "item_id", orphan.ID, "name", orphan.Name, "path", orphan.Path)
			results = append(results, result)
			continue
		}

		if err := c.RefreshItem(orphan.ID); err != nil {
			result.Action = "failed"
			result.Error = err
			slog.Error("orphan remediation refresh failed", "item_id", orphan.ID, "name", orphan.Name, "error", err)
			results = append(results, result)
			continue
		}

		result.Action = "refreshed"
		slog.Info("orphan remediated", "item_id", orphan.ID, "name", orphan.Name, "path", orphan.Path)
		results = append(results, result)
	}

	return results, nil
}
