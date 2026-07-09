package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"sync"

	"github.com/Nomadcxx/plex2jellyfin/internal/config"
	"github.com/go-chi/chi/v5"
)

type PathsHandlers struct {
	mu  sync.Mutex
	Cfg *config.Config
	IPC IPCCaller
}

type LibrariesHandlers struct {
	mu  sync.Mutex
	Cfg *config.Config
	IPC IPCCaller
}

type addPathBody struct {
	Path string `json:"path"`
}

type replacePathsBody struct {
	Paths []string `json:"paths"`
}

func (h *PathsHandlers) Get(w http.ResponseWriter, r *http.Request) {
	pathsHandlerGet(w, r, h.read)
}

func (h *PathsHandlers) Add(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()
	pathsHandlerAdd(w, r, h.read, h.write, h.persist)
}

func (h *PathsHandlers) Remove(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()
	pathsHandlerRemove(w, r, h.read, h.write, h.persist)
}

func (h *PathsHandlers) Replace(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()
	pathsHandlerReplace(w, r, h.write, h.persist)
}

func (h *PathsHandlers) read(kind string) ([]string, error) {
	switch kind {
	case "tv":
		return append([]string{}, h.Cfg.Watch.TV...), nil
	case "movies":
		return append([]string{}, h.Cfg.Watch.Movies...), nil
	default:
		return nil, errKindUnknown
	}
}

func (h *PathsHandlers) write(kind string, paths []string) error {
	switch kind {
	case "tv":
		h.Cfg.Watch.TV = append([]string{}, paths...)
		return nil
	case "movies":
		h.Cfg.Watch.Movies = append([]string{}, paths...)
		return nil
	default:
		return errKindUnknown
	}
}

func (h *PathsHandlers) persist(ctx context.Context) error {
	return persistConfigAndReload(ctx, h.Cfg, h.IPC)
}

func (h *LibrariesHandlers) Get(w http.ResponseWriter, r *http.Request) {
	pathsHandlerGet(w, r, h.read)
}

func (h *LibrariesHandlers) Add(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()
	pathsHandlerAdd(w, r, h.read, h.write, h.persist)
}

func (h *LibrariesHandlers) Remove(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()
	pathsHandlerRemove(w, r, h.read, h.write, h.persist)
}

func (h *LibrariesHandlers) Replace(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()
	pathsHandlerReplace(w, r, h.write, h.persist)
}

func (h *LibrariesHandlers) read(kind string) ([]string, error) {
	switch kind {
	case "tv":
		return append([]string{}, h.Cfg.Libraries.TV...), nil
	case "movies":
		return append([]string{}, h.Cfg.Libraries.Movies...), nil
	default:
		return nil, errKindUnknown
	}
}

func (h *LibrariesHandlers) write(kind string, paths []string) error {
	switch kind {
	case "tv":
		h.Cfg.Libraries.TV = append([]string{}, paths...)
		return nil
	case "movies":
		h.Cfg.Libraries.Movies = append([]string{}, paths...)
		return nil
	default:
		return errKindUnknown
	}
}

func (h *LibrariesHandlers) persist(ctx context.Context) error {
	return persistConfigAndReload(ctx, h.Cfg, h.IPC)
}

type pathReader func(kind string) ([]string, error)
type pathWriter func(kind string, paths []string) error
type pathPersister func(ctx context.Context) error

func pathsHandlerGet(w http.ResponseWriter, r *http.Request, read pathReader) {
	paths, err := read(chi.URLParam(r, "kind"))
	if err != nil {
		writeError(w, http.StatusNotFound, "unknown_kind", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string][]string{"paths": paths})
}

func pathsHandlerAdd(w http.ResponseWriter, r *http.Request, read pathReader, write pathWriter, persist pathPersister) {
	var body addPathBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if body.Path == "" {
		writeError(w, http.StatusBadRequest, "path_required", "path required")
		return
	}
	kind := chi.URLParam(r, "kind")
	paths, err := read(kind)
	if err != nil {
		writeError(w, http.StatusNotFound, "unknown_kind", err.Error())
		return
	}
	paths = append(paths, body.Path)
	if err := write(kind, paths); err != nil {
		writeError(w, http.StatusBadRequest, "unknown_kind", err.Error())
		return
	}
	if err := persist(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "config_save_error", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string][]string{"paths": paths})
}

func pathsHandlerRemove(w http.ResponseWriter, r *http.Request, read pathReader, write pathWriter, persist pathPersister) {
	idx, err := strconv.Atoi(chi.URLParam(r, "index"))
	if err != nil || idx < 0 {
		writeError(w, http.StatusBadRequest, "bad_index", "bad index")
		return
	}
	kind := chi.URLParam(r, "kind")
	paths, err := read(kind)
	if err != nil {
		writeError(w, http.StatusNotFound, "unknown_kind", err.Error())
		return
	}
	if idx >= len(paths) {
		writeError(w, http.StatusNotFound, "index_out_of_range", "index out of range")
		return
	}
	paths = append(paths[:idx], paths[idx+1:]...)
	if err := write(kind, paths); err != nil {
		writeError(w, http.StatusBadRequest, "unknown_kind", err.Error())
		return
	}
	if err := persist(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "config_save_error", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func pathsHandlerReplace(w http.ResponseWriter, r *http.Request, write pathWriter, persist pathPersister) {
	var body replacePathsBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if err := write(chi.URLParam(r, "kind"), body.Paths); err != nil {
		writeError(w, http.StatusBadRequest, "unknown_kind", err.Error())
		return
	}
	if err := persist(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "config_save_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string][]string{"paths": body.Paths})
}

func persistConfigAndReload(ctx context.Context, cfg *config.Config, ipcClient IPCCaller) error {
	configPath, err := config.ConfigPath()
	if err != nil {
		return err
	}
	resp, err := SaveConfigAndReload(ctx, configPath, cfg, ipcClient)
	if err != nil {
		return err
	}
	if !resp.Reload.OK {
		if restored, loadErr := config.Load(); loadErr == nil {
			*cfg = *restored
		}
		return errors.New("daemon reload failed; previous config restored")
	}
	return nil
}

var errKindUnknown = errors.New("unknown kind (must be tv or movies)")
