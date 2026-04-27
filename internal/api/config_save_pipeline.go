package api

import (
	"context"
	"encoding/json"
	"os"

	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/daemon/ipc"
)

type IPCCaller interface {
	Call(ctx context.Context, cmd ipc.Command, args any) (json.RawMessage, error)
}

type ReloadResult struct {
	OK       bool                    `json:"ok"`
	Reloaded []string                `json:"reloaded"`
	Failed   []ReloadFailedSubsystem `json:"failed,omitempty"`
}

type ReloadFailedSubsystem struct {
	Name  string `json:"name"`
	Error string `json:"error"`
}

type PutSectionResponse struct {
	Saved                  bool         `json:"saved"`
	Validation             any          `json:"validation,omitempty"`
	Reload                 ReloadResult `json:"reload"`
	RestoredPreviousConfig bool         `json:"restored_previous_config"`
}

func SaveConfigAndReload(ctx context.Context, path string, candidate *config.Config, ipcClient IPCCaller) (PutSectionResponse, error) {
	prev, _ := os.ReadFile(path)
	if err := config.AtomicWriteWithLock(path, []byte(candidate.ToTOML()), 0600); err != nil {
		return PutSectionResponse{}, err
	}

	reload := ReloadResult{}
	data, err := ipcClient.Call(ctx, ipc.CmdReload, nil)
	if err != nil {
		reload = ReloadResult{OK: false, Failed: []ReloadFailedSubsystem{{Name: "ipc", Error: err.Error()}}}
	} else if err := json.Unmarshal(data, &reload); err != nil {
		reload = ReloadResult{OK: false, Failed: []ReloadFailedSubsystem{{Name: "ipc", Error: err.Error()}}}
	}

	resp := PutSectionResponse{Saved: true, Reload: reload}
	if !reload.OK {
		if len(prev) > 0 {
			if err := config.AtomicWriteWithLock(path, prev, 0600); err != nil {
				return resp, err
			}
		}
		resp.RestoredPreviousConfig = true
	}
	return resp, nil
}
