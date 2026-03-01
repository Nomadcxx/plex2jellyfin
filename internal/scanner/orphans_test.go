package scanner

import (
	"errors"
	"testing"

	"github.com/Nomadcxx/jellywatch/internal/jellyfin"
	"github.com/Nomadcxx/jellywatch/internal/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockOrphanChecker struct {
	called  bool
	orphans []jellyfin.Item
	err     error
}

func (m *mockOrphanChecker) GetOrphanedEpisodes() ([]jellyfin.Item, error) {
	m.called = true
	if m.err != nil {
		return nil, m.err
	}
	return m.orphans, nil
}

func TestPeriodicScannerCheckOrphansCallsChecker(t *testing.T) {
	checker := &mockOrphanChecker{
		orphans: []jellyfin.Item{{ID: "ep-1", Name: "Orphan One", Path: "/media/orphan1.mkv"}},
	}

	scanner := &PeriodicScanner{
		logger:      logging.Nop(),
		orphanCheck: checker,
	}

	require.NotPanics(t, func() {
		scanner.checkOrphans()
	})
	assert.True(t, checker.called)
}

func TestPeriodicScannerCheckOrphansHandlesError(t *testing.T) {
	checker := &mockOrphanChecker{err: errors.New("query failed")}

	scanner := &PeriodicScanner{
		logger:      logging.Nop(),
		orphanCheck: checker,
	}

	require.NotPanics(t, func() {
		scanner.checkOrphans()
	})
	assert.True(t, checker.called)
}
