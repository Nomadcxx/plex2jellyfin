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
	StreamWithID(ctx context.Context, cmd ipc.Command, args any, opID string) error
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
	return SaveConfigAndReloadSection(ctx, path, candidate, ipcClient, "")
}

// SaveConfigAndReloadSection saves the candidate config and triggers a daemon
// reload. If section is non-empty, only a reload failure for that specific
// subsystem rolls back the config; failures from other subsystems are surfaced
// as warnings in the response but the new config is kept. This prevents a
// stale watch on an unrelated path (e.g., scanner) from blocking a save to
// another section (e.g., ai).
func SaveConfigAndReloadSection(ctx context.Context, path string, candidate *config.Config, ipcClient IPCCaller, section string) (PutSectionResponse, error) {
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

	rollback := !reload.OK
	if rollback && section != "" {
		// Only roll back if the section we changed actually failed to
		// reload (or if the IPC call itself failed).
		sectionFailed := false
		for _, f := range reload.Failed {
			if f.Name == section || f.Name == "ipc" {
				sectionFailed = true
				break
			}
		}
		rollback = sectionFailed
		if !rollback {
			// Other subsystems failed but our change reloaded fine; surface
			// the partial success so the UI can stop reporting a hard error.
			resp.Reload.OK = true
		}
	}

	if rollback {
		if len(prev) > 0 {
			if err := config.AtomicWriteWithLock(path, prev, 0600); err != nil {
				return resp, err
			}
		}
		resp.RestoredPreviousConfig = true
	}
	return resp, nil
}
