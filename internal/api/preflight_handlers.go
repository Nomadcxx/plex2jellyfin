package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

type PreflightHandler struct{}

type preflightBody struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
}

type preflightResult struct {
	Path           string   `json:"path"`
	Exists         bool     `json:"exists"`
	IsDir          bool     `json:"is_dir"`
	Readable       bool     `json:"readable"`
	Writable       bool     `json:"writable"`
	OwnerUID       int      `json:"owner_uid"`
	DaemonUIDOK    bool     `json:"daemon_uid_can_access"`
	FreeSpaceBytes int64    `json:"free_space_bytes"`
	Warnings       []string `json:"warnings,omitempty"`
}

func (PreflightHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var body preflightBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}

	res := preflightResult{Path: body.Path}
	info, err := os.Stat(body.Path)
	if err != nil {
		res.Warnings = append(res.Warnings, err.Error())
		writeJSON(w, http.StatusOK, res)
		return
	}

	res.Exists = true
	res.IsDir = info.IsDir()
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		res.OwnerUID = int(stat.Uid)
		res.DaemonUIDOK = res.OwnerUID == os.Getuid()
	}
	if _, err := os.ReadDir(body.Path); err == nil {
		res.Readable = true
	} else {
		res.Warnings = append(res.Warnings, "not readable: "+err.Error())
	}
	if body.Kind == "library" {
		testPath := filepath.Join(body.Path, ".plex2jellyfin_write_test_"+time.Now().Format("150405.000000"))
		if f, err := os.Create(testPath); err == nil {
			_ = f.Close()
			_ = os.Remove(testPath)
			res.Writable = true
		} else {
			res.Warnings = append(res.Warnings, "not writable: "+err.Error())
		}
	}
	var stat syscall.Statfs_t
	if err := syscall.Statfs(body.Path, &stat); err == nil {
		res.FreeSpaceBytes = int64(stat.Bavail) * stat.Bsize
	}
	writeJSON(w, http.StatusOK, res)
}
