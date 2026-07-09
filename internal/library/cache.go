package library

import (
	"fmt"
	"sync"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/sonarr"
)

type SeriesCache struct {
	client     *sonarr.Client
	series     map[string]*sonarr.Series // normalized title -> series
	expiry     time.Time
	ttl        time.Duration
	mu         sync.RWMutex
	refreshing bool
}

func NewSeriesCache(client *sonarr.Client, ttl time.Duration) *SeriesCache {
	c := &SeriesCache{
		client: client,
		series: make(map[string]*sonarr.Series),
		ttl:    ttl,
	}
	// Initial refresh
	c.refresh()
	return c
}

func (c *SeriesCache) FindSeries(showName, year string) *sonarr.Series {
	c.ensureFresh()

	c.mu.RLock()
	defer c.mu.RUnlock()

	key := normalizeTitle(showName)
	if s, ok := c.series[key]; ok {
		return s
	}

	// Try with year
	keyWithYear := key + year
	if s, ok := c.series[keyWithYear]; ok {
		return s
	}

	return nil
}

func (c *SeriesCache) ensureFresh() {
	c.mu.RLock()
	needsRefresh := time.Now().After(c.expiry) && !c.refreshing
	c.mu.RUnlock()

	if needsRefresh {
		go c.refresh()
	}
}

func (c *SeriesCache) refresh() {
	c.mu.Lock()
	if c.refreshing {
		c.mu.Unlock()
		return
	}
	c.refreshing = true
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		c.refreshing = false
		c.mu.Unlock()
	}()

	// Skip if client is nil (for testing)
	if c.client == nil {
		return
	}

	allSeries, err := c.client.GetAllSeries()
	if err != nil {
		return // Keep stale cache on error
	}

	newCache := make(map[string]*sonarr.Series)
	for i := range allSeries {
		s := &allSeries[i]
		key := normalizeTitle(s.Title)
		newCache[key] = s

		// Also index by title+year
		if s.Year > 0 {
			keyWithYear := key + fmt.Sprintf("%d", s.Year)
			newCache[keyWithYear] = s
		}
	}

	c.mu.Lock()
	c.series = newCache
	c.expiry = time.Now().Add(c.ttl)
	c.mu.Unlock()
}

// ForceRefresh triggers an immediate cache refresh
func (c *SeriesCache) ForceRefresh() {
	c.mu.Lock()
	c.expiry = time.Time{} // Expire immediately
	c.mu.Unlock()
	c.refresh()
}
