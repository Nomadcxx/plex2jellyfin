package api

import (
	"encoding/json"
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
	response := api.DuplicateAnalysis{
		Groups:           make([]api.DuplicateGroup, len(analysis.Groups)),
		TotalFiles:       analysis.TotalFiles,
		TotalGroups:      analysis.TotalGroups,
		ReclaimableBytes: analysis.ReclaimableBytes,
	}

	for i, g := range analysis.Groups {
		group := api.DuplicateGroup{
			Id:               g.ID,
			Title:            g.Title,
			MediaType:        api.DuplicateGroupMediaType(g.MediaType),
			Files:            make([]api.MediaFile, len(g.Files)),
			BestFileId:       g.BestFileID,
			ReclaimableBytes: g.ReclaimableBytes,
		}
		if g.Year != nil {
			group.Year = g.Year
		}
		if g.Season != nil {
			group.Season = g.Season
		}
		if g.Episode != nil {
			group.Episode = g.Episode
		}

		for j, f := range g.Files {
			group.Files[j] = api.MediaFile{
				Id:           f.ID,
				Path:         f.Path,
				Size:         f.Size,
				Resolution:   &f.Resolution,
				SourceType:   &f.SourceType,
				QualityScore: f.QualityScore,
			}
		}
		response.Groups[i] = group
	}

	writeJSON(w, http.StatusOK, response)
}

// DeleteDuplicate implements api.ServerInterface
func (s *Server) DeleteDuplicate(w http.ResponseWriter, r *http.Request, groupId string, params api.DeleteDuplicateParams) {
	// TODO: Implement deletion
	writeJSON(w, http.StatusOK, api.OperationResult{
		Success: true,
		Message: ptrString("Not implemented yet"),
	})
}

// GetScattered implements api.ServerInterface - not in spec but needed
func (s *Server) GetScattered(w http.ResponseWriter, r *http.Request) {
	analysis, err := s.service.AnalyzeScattered()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "analysis_failed", err.Error())
		return
	}

	response := api.ScatteredAnalysis{
		Items:      make([]api.ScatteredItem, len(analysis.Items)),
		TotalItems: analysis.TotalItems,
		TotalMoves: analysis.TotalMoves,
		TotalBytes: analysis.TotalBytes,
	}

	for i, item := range analysis.Items {
		response.Items[i] = api.ScatteredItem{
			Id:             item.ID,
			Title:          item.Title,
			Year:           item.Year,
			MediaType:      item.MediaType,
			Locations:      item.Locations,
			TargetLocation: item.TargetLocation,
			FilesToMove:    item.FilesToMove,
			BytesToMove:    &item.BytesToMove,
		}
	}

	writeJSON(w, http.StatusOK, response)
}

// ConsolidateItem implements api.ServerInterface
func (s *Server) ConsolidateItem(w http.ResponseWriter, r *http.Request, itemId int64) {
	// TODO: Implement consolidation
	writeJSON(w, http.StatusOK, api.OperationResult{
		Success: true,
		Message: ptrString("Not implemented yet"),
	})
}

// StartScan implements api.ServerInterface
func (s *Server) StartScan(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement scan trigger
	writeJSON(w, http.StatusAccepted, api.ScanStatus{
		Status:  api.ScanStatusStatusScanning,
		Message: ptrString("Scan started"),
	})
}

// GetScanStatus implements api.ServerInterface
func (s *Server) GetScanStatus(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement SSE streaming
	writeJSON(w, http.StatusOK, api.ScanStatus{
		Status: api.ScanStatusStatusIdle,
	})
}

// HealthCheck implements api.ServerInterface
func (s *Server) HealthCheck(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"version": "1.0.0",
	})
}

// Helper functions
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(api.Error{
		Code:    code,
		Message: message,
	})
}

func ptrString(s string) *string {
	return &s
}
