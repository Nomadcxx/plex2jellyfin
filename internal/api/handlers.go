package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/Nomadcxx/jellywatch/api"
	"github.com/Nomadcxx/jellywatch/internal/activity"
	"github.com/google/uuid"
)

// Ensure Server implements the interface
var _ api.ServerInterface = (*Server)(nil)

// GetDuplicates implements api.ServerInterface
func (s *Server) GetDuplicates(w http.ResponseWriter, r *http.Request) {
	analysis, err := s.service.AnalyzeDuplicates()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "analysis_failed", err.Error())
		return
	}

	// Convert to API types
	groups := make([]api.DuplicateGroup, len(analysis.Groups))
	for i, g := range analysis.Groups {
		group := api.DuplicateGroup{
			Id:               &g.ID,
			Title:            &g.Title,
			MediaType:        ptrDuplicateGroupMediaType(g.MediaType),
			ReclaimableBytes: &g.ReclaimableBytes,
		}
		if g.BestFileID != 0 {
			group.BestFileId = ptrInt64(g.BestFileID)
		}
		if g.Year != nil {
			group.Year = g.Year
		}

		files := make([]api.MediaFile, len(g.Files))
		for j, f := range g.Files {
			files[j] = api.MediaFile{
				Id:           &f.ID,
				Path:         &f.Path,
				Size:         &f.Size,
				Resolution:   &f.Resolution,
				SourceType:   &f.SourceType,
				QualityScore: &f.QualityScore,
			}
		}
		group.Files = &files
		groups[i] = group
	}

	response := api.DuplicateAnalysis{
		Groups:           &groups,
		TotalFiles:       &analysis.TotalFiles,
		TotalGroups:      &analysis.TotalGroups,
		ReclaimableBytes: &analysis.ReclaimableBytes,
	}

	writeJSON(w, http.StatusOK, response)
}

// DeleteDuplicate implements api.ServerInterface
func (s *Server) DeleteDuplicate(w http.ResponseWriter, r *http.Request, groupId string, params api.DeleteDuplicateParams) {
	// TODO: Implement deletion
	writeJSON(w, http.StatusOK, api.OperationResult{
		Success: ptrBool(true),
		Message: ptrString("Not implemented yet"),
	})
}

// GetScattered implements api.ServerInterface
func (s *Server) GetScattered(w http.ResponseWriter, r *http.Request) {
	analysis, err := s.service.AnalyzeScattered()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "analysis_failed", err.Error())
		return
	}

	items := make([]api.ScatteredItem, len(analysis.Items))
	for i, item := range analysis.Items {
		items[i] = api.ScatteredItem{
			Id:             &item.ID,
			Title:          &item.Title,
			Year:           item.Year,
			MediaType:      &item.MediaType,
			Locations:      &item.Locations,
			TargetLocation: &item.TargetLocation,
			FilesToMove:    &item.FilesToMove,
			BytesToMove:    &item.BytesToMove,
		}
	}

	response := api.ScatteredAnalysis{
		Items:      &items,
		TotalItems: &analysis.TotalItems,
		TotalMoves: &analysis.TotalMoves,
		TotalBytes: &analysis.TotalBytes,
	}

	writeJSON(w, http.StatusOK, response)
}

// ConsolidateItem implements api.ServerInterface
func (s *Server) ConsolidateItem(w http.ResponseWriter, r *http.Request, itemId int64) {
	// TODO: Implement consolidation
	writeJSON(w, http.StatusOK, api.OperationResult{
		Success: ptrBool(true),
		Message: ptrString("Not implemented yet"),
	})
}

// StartScan implements api.ServerInterface
func (s *Server) StartScan(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement scan trigger
	status := api.Scanning
	writeJSON(w, http.StatusAccepted, api.ScanStatus{
		Status:  &status,
		Message: ptrString("Scan started"),
	})
}

// GetScanStatus implements api.ServerInterface
func (s *Server) GetScanStatus(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement SSE streaming
	status := api.Idle
	writeJSON(w, http.StatusOK, api.ScanStatus{
		Status: &status,
	})
}

// HealthCheck implements api.ServerInterface
func (s *Server) HealthCheck(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"version": "1.0.0",
	})
}

// ActivityStream implements api.ServerInterface - SSE stream for real-time activity
func (s *Server) ActivityStream(w http.ResponseWriter, r *http.Request) {
	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Flush helper
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming_unsupported", "Streaming not supported")
		return
	}

	// Send initial connection message
	fmt.Fprintf(w, "event: connected\ndata: {\"message\":\"Connected to activity stream\"}\n\n")
	flusher.Flush()

	// Heartbeat ticker
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			// Client disconnected
			return
		case <-ticker.C:
			// Send heartbeat
			fmt.Fprintf(w, "event: heartbeat\ndata: {\"ts\":%d}\n\n", time.Now().Unix())
			flusher.Flush()
		}
	}
}

// GetActivity implements api.ServerInterface - Get paginated activity log
func (s *Server) GetActivity(w http.ResponseWriter, r *http.Request, params api.GetActivityParams) {
	// Default limit to 50 if not specified
	limit := 50
	if params.Limit != nil && *params.Limit > 0 {
		limit = *params.Limit
		// Cap at 500
		if limit > 500 {
			limit = 500
		}
	}

	// Check if activity logger is available
	if s.activityLogger == nil {
		writeJSON(w, http.StatusOK, []api.ActivityEvent{})
		return
	}

	// Get recent entries from activity log
	entries, err := s.activityLogger.GetRecentEntries(limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "activity_read_error", fmt.Sprintf("Failed to read activity log: %v", err))
		return
	}

	// Convert to API ActivityEvent type
	events := make([]api.ActivityEvent, len(entries))
	for i, entry := range entries {
		events[i] = convertEntryToActivityEvent(entry)
	}

	writeJSON(w, http.StatusOK, events)
}

// convertEntryToActivityEvent converts an activity.Entry to an API ActivityEvent
func convertEntryToActivityEvent(entry activity.Entry) api.ActivityEvent {
	event := api.ActivityEvent{
		Id:        ptrString(generateEventID(entry)),
		Timestamp: &entry.Timestamp,
		Message:   ptrString(buildActivityMessage(entry)),
		Type:      ptrActivityEventType(determineEventType(entry)),
	}
	return event
}

// generateEventID creates a unique ID for an activity entry
func generateEventID(entry activity.Entry) string {
	// Use timestamp and source to create a unique ID
	return uuid.NewMD5(uuid.Nil, []byte(fmt.Sprintf("%d-%s-%s", entry.Timestamp.UnixNano(), entry.Action, entry.Source))).String()
}

// buildActivityMessage creates a human-readable message for an activity entry
func buildActivityMessage(entry activity.Entry) string {
	switch entry.Action {
	case "file_detected":
		if entry.Success {
			return fmt.Sprintf("Detected: %s", entry.Source)
		}
		return fmt.Sprintf("Detection failed: %s", entry.Source)
	case "file_organized":
		if entry.Success {
			if entry.Target != "" {
				return fmt.Sprintf("Organized: %s -> %s", entry.Source, entry.Target)
			}
			return fmt.Sprintf("Organized: %s", entry.Source)
		}
		return fmt.Sprintf("Organization failed: %s - %s", entry.Source, entry.Error)
	case "duplicate_found":
		return fmt.Sprintf("Duplicate found: %s", entry.Source)
	case "sync_completed":
		return fmt.Sprintf("Sync completed: %s", entry.Source)
	default:
		if entry.Error != "" {
			return fmt.Sprintf("%s: %s (error: %s)", entry.Action, entry.Source, entry.Error)
		}
		return fmt.Sprintf("%s: %s", entry.Action, entry.Source)
	}
}

// determineEventType maps an activity entry to an API event type
func determineEventType(entry activity.Entry) api.ActivityEventType {
	switch entry.Action {
	case "file_detected":
		return api.FILEDETECTED
	case "file_organized":
		return api.FILEORGANIZED
	case "duplicate_found":
		return api.DUPLICATEFOUND
	case "sync_completed":
		return api.SYNCCOMPLETED
	default:
		if !entry.Success {
			return api.ERROR
		}
		return api.FILEORGANIZED
	}
}

// ptrActivityEventType creates a pointer to an ActivityEventType
func ptrActivityEventType(t api.ActivityEventType) *api.ActivityEventType {
	return &t
}

// GetAISettings implements api.ServerInterface
func (s *Server) GetAISettings(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement AI settings retrieval
	writeJSON(w, http.StatusOK, api.AISettings{})
}

// ListLLMProviders implements api.ServerInterface
func (s *Server) ListLLMProviders(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement LLM providers listing
	writeJSON(w, http.StatusOK, []api.LLMProviderInfo{})
}

// GetLLMProviderStatus implements api.ServerInterface
func (s *Server) GetLLMProviderStatus(w http.ResponseWriter, r *http.Request, providerId string) {
	// TODO: Implement LLM provider status check
	writeJSON(w, http.StatusOK, api.LLMProviderStatus{
		Online: ptrBool(false),
	})
}

// ListMediaManagers implements api.ServerInterface
func (s *Server) ListMediaManagers(w http.ResponseWriter, r *http.Request) {
	managers := buildMediaManagers(s.cfg)
	writeJSON(w, http.StatusOK, managers)
}

// GetMediaManagerQueue implements api.ServerInterface
func (s *Server) GetMediaManagerQueue(w http.ResponseWriter, r *http.Request, managerId string, params api.GetMediaManagerQueueParams) {
	// Check if manager is configured
	if !isManagerConfigured(s.cfg, managerId) {
		writeError(w, http.StatusNotFound, "not_found", fmt.Sprintf("Media manager '%s' not configured", managerId))
		return
	}

	// Get client
	client, err := getManagerClient(s.cfg, managerId)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", err.Error())
		return
	}

	// Fetch queue items
	items, err := client.GetAllQueueItems()
	if err != nil {
		writeError(w, http.StatusBadGateway, "manager_error", fmt.Sprintf("Failed to get queue: %v", err))
		return
	}

	// Convert to API types
	apiItems := make([]api.QueueItem, len(items))
	for i, item := range items {
		apiItems[i] = convertToAPIQueueItem(item)
	}

	// Apply limit if specified
	if params.Limit != nil && *params.Limit > 0 && len(apiItems) > *params.Limit {
		apiItems = apiItems[:*params.Limit]
	}

	writeJSON(w, http.StatusOK, apiItems)
}

// ClearQueueItem implements api.ServerInterface
func (s *Server) ClearQueueItem(w http.ResponseWriter, r *http.Request, managerId string, itemId int64, params api.ClearQueueItemParams) {
	// Check if manager is configured
	if !isManagerConfigured(s.cfg, managerId) {
		writeError(w, http.StatusNotFound, "not_found", fmt.Sprintf("Media manager '%s' not configured", managerId))
		return
	}

	// Get client
	client, err := getManagerClient(s.cfg, managerId)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", err.Error())
		return
	}

	// Determine blocklist option
	blocklist := false
	if params.Blocklist != nil {
		blocklist = *params.Blocklist
	}

	// Remove from queue
	if err := client.RemoveFromQueue(int(itemId), blocklist); err != nil {
		writeError(w, http.StatusBadGateway, "manager_error", fmt.Sprintf("Failed to clear queue item: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, api.OperationResult{
		Success: ptrBool(true),
		Message: ptrString(fmt.Sprintf("Queue item %d cleared successfully", itemId)),
	})
}

// TriggerManagerScan implements api.ServerInterface
func (s *Server) TriggerManagerScan(w http.ResponseWriter, r *http.Request, managerId string) {
	// TODO: Implement manager scan trigger
	writeJSON(w, http.StatusOK, api.OperationResult{
		Success: ptrBool(true),
		Message: ptrString("Not implemented yet"),
	})
}

// GetMediaManagerStatus implements api.ServerInterface
func (s *Server) GetMediaManagerStatus(w http.ResponseWriter, r *http.Request, managerId string) {
	// Check if manager is configured
	if !isManagerConfigured(s.cfg, managerId) {
		writeError(w, http.StatusNotFound, "not_found", fmt.Sprintf("Media manager '%s' not configured", managerId))
		return
	}

	// Get client
	client, err := getManagerClient(s.cfg, managerId)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", err.Error())
		return
	}

	// Get system status (version)
	version, err := client.GetSystemStatus()
	if err != nil {
		writeJSON(w, http.StatusOK, api.ServiceStatus{
			Online: ptrBool(false),
			Error:  ptrString(fmt.Sprintf("Failed to connect: %v", err)),
		})
		return
	}

	// Get queue info for additional status
	items, err := client.GetAllQueueItems()
	if err != nil {
		writeJSON(w, http.StatusOK, api.ServiceStatus{
			Online:  ptrBool(true),
			Version: &version,
			Error:   ptrString(fmt.Sprintf("Failed to get queue: %v", err)),
		})
		return
	}

	// Get stuck count
	stuckItems, _ := client.GetStuckItems()
	stuckCount := len(stuckItems)

	writeJSON(w, http.StatusOK, api.ServiceStatus{
		Online:     ptrBool(true),
		Version:    &version,
		QueueSize:  ptrInt(len(items)),
		StuckCount: ptrInt(stuckCount),
	})
}

// ClearStuckItems implements api.ServerInterface
func (s *Server) ClearStuckItems(w http.ResponseWriter, r *http.Request, managerId string, params api.ClearStuckItemsParams) {
	// Check if manager is configured
	if !isManagerConfigured(s.cfg, managerId) {
		writeError(w, http.StatusNotFound, "not_found", fmt.Sprintf("Media manager '%s' not configured", managerId))
		return
	}

	// Get client
	client, err := getManagerClient(s.cfg, managerId)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", err.Error())
		return
	}

	// Determine blocklist option
	blocklist := false
	if params.Blocklist != nil {
		blocklist = *params.Blocklist
	}

	// Clear stuck items
	count, err := client.ClearStuckItems(blocklist)
	if err != nil {
		writeError(w, http.StatusBadGateway, "manager_error", fmt.Sprintf("Failed to clear stuck items: %v", err))
		return
	}

	message := fmt.Sprintf("Cleared %d stuck items", count)
	if blocklist {
		message = fmt.Sprintf("Cleared %d stuck items and added to blocklist", count)
	}

	writeJSON(w, http.StatusOK, api.OperationResult{
		Success:       ptrBool(true),
		Message:       &message,
		FilesAffected: ptrInt(count),
	})
}

// GetStuckItems implements api.ServerInterface
func (s *Server) GetStuckItems(w http.ResponseWriter, r *http.Request, managerId string) {
	// Check if manager is configured
	if !isManagerConfigured(s.cfg, managerId) {
		writeError(w, http.StatusNotFound, "not_found", fmt.Sprintf("Media manager '%s' not configured", managerId))
		return
	}

	// Get client
	client, err := getManagerClient(s.cfg, managerId)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", err.Error())
		return
	}

	// Fetch stuck items
	items, err := client.GetStuckItems()
	if err != nil {
		writeError(w, http.StatusBadGateway, "manager_error", fmt.Sprintf("Failed to get stuck items: %v", err))
		return
	}

	// Convert to API types
	apiItems := make([]api.QueueItem, len(items))
	for i, item := range items {
		apiItems[i] = convertToAPIQueueItem(item)
	}

	writeJSON(w, http.StatusOK, apiItems)
}

// ScanStream implements api.ServerInterface - SSE stream for scan progress
func (s *Server) ScanStream(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement SSE streaming
	w.WriteHeader(http.StatusNotImplemented)
}

// Helper functions
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{
		"code":    code,
		"message": message,
	})
}

func ptrString(s string) *string {
	return &s
}

func ptrBool(b bool) *bool {
	return &b
}

func ptrInt(i int) *int {
	return &i
}

func ptrInt64(i int64) *int64 {
	return &i
}

func ptrDuplicateGroupMediaType(s string) *api.DuplicateGroupMediaType {
	t := api.DuplicateGroupMediaType(s)
	return &t
}