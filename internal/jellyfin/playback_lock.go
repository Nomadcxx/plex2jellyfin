package jellyfin

import (
	"strings"
	"sync"
	"time"
)

// PlaybackInfo captures who is actively playing a file.
type PlaybackInfo struct {
	UserName   string
	DeviceName string
	ClientName string
	ItemID     string
	StartedAt  time.Time
}

// PlaybackLockManager tracks file paths currently being streamed.
type PlaybackLockManager struct {
	mu    sync.RWMutex
	locks map[string]PlaybackInfo
}

func NewPlaybackLockManager() *PlaybackLockManager {
	return &PlaybackLockManager{locks: make(map[string]PlaybackInfo)}
}

func (m *PlaybackLockManager) Lock(path string, info PlaybackInfo) {
	key := normalizePath(path)
	if key == "" {
		return
	}
	if info.StartedAt.IsZero() {
		info.StartedAt = time.Now()
	}

	m.mu.Lock()
	m.locks[key] = info
	m.mu.Unlock()
}

func (m *PlaybackLockManager) Unlock(path string) {
	key := normalizePath(path)
	if key == "" {
		return
	}

	m.mu.Lock()
	delete(m.locks, key)
	m.mu.Unlock()
}

func (m *PlaybackLockManager) IsLocked(path string) (bool, *PlaybackInfo) {
	key := normalizePath(path)
	if key == "" {
		return false, nil
	}

	m.mu.RLock()
	info, ok := m.locks[key]
	m.mu.RUnlock()
	if !ok {
		return false, nil
	}
	copyInfo := info
	return true, &copyInfo
}

func (m *PlaybackLockManager) GetAllLocks() map[string]PlaybackInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make(map[string]PlaybackInfo, len(m.locks))
	for path, info := range m.locks {
		out[path] = info
	}
	return out
}

func (m *PlaybackLockManager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.locks)
}

func normalizePath(path string) string {
	return strings.TrimSpace(path)
}
