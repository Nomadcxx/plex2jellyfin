package api

import (
	"encoding/json"
	"net/http"
	"os"
	"syscall"
)

type PreflightHandler struct{}

type preflightBody struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
}

type PreflightResult struct {
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

type preflightResult = PreflightResult

func (PreflightHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var body preflightBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}

	if body.Kind != "watch" && body.Kind != "library" {
		writeError(w, http.StatusBadRequest, "invalid_kind", "kind must be watch or library")
		return
	}

	writeJSON(w, http.StatusOK, PreflightPath(body.Path))
}

func PreflightPath(path string) PreflightResult {
	res := PreflightResult{Path: path}
	info, err := os.Stat(path)
	if err != nil {
		res.Warnings = append(res.Warnings, err.Error())
		return res
	}

	res.Exists = true
	res.IsDir = info.IsDir()
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		res.OwnerUID = int(stat.Uid)
		res.DaemonUIDOK = res.OwnerUID == os.Getuid()
	}
	if _, err := os.ReadDir(path); err == nil {
		res.Readable = true
	} else {
		res.Warnings = append(res.Warnings, "not readable: "+err.Error())
	}
	if res.IsDir {
		if f, err := os.CreateTemp(path, ".plex2jellyfin_write_test_*"); err == nil {
			testPath := f.Name()
			_ = f.Close()
			_ = os.Remove(testPath)
			res.Writable = true
		} else {
			res.Warnings = append(res.Warnings, "not writable: "+err.Error())
		}
	}
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err == nil {
		res.FreeSpaceBytes = int64(stat.Bavail) * stat.Bsize
	}
	return res
}
