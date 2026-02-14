package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Nomadcxx/jellywatch/api"
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
	// TODO: Implement SSE streaming
	w.WriteHeader(http.StatusNotImplemented)
}

// GetActivity implements api.ServerInterface - Get paginated activity log
func (s *Server) GetActivity(w http.ResponseWriter, r *http.Request, params api.GetActivityParams) {
	// TODO: Implement activity log retrieval
	writeJSON(w, http.StatusOK, []api.ActivityEvent{})
}

// GetAISettings implements api.ServerInterface
func (s *Server) GetAISettings(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement AI settings retrieval
	writeJSON(w, http.StatusOK, api.AISettings{})
}

// Login implements api.ServerInterface
func (s *Server) Login(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement authentication
	writeJSON(w, http.StatusOK, api.AuthStatus{
		Authenticated: ptrBool(false),
		Enabled:       ptrBool(false),
	})
}

// Logout implements api.ServerInterface
func (s *Server) Logout(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement logout
	writeJSON(w, http.StatusOK, api.AuthStatus{
		Authenticated: ptrBool(false),
		Enabled:       ptrBool(false),
	})
}

// GetAuthStatus implements api.ServerInterface
func (s *Server) GetAuthStatus(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement auth status check
	writeJSON(w, http.StatusOK, api.AuthStatus{
		Authenticated: ptrBool(false),
		Enabled:       ptrBool(false),
	})
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