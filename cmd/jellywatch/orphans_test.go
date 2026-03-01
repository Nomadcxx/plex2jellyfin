package main

import (
	"bytes"
	"errors"
	"testing"

	"github.com/Nomadcxx/jellywatch/internal/config"
	"github.com/Nomadcxx/jellywatch/internal/jellyfin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockOrphansClient struct {
	orphans   []jellyfin.Item
	results   []jellyfin.RemediationResult
	getErr    error
	fixErr    error
	calledFix bool
	dryRun    bool
}

func (m *mockOrphansClient) GetOrphanedEpisodes() ([]jellyfin.Item, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.orphans, nil
}

func (m *mockOrphansClient) RemediateOrphans(orphans []jellyfin.Item, dryRun bool) ([]jellyfin.RemediationResult, error) {
	m.calledFix = true
	m.dryRun = dryRun
	if m.fixErr != nil {
		return nil, m.fixErr
	}
	return m.results, nil
}

func TestOrphansCommand_ListsOrphansWithoutFix(t *testing.T) {
	client := &mockOrphansClient{
		orphans: []jellyfin.Item{
			{ID: "ep-1", Name: "Orphan One", Path: "/media/orphan1.mkv"},
			{ID: "ep-2", Name: "Orphan Two", Path: "/media/orphan2.mkv"},
		},
	}

	originalLoadConfig := orphansLoadConfig
	originalClientFactory := orphansClientFactory
	t.Cleanup(func() {
		orphansLoadConfig = originalLoadConfig
		orphansClientFactory = originalClientFactory
	})

	orphansLoadConfig = func() (*config.Config, error) {
		return &config.Config{Jellyfin: config.JellyfinConfig{URL: "http://test", APIKey: "test-key"}}, nil
	}
	orphansClientFactory = func(_ *config.Config) orphansClient {
		return client
	}

	cmd := newOrphansCmd()
	buf := bytes.NewBuffer(nil)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Found 2 orphaned episodes")
	assert.Contains(t, buf.String(), "Run with --fix")
	assert.False(t, client.calledFix)
}

func TestOrphansCommand_FixUsesDryRunFlag(t *testing.T) {
	client := &mockOrphansClient{
		orphans: []jellyfin.Item{{ID: "ep-1", Name: "Orphan One", Path: "/media/orphan1.mkv"}},
		results: []jellyfin.RemediationResult{{ItemID: "ep-1", ItemName: "Orphan One", Action: "skipped"}},
	}

	originalLoadConfig := orphansLoadConfig
	originalClientFactory := orphansClientFactory
	t.Cleanup(func() {
		orphansLoadConfig = originalLoadConfig
		orphansClientFactory = originalClientFactory
	})

	orphansLoadConfig = func() (*config.Config, error) {
		return &config.Config{Jellyfin: config.JellyfinConfig{URL: "http://test", APIKey: "test-key"}}, nil
	}
	orphansClientFactory = func(_ *config.Config) orphansClient {
		return client
	}

	cmd := newOrphansCmd()
	buf := bytes.NewBuffer(nil)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--fix"})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.True(t, client.calledFix)
	assert.True(t, client.dryRun)
	assert.Contains(t, buf.String(), "Results")
}

func TestOrphansCommand_ReturnsGetError(t *testing.T) {
	client := &mockOrphansClient{getErr: errors.New("lookup failed")}

	originalLoadConfig := orphansLoadConfig
	originalClientFactory := orphansClientFactory
	t.Cleanup(func() {
		orphansLoadConfig = originalLoadConfig
		orphansClientFactory = originalClientFactory
	})

	orphansLoadConfig = func() (*config.Config, error) {
		return &config.Config{Jellyfin: config.JellyfinConfig{URL: "http://test", APIKey: "test-key"}}, nil
	}
	orphansClientFactory = func(_ *config.Config) orphansClient {
		return client
	}

	cmd := newOrphansCmd()
	buf := bytes.NewBuffer(nil)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "finding orphans")
}
