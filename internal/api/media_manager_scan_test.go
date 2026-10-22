package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Nomadcxx/plex2jellyfin/internal/config"
)

func TestTriggerManagerScanDoesNotReportUnsupportedScansAsSuccessful(t *testing.T) {
	tests := []struct {
		name        string
		managerID   string
		configure   func(*config.Config)
		wantStatus  int
		wantCode    string
		wantMessage string
	}{
		{
			name:        "configured manager is unsupported",
			managerID:   "sonarr",
			configure:   func(cfg *config.Config) { cfg.Sonarr.Enabled = true; cfg.Sonarr.URL = "http://sonarr.local" },
			wantStatus:  http.StatusNotImplemented,
			wantCode:    "not_implemented",
			wantMessage: "Scan trigger is not implemented",
		},
		{
			name:       "unconfigured manager is rejected",
			managerID:  "sonarr",
			wantStatus: http.StatusNotFound,
			wantCode:   "not_found",
		},
		{
			name:       "unknown manager is rejected",
			managerID:  "unknown",
			wantStatus: http.StatusNotFound,
			wantCode:   "not_found",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.DefaultConfig()
			if tc.configure != nil {
				tc.configure(cfg)
			}
			server := NewServer(nil, cfg)
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/media-managers/"+tc.managerID+"/scan", nil)

			server.TriggerManagerScan(response, request, tc.managerID)

			if response.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d: %s", response.Code, tc.wantStatus, response.Body.String())
			}
			var got struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if got.Code != tc.wantCode {
				t.Fatalf("error code = %q, want %q", got.Code, tc.wantCode)
			}
			if tc.wantMessage != "" && !strings.Contains(got.Message, tc.wantMessage) {
				t.Fatalf("error message = %q, want it to contain %q", got.Message, tc.wantMessage)
			}
		})
	}
}
