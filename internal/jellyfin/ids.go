package jellyfin

import (
	"strings"

	"github.com/google/uuid"
)

// NormalizeItemID returns GUID-shaped Jellyfin IDs in the compact lowercase
// form used by the REST API. Other ID forms pass through unchanged after trim.
func NormalizeItemID(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) != 32 && len(trimmed) != 36 {
		return trimmed
	}
	id, err := uuid.Parse(trimmed)
	if err != nil {
		return trimmed
	}
	return strings.ReplaceAll(id.String(), "-", "")
}
