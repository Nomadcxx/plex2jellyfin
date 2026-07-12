package api

import (
	"net/http"
	"time"
)

// clearSSEWriteDeadline removes the server WriteTimeout for long-lived SSE
// responses. Without this, http.Server.WriteTimeout (30s by default in
// plex2jellyfin-web) aborts library indexing mid-stream.
func clearSSEWriteDeadline(w http.ResponseWriter) {
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
}
