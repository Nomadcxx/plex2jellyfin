package jellyfin

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassifyMetadata(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	oldTargetAt := now.Add(-30 * time.Minute)
	recentTargetAt := now.Add(-5 * time.Minute)
	episodeNumber := 3
	seasonNumber := 1

	tests := []struct {
		name       string
		row        *database.ParseDecision
		item       *Item
		series     *Item
		wantState  string
		identified bool
	}{
		{
			name:       "movie with provider IDs",
			row:        decisionWithTarget("/media/movie.mkv", oldTargetAt),
			item:       &Item{ID: "movie-1", Type: "Movie", Path: "/media/movie.mkv", ProviderIDs: map[string]string{"Tmdb": "123"}},
			wantState:  MetadataStateIdentified,
			identified: true,
		},
		{
			name:      "movie without provider IDs",
			row:       decisionWithTarget("/media/movie.mkv", oldTargetAt),
			item:      &Item{ID: "movie-1", Type: "Movie", Path: "/media/movie.mkv"},
			wantState: MetadataStateMissingProviderIDs,
		},
		{
			name: "episode with provider IDs and episode numbers",
			row:  decisionWithTarget("/media/show/s01e03.mkv", oldTargetAt),
			item: &Item{
				ID:                "episode-1",
				Type:              "Episode",
				Path:              "/media/show/s01e03.mkv",
				ProviderIDs:       map[string]string{"Tvdb": "456"},
				SeriesID:          "series-1",
				IndexNumber:       &episodeNumber,
				ParentIndexNumber: &seasonNumber,
			},
			wantState:  MetadataStateIdentified,
			identified: true,
		},
		{
			name: "episode with SeriesId and missing IndexNumber",
			row:  decisionWithTarget("/media/show/s01e03.mkv", oldTargetAt),
			item: &Item{
				ID:                "episode-1",
				Type:              "Episode",
				Path:              "/media/show/s01e03.mkv",
				ProviderIDs:       map[string]string{"Tvdb": "456"},
				SeriesID:          "series-1",
				ParentIndexNumber: &seasonNumber,
			},
			wantState: MetadataStateMissingEpisodeNumbers,
		},
		{
			name:      "episode with no provider IDs but parent series identified",
			row:       decisionWithTarget("/media/show/s01e03.mkv", oldTargetAt),
			item:      &Item{ID: "episode-1", Type: "Episode", Path: "/media/show/s01e03.mkv", SeriesID: "series-1"},
			series:    &Item{ID: "series-1", Type: "Series", ProviderIDs: map[string]string{"Tvdb": "789"}},
			wantState: MetadataStateSeriesIdentifiedEpisodeStale,
		},
		{
			name: "episode with no provider IDs but parent series identified and episode metadata present",
			row:  decisionWithTarget("/media/show/s01e03.mkv", oldTargetAt),
			item: &Item{
				ID:                "episode-1",
				Type:              "Episode",
				Path:              "/media/show/s01e03.mkv",
				SeriesID:          "series-1",
				IndexNumber:       &episodeNumber,
				ParentIndexNumber: &seasonNumber,
				Overview:          "A real episode overview.",
				ImageTags:         map[string]string{"Primary": "abc"},
			},
			series:     &Item{ID: "series-1", Type: "Series", ProviderIDs: map[string]string{"Tvdb": "789"}},
			wantState:  MetadataStateIdentified,
			identified: true,
		},
		{
			name:      "episode with parent series no provider IDs",
			row:       decisionWithTarget("/media/show/s01e03.mkv", oldTargetAt),
			item:      &Item{ID: "episode-1", Type: "Episode", Path: "/media/show/s01e03.mkv", SeriesID: "series-1"},
			series:    &Item{ID: "series-1", Type: "Series"},
			wantState: MetadataStateSeriesUnidentified,
		},
		{
			name:      "nil item",
			row:       decisionWithTarget("/media/missing.mkv", oldTargetAt),
			wantState: MetadataStateJellyfinItemMissing,
		},
		{
			name:      "path mismatch",
			row:       decisionWithTarget("/media/movie.mkv", oldTargetAt),
			item:      &Item{ID: "movie-1", Type: "Movie", Path: "/other/movie.mkv", ProviderIDs: map[string]string{"Tmdb": "123"}},
			wantState: MetadataStatePathMismatch,
		},
		{
			name:      "recent target_at within 15 minutes",
			row:       decisionWithTarget("/media/movie.mkv", recentTargetAt),
			item:      &Item{ID: "movie-1", Type: "Movie", Path: "/media/movie.mkv"},
			wantState: MetadataStateRecentImportWaiting,
		},
		{
			name: "date-based Daily Show episode without IndexNumber",
			row: dateEpisodeDecision(
				"/media/The Daily Show/The Daily Show 2026-07-29.mkv",
				oldTargetAt, 2026, 729,
			),
			item: &Item{
				ID:           "episode-daily",
				Type:         "Episode",
				Path:         "/media/The Daily Show/The Daily Show 2026-07-29.mkv",
				SeriesID:     "series-daily",
				PremiereDate: "2026-07-29",
				Overview:     "Guest episode.",
			},
			series:     &Item{ID: "series-daily", Type: "Series", ProviderIDs: map[string]string{"Tvdb": "71256"}},
			wantState:  MetadataStateIdentified,
			identified: true,
		},
		{
			// Date-based numbering is a daily-show convention, so the episode
			// one day away is a real, distinct episode. A tolerance window
			// would validate that neighbour as confidently as the right file,
			// which is the exact off-by-one this check has to catch.
			name: "date-based episode one day off is the neighbouring episode, not a match",
			row: dateEpisodeDecision(
				"/media/The Daily Show/The Daily Show 2026-07-30.mkv",
				oldTargetAt, 2026, 730,
			),
			item: &Item{
				ID:           "episode-offbyone",
				Type:         "Episode",
				Path:         "/media/The Daily Show/The Daily Show 2026-07-30.mkv",
				SeriesID:     "series-daily",
				PremiereDate: "2026-07-31", // next calendar day
				Overview:     "The following night's episode.",
			},
			series:    &Item{ID: "series-daily", Type: "Series", ProviderIDs: map[string]string{"Tvdb": "71256"}},
			wantState: MetadataStateSeriesIdentifiedEpisodeStale,
		},
		{
			// Same shape in the other direction: the previous day's episode.
			name: "date-based episode one day early is the previous episode, not a match",
			row: dateEpisodeDecision(
				"/media/The Daily Show/The Daily Show 2026-07-30.mkv",
				oldTargetAt, 2026, 730,
			),
			item: &Item{
				ID:           "episode-offbyone-back",
				Type:         "Episode",
				Path:         "/media/The Daily Show/The Daily Show 2026-07-30.mkv",
				SeriesID:     "series-daily",
				PremiereDate: "2026-07-29",
				Overview:     "The previous night's episode.",
			},
			series:    &Item{ID: "series-daily", Type: "Series", ProviderIDs: map[string]string{"Tvdb": "71256"}},
			wantState: MetadataStateSeriesIdentifiedEpisodeStale,
		},
		{
			// The item carries its own provider IDs but has no episode
			// numbers. Date validation is only meaningful against an
			// identified parent; without one the premiere date is anchored to
			// nothing, so this must stay a review state.
			name: "date-based episode with unidentified parent is not validated",
			row: dateEpisodeDecision(
				"/media/The Daily Show/The Daily Show 2026-07-29.mkv",
				oldTargetAt, 2026, 729,
			),
			item: &Item{
				ID:           "episode-orphan-parent",
				Type:         "Episode",
				Path:         "/media/The Daily Show/The Daily Show 2026-07-29.mkv",
				SeriesID:     "series-daily",
				ProviderIDs:  map[string]string{"Tvdb": "99887"},
				PremiereDate: "2026-07-29",
				Overview:     "Guest episode.",
			},
			series:    &Item{ID: "series-daily", Type: "Series"},
			wantState: MetadataStateMissingEpisodeNumbers,
		},
		{
			name: "date-based episode with identified parent validates without episode numbers",
			row: dateEpisodeDecision(
				"/media/The Daily Show/The Daily Show 2026-07-29.mkv",
				oldTargetAt, 2026, 729,
			),
			item: &Item{
				ID:           "episode-own-ids",
				Type:         "Episode",
				Path:         "/media/The Daily Show/The Daily Show 2026-07-29.mkv",
				SeriesID:     "series-daily",
				ProviderIDs:  map[string]string{"Tvdb": "99887"},
				PremiereDate: "2026-07-29",
				Overview:     "Guest episode.",
			},
			series:     &Item{ID: "series-daily", Type: "Series", ProviderIDs: map[string]string{"Tvdb": "71256"}},
			wantState:  MetadataStateIdentified,
			identified: true,
		},
		{
			name: "indexless episode without date evidence stays stale",
			row:  decisionWithTarget("/media/show/s01e03.mkv", oldTargetAt),
			item: &Item{
				ID:           "episode-stale",
				Type:         "Episode",
				Path:         "/media/show/s01e03.mkv",
				SeriesID:     "series-1",
				PremiereDate: "2026-01-15T00:00:00Z",
				Overview:     "Has metadata but no date parse.",
			},
			series:    &Item{ID: "series-1", Type: "Series", ProviderIDs: map[string]string{"Tvdb": "789"}},
			wantState: MetadataStateSeriesIdentifiedEpisodeStale,
		},
		{
			name: "date-based episode with mismatched premiere stays stale",
			row: dateEpisodeDecision(
				"/media/The Daily Show/The Daily Show 2026-07-29.mkv",
				oldTargetAt, 2026, 729,
			),
			item: &Item{
				ID:           "episode-mismatch",
				Type:         "Episode",
				Path:         "/media/The Daily Show/The Daily Show 2026-07-29.mkv",
				SeriesID:     "series-daily",
				PremiereDate: "2026-06-01T00:00:00Z",
				Overview:     "Wrong date.",
			},
			series:    &Item{ID: "series-daily", Type: "Series", ProviderIDs: map[string]string{"Tvdb": "71256"}},
			wantState: MetadataStateSeriesIdentifiedEpisodeStale,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ClassifyMetadata(tt.row, tt.item, tt.series, now)

			assert.Equal(t, tt.wantState, got.State)
			assert.Equal(t, tt.identified, got.Identified)
		})
	}
}

func dateEpisodeDecision(path string, targetAt time.Time, season, episode int) *database.ParseDecision {
	s, e := season, episode
	return &database.ParseDecision{
		TargetPath:    path,
		TargetAt:      &targetAt,
		ParsedSeason:  &s,
		ParsedEpisode: &e,
	}
}

func TestHasProviderIDs(t *testing.T) {
	t.Parallel()

	assert.False(t, HasProviderIDs(nil))
	assert.False(t, HasProviderIDs(&Item{ProviderIDs: map[string]string{"Tmdb": ""}}))
	assert.True(t, HasProviderIDs(&Item{ProviderIDs: map[string]string{"Tmdb": "123"}}))
}

func TestHasEpisodeNumbers(t *testing.T) {
	t.Parallel()

	episodeNumber := 3
	seasonNumber := 1

	assert.False(t, HasEpisodeNumbers(nil))
	assert.False(t, HasEpisodeNumbers(&Item{IndexNumber: &episodeNumber}))
	assert.True(t, HasEpisodeNumbers(&Item{IndexNumber: &episodeNumber, ParentIndexNumber: &seasonNumber}))
}

func decisionWithTarget(path string, targetAt time.Time) *database.ParseDecision {
	return &database.ParseDecision{
		TargetPath: path,
		TargetAt:   &targetAt,
	}
}

func TestMetadataReconcilerPassiveUpgradesLateIdentifiedItem(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	targetAt := now.Add(-30 * time.Minute)
	episode := 2
	season := 1
	row := &database.ParseDecision{
		ID:                 10,
		ParsedTitle:        "Maximum Pleasure Guaranteed",
		TargetPath:         "/mnt/STORAGE4/TV/Maximum Pleasure Guaranteed/Season 01/file.mkv",
		TargetAt:           &targetAt,
		OrganizeOutcome:    "success",
		JellyfinItemID:     "episode-1",
		JellyfinResolvedAt: &targetAt,
		JellyfinIdentified: boolPtr(false),
	}
	store := &metadataStoreFake{due: []*database.ParseDecision{row}}
	client := &metadataClientFake{items: map[string]Item{
		"episode-1": {
			ID:                "episode-1",
			Type:              "Episode",
			Path:              "/tv4/Maximum Pleasure Guaranteed/Season 01/file.mkv",
			SeriesID:          "series-1",
			ProviderIDs:       map[string]string{"Tvdb": "12345"},
			IndexNumber:       &episode,
			ParentIndexNumber: &season,
		},
		"series-1": {ID: "series-1", Type: "Series", ProviderIDs: map[string]string{"Tvdb": "999"}},
	}}
	reconciler := NewMetadataReconciler(client, store, MetadataRecoveryConfig{})
	reconciler.now = func() time.Time { return now }
	reconciler.SetPathTranslator(NewPathTranslator([]PathMapping{{Jellyfin: "/tv4", Daemon: "/mnt/STORAGE4/TV"}}))

	summary, err := reconciler.RunPassive(context.Background(), 25, nil)

	require.NoError(t, err)
	assert.Equal(t, 1, summary.Checked)
	assert.Equal(t, 1, summary.Identified)
	require.Len(t, store.outcomes, 1)
	assert.Equal(t, "12345", store.outcomes[0].JellyfinTvdbID)
	require.Len(t, store.checks, 1)
	assert.Equal(t, MetadataStateIdentified, store.checks[0].state)
	assert.Nil(t, store.checks[0].next)
}

func TestMetadataReconcilerPassiveRecoversStaleItemIDByTargetPath(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	targetAt := now.Add(-30 * time.Minute)
	row := &database.ParseDecision{
		ID:                 14,
		ParsedTitle:        "Is This Thing On",
		TargetPath:         "/mnt/STORAGE1/MOVIES/Is This Thing On (2025)/Is This Thing On (2025).mp4",
		TargetAt:           &targetAt,
		OrganizeOutcome:    "success",
		JellyfinItemID:     "stale-missing-id",
		JellyfinResolvedAt: &targetAt,
		JellyfinIdentified: boolPtr(false),
	}
	store := &metadataStoreFake{due: []*database.ParseDecision{row}}
	client := &metadataClientFake{
		items: map[string]Item{},
		pages: []*ItemsResponse{{
			Items: []Item{{
				ID:          "current-movie-id",
				Type:        "Movie",
				Path:        "/movies1/Is This Thing On (2025)/Is This Thing On (2025).mp4",
				ProviderIDs: map[string]string{"Tmdb": "12345", "Imdb": "tt999"},
			}},
			TotalRecordCount: 1,
		}},
	}
	reconciler := NewMetadataReconciler(client, store, MetadataRecoveryConfig{})
	reconciler.now = func() time.Time { return now }
	reconciler.SetPathTranslator(NewPathTranslator([]PathMapping{{Jellyfin: "/movies1", Daemon: "/mnt/STORAGE1/MOVIES"}}))

	summary, err := reconciler.RunPassive(context.Background(), 25, nil)

	require.NoError(t, err)
	assert.Equal(t, 1, summary.Checked)
	assert.Equal(t, 1, summary.Identified)
	require.Len(t, store.outcomes, 1)
	assert.Equal(t, "current-movie-id", store.outcomes[0].JellyfinItemID)
	assert.Equal(t, "12345", store.outcomes[0].JellyfinTmdbID)
	require.Len(t, store.checks, 1)
	assert.Equal(t, MetadataStateIdentified, store.checks[0].state)
}

func TestMetadataReconcilerPassiveClassifiesMissingTargetFile(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	targetAt := now.Add(-30 * time.Minute)
	row := &database.ParseDecision{
		ID:                 15,
		ParsedTitle:        "Dust Bunny",
		TargetPath:         filepath.Join(t.TempDir(), "Dust Bunny (2025)", "Dust Bunny (2025).mkv"),
		TargetAt:           &targetAt,
		OrganizeOutcome:    "success",
		JellyfinItemID:     "missing-id",
		JellyfinResolvedAt: &targetAt,
		JellyfinIdentified: boolPtr(false),
	}
	store := &metadataStoreFake{due: []*database.ParseDecision{row}}
	client := &metadataClientFake{}
	reconciler := NewMetadataReconciler(client, store, MetadataRecoveryConfig{})
	reconciler.now = func() time.Time { return now }

	summary, err := reconciler.RunPassive(context.Background(), 25, nil)

	require.NoError(t, err)
	assert.Equal(t, 1, summary.Checked)
	assert.Equal(t, 0, summary.Identified)
	require.Len(t, store.checks, 1)
	assert.Equal(t, MetadataStateTargetFileMissing, store.checks[0].state)
	assert.Equal(t, "target file is missing", store.checks[0].errMsg)
}

func TestMetadataReconcilerRepairQueuesSeriesRefreshForStaleEpisode(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	targetAt := now.Add(-30 * time.Minute)
	row := &database.ParseDecision{
		ID:                 11,
		ParsedTitle:        "Maximum Pleasure Guaranteed",
		TargetPath:         "/mnt/STORAGE4/TV/Maximum Pleasure Guaranteed/Season 01/file.mkv",
		TargetAt:           &targetAt,
		OrganizeOutcome:    "success",
		JellyfinItemID:     "episode-1",
		JellyfinResolvedAt: &targetAt,
		JellyfinIdentified: boolPtr(false),
	}
	store := &metadataStoreFake{due: []*database.ParseDecision{row}}
	client := &metadataClientFake{items: map[string]Item{
		"episode-1": {
			ID:       "episode-1",
			Type:     "Episode",
			Path:     "/mnt/STORAGE4/TV/Maximum Pleasure Guaranteed/Season 01/file.mkv",
			SeriesID: "series-1",
		},
		"series-1": {ID: "series-1", Type: "Series", ProviderIDs: map[string]string{"Tvdb": "999"}},
	}}
	reconciler := NewMetadataReconciler(client, store, MetadataRecoveryConfig{RepairCooldown: 6 * time.Hour})
	reconciler.now = func() time.Time { return now }

	summary, err := reconciler.RunRepair(context.Background(), 5, nil)

	require.NoError(t, err)
	assert.Equal(t, 1, summary.Checked)
	assert.Equal(t, 1, summary.Repaired)
	assert.Empty(t, client.refreshes)
	assert.Equal(t, []string{"series-1"}, client.recursiveRefreshes)
	require.Len(t, store.repairs, 1)
	assert.Equal(t, MetadataStateSeriesIdentifiedEpisodeStale, store.repairs[0].state)
	require.NotNil(t, store.repairs[0].next)
	assert.Equal(t, now.Add(metadataInitialWait), *store.repairs[0].next)
}

func TestMetadataReconcilerRepairQueuesSeriesRefreshWhenParentSeriesUnidentified(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	targetAt := now.Add(-30 * time.Minute)
	row := &database.ParseDecision{
		ID:                 13,
		ParsedTitle:        "Spider Noir",
		TargetPath:         "/mnt/STORAGE10/TV/Spider Noir/Season 01/file.mkv",
		TargetAt:           &targetAt,
		OrganizeOutcome:    "success",
		JellyfinItemID:     "episode-1",
		JellyfinResolvedAt: &targetAt,
		JellyfinIdentified: boolPtr(false),
	}
	store := &metadataStoreFake{due: []*database.ParseDecision{row}}
	client := &metadataClientFake{items: map[string]Item{
		"episode-1": {
			ID:       "episode-1",
			Type:     "Episode",
			Path:     "/mnt/STORAGE10/TV/Spider Noir/Season 01/file.mkv",
			SeriesID: "series-1",
		},
		"series-1": {ID: "series-1", Type: "Series"},
	}}
	reconciler := NewMetadataReconciler(client, store, MetadataRecoveryConfig{RepairCooldown: 6 * time.Hour})
	reconciler.now = func() time.Time { return now }

	summary, err := reconciler.RunRepair(context.Background(), 5, nil)

	require.NoError(t, err)
	assert.Equal(t, 1, summary.Checked)
	assert.Equal(t, 1, summary.Repaired)
	assert.Empty(t, client.refreshes)
	assert.Equal(t, []string{"series-1"}, client.recursiveRefreshes)
	require.Len(t, store.repairs, 1)
	assert.Equal(t, MetadataStateSeriesUnidentified, store.repairs[0].state)
	require.NotNil(t, store.repairs[0].next)
	assert.Equal(t, now.Add(metadataInitialWait), *store.repairs[0].next)
}

func TestMetadataReconcilerRepairDedupesSeriesRefreshesWithinBatch(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	targetAt := now.Add(-30 * time.Minute)
	rows := []*database.ParseDecision{
		{
			ID:                 21,
			ParsedTitle:        "Bad Thoughts",
			TargetPath:         "/mnt/STORAGE5/TV/Bad Thoughts/Season 02/e01.mkv",
			TargetAt:           &targetAt,
			OrganizeOutcome:    "success",
			JellyfinItemID:     "episode-1",
			JellyfinResolvedAt: &targetAt,
			JellyfinIdentified: boolPtr(false),
		},
		{
			ID:                 22,
			ParsedTitle:        "Bad Thoughts",
			TargetPath:         "/mnt/STORAGE5/TV/Bad Thoughts/Season 02/e02.mkv",
			TargetAt:           &targetAt,
			OrganizeOutcome:    "success",
			JellyfinItemID:     "episode-2",
			JellyfinResolvedAt: &targetAt,
			JellyfinIdentified: boolPtr(false),
		},
	}
	store := &metadataStoreFake{due: rows}
	client := &metadataClientFake{items: map[string]Item{
		"episode-1": {ID: "episode-1", Type: "Episode", Path: "/mnt/STORAGE5/TV/Bad Thoughts/Season 02/e01.mkv", SeriesID: "series-1"},
		"episode-2": {ID: "episode-2", Type: "Episode", Path: "/mnt/STORAGE5/TV/Bad Thoughts/Season 02/e02.mkv", SeriesID: "series-1"},
		"series-1":  {ID: "series-1", Type: "Series", ProviderIDs: map[string]string{"Tvdb": "999"}},
	}}
	reconciler := NewMetadataReconciler(client, store, MetadataRecoveryConfig{RepairCooldown: 6 * time.Hour})
	reconciler.now = func() time.Time { return now }

	summary, err := reconciler.RunRepair(context.Background(), 5, nil)

	require.NoError(t, err)
	assert.Equal(t, 2, summary.Checked)
	assert.Equal(t, 2, summary.Repaired)
	assert.Empty(t, client.refreshes)
	assert.Equal(t, []string{"series-1"}, client.recursiveRefreshes)
	require.Len(t, store.repairs, 2)
	for _, repair := range store.repairs {
		assert.Equal(t, MetadataStateSeriesIdentifiedEpisodeStale, repair.state)
		assert.Equal(t, "full metadata refresh queued", repair.errMsg)
		require.NotNil(t, repair.repairedAt)
	}
}

func TestMetadataReconcilerRepairRespectsCooldown(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	targetAt := now.Add(-30 * time.Minute)
	lastRepair := now.Add(-1 * time.Hour)
	row := &database.ParseDecision{
		ID:                   12,
		TargetPath:           "/media/file.mkv",
		TargetAt:             &targetAt,
		JellyfinItemID:       "movie-1",
		JellyfinResolvedAt:   &targetAt,
		JellyfinIdentified:   boolPtr(false),
		LastMetadataRepairAt: &lastRepair,
	}
	store := &metadataStoreFake{due: []*database.ParseDecision{row}}
	client := &metadataClientFake{items: map[string]Item{
		"movie-1": {ID: "movie-1", Type: "Movie", Path: "/media/file.mkv"},
	}}
	reconciler := NewMetadataReconciler(client, store, MetadataRecoveryConfig{RepairCooldown: 6 * time.Hour})
	reconciler.now = func() time.Time { return now }

	summary, err := reconciler.RunRepair(context.Background(), 5, nil)

	require.NoError(t, err)
	assert.Equal(t, 1, summary.Checked)
	assert.Equal(t, 1, summary.Skipped)
	assert.Empty(t, client.refreshes)
	require.Len(t, store.repairs, 1)
	require.NotNil(t, store.repairs[0].next)
	assert.Equal(t, lastRepair.Add(6*time.Hour), *store.repairs[0].next)
}

type metadataClientFake struct {
	items              map[string]Item
	pages              []*ItemsResponse
	refreshes          []string
	recursiveRefreshes []string
}

func (f *metadataClientFake) GetItemsByIDs(ctx context.Context, ids []string) (*ItemsResponse, error) {
	resp := &ItemsResponse{}
	for _, id := range ids {
		if item, ok := f.items[id]; ok {
			resp.Items = append(resp.Items, item)
		}
	}
	resp.TotalRecordCount = len(resp.Items)
	return resp, nil
}

func (f *metadataClientFake) ListItemsPageCtx(ctx context.Context, startIndex, limit int) (*ItemsResponse, error) {
	if len(f.pages) == 0 {
		return &ItemsResponse{}, nil
	}
	page := f.pages[0]
	f.pages = f.pages[1:]
	return page, nil
}

func (f *metadataClientFake) RefreshItemFullMetadata(itemID string) error {
	f.refreshes = append(f.refreshes, itemID)
	return nil
}

func (f *metadataClientFake) RefreshItemFullMetadataRecursive(itemID string) error {
	f.recursiveRefreshes = append(f.recursiveRefreshes, itemID)
	return nil
}

type metadataStoreFake struct {
	byID     map[int64]*database.ParseDecision
	due      []*database.ParseDecision
	outcomes []database.OutcomeUpdate
	checks   []metadataCheckWrite
	repairs  []metadataRepairWrite
}

func (f *metadataStoreFake) GetDecision(id int64) (*database.ParseDecision, error) {
	if f.byID != nil {
		return f.byID[id], nil
	}
	for _, row := range f.due {
		if row.ID == id {
			return row, nil
		}
	}
	return nil, nil
}

func (f *metadataStoreFake) ListDueMetadataChecks(now time.Time, limit int) ([]*database.ParseDecision, error) {
	return f.due, nil
}

func (f *metadataStoreFake) UpgradeOutcome(id int64, u database.OutcomeUpdate) error {
	f.outcomes = append(f.outcomes, u)
	return nil
}

func (f *metadataStoreFake) UpdateMetadataCheckState(id int64, state, errMsg string, nextCheck *time.Time) error {
	f.checks = append(f.checks, metadataCheckWrite{id: id, state: state, errMsg: errMsg, next: nextCheck})
	return nil
}

func (f *metadataStoreFake) UpdateMetadataRepairState(id int64, state, errMsg string, nextCheck *time.Time, repairedAt *time.Time) error {
	f.repairs = append(f.repairs, metadataRepairWrite{id: id, state: state, errMsg: errMsg, next: nextCheck, repairedAt: repairedAt})
	return nil
}

type metadataCheckWrite struct {
	id     int64
	state  string
	errMsg string
	next   *time.Time
}

type metadataRepairWrite struct {
	id         int64
	state      string
	errMsg     string
	next       *time.Time
	repairedAt *time.Time
}

func boolPtr(v bool) *bool {
	return &v
}

type fakeCorrector struct {
	decision CorrectionDecision
	calls    []fakeCorrectorCall
}

type fakeCorrectorCall struct {
	title string
	year  string
}

func (f *fakeCorrector) Decide(ctx context.Context, currentTitle, year string) CorrectionDecision {
	f.calls = append(f.calls, fakeCorrectorCall{title: currentTitle, year: year})
	return f.decision
}

type fakeEnqueuer struct {
	calls []fakeEnqueuerCall
}

type fakeEnqueuerCall struct {
	parseDecisionID int64
	srcPath         string
	dstPath         string
	newTitle        string
	newYear         string
	tmdbID          string
}

func (f *fakeEnqueuer) EnqueueVerifierRename(parseDecisionID int64, srcPath, dstPath, newTitle, newYear, tmdbID string) error {
	f.calls = append(f.calls, fakeEnqueuerCall{parseDecisionID: parseDecisionID, srcPath: srcPath, dstPath: dstPath, newTitle: newTitle, newYear: newYear, tmdbID: tmdbID})
	return nil
}

func TestPassiveCorrectionEnqueuesAndConsumesAttempt(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	targetAt := now.Add(-72 * time.Hour) // past 48h grace
	year := 2023
	row := &database.ParseDecision{
		ID:                  42,
		ParsedTitle:         "Some Wrong Title",
		ParsedYear:          &year,
		MediaTypeGuessed:    "movie",
		TargetPath:          "/mnt/STORAGE1/MOVIES/Some Wrong Title (2023)/Some Wrong Title (2023).mkv",
		TargetAt:            &targetAt,
		OrganizeOutcome:     "success",
		JellyfinItemID:      "movie-1",
		JellyfinResolvedAt:  &targetAt,
		JellyfinIdentified:  boolPtr(false),
		MetadataRepairCount: 0,
	}
	store := &metadataStoreFake{due: []*database.ParseDecision{row}}
	client := &metadataClientFake{items: map[string]Item{
		"movie-1": {ID: "movie-1", Type: "Movie", Path: "/movies/Some Wrong Title (2023)/Some Wrong Title (2023).mkv"},
	}}
	reconciler := NewMetadataReconciler(client, store, MetadataRecoveryConfig{CorrectionEnabled: true, NeedsReviewAfter: 4})
	reconciler.now = func() time.Time { return now }
	reconciler.SetPathTranslator(NewPathTranslator([]PathMapping{{Jellyfin: "/movies", Daemon: "/mnt/STORAGE1/MOVIES"}}))

	corr := &fakeCorrector{decision: CorrectionDecision{
		Action:   "correct",
		NewTitle: "Correct Title",
		NewYear:  "2023",
		TmdbID:   "12345",
		Reason:   "matched via candidate strip",
	}}
	enq := &fakeEnqueuer{}
	reconciler.SetCorrector(corr)
	reconciler.SetEnqueuer(enq)

	summary, err := reconciler.RunPassive(context.Background(), 25, nil)

	require.NoError(t, err)
	assert.Equal(t, 1, summary.Checked)

	// Enqueued exactly once with the right args.
	require.Len(t, enq.calls, 1)
	assert.Equal(t, int64(42), enq.calls[0].parseDecisionID)
	assert.Equal(t, "Correct Title", enq.calls[0].newTitle)
	assert.Equal(t, "2023", enq.calls[0].newYear)
	assert.Equal(t, "12345", enq.calls[0].tmdbID)
	assert.Contains(t, enq.calls[0].srcPath, "Some Wrong Title (2023)")
	assert.Contains(t, enq.calls[0].dstPath, "Correct Title (2023)")
	assert.Contains(t, enq.calls[0].dstPath, "Correct Title (2023).mkv", "single dot before extension")
	assert.NotContains(t, enq.calls[0].dstPath, "..mkv")

	// One attempt consumed: UpdateMetadataRepairState called with non-nil repairedAt.
	require.Len(t, store.repairs, 1)
	assert.NotNil(t, store.repairs[0].repairedAt)

	// Corrector was asked about the current title.
	require.Len(t, corr.calls, 1)
	assert.Equal(t, "Some Wrong Title", corr.calls[0].title)
}

func TestPassiveCorrectionSkipsWhenCorrectorReturnsLeave(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	targetAt := now.Add(-72 * time.Hour)
	year := 2023
	row := &database.ParseDecision{
		ID:                  43,
		ParsedTitle:         "Some Title",
		ParsedYear:          &year,
		MediaTypeGuessed:    "movie",
		TargetPath:          "/mnt/STORAGE1/MOVIES/Some Title (2023)/Some Title (2023).mkv",
		TargetAt:            &targetAt,
		OrganizeOutcome:     "success",
		JellyfinItemID:      "movie-2",
		JellyfinResolvedAt:  &targetAt,
		JellyfinIdentified:  boolPtr(false),
		MetadataRepairCount: 0,
	}
	store := &metadataStoreFake{due: []*database.ParseDecision{row}}
	client := &metadataClientFake{items: map[string]Item{
		"movie-2": {ID: "movie-2", Type: "Movie", Path: "/movies/Some Title (2023)/Some Title (2023).mkv"},
	}}
	reconciler := NewMetadataReconciler(client, store, MetadataRecoveryConfig{CorrectionEnabled: true, NeedsReviewAfter: 4})
	reconciler.now = func() time.Time { return now }
	reconciler.SetPathTranslator(NewPathTranslator([]PathMapping{{Jellyfin: "/movies", Daemon: "/mnt/STORAGE1/MOVIES"}}))

	corr := &fakeCorrector{decision: CorrectionDecision{Action: "leave"}}
	enq := &fakeEnqueuer{}
	reconciler.SetCorrector(corr)
	reconciler.SetEnqueuer(enq)

	summary, err := reconciler.RunPassive(context.Background(), 25, nil)

	require.NoError(t, err)
	assert.Equal(t, 1, summary.Checked)
	assert.Empty(t, enq.calls)
	assert.Empty(t, store.repairs)
}

func TestPassiveCorrectionSkipsWithinGracePeriod(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	targetAt := now.Add(-10 * time.Minute) // within 48h grace
	year := 2023
	row := &database.ParseDecision{
		ID:                  44,
		ParsedTitle:         "Fresh Import",
		ParsedYear:          &year,
		MediaTypeGuessed:    "movie",
		TargetPath:          "/mnt/STORAGE1/MOVIES/Fresh Import (2023)/Fresh Import (2023).mkv",
		TargetAt:            &targetAt,
		OrganizeOutcome:     "success",
		JellyfinItemID:      "movie-3",
		JellyfinResolvedAt:  &targetAt,
		JellyfinIdentified:  boolPtr(false),
		MetadataRepairCount: 0,
	}
	store := &metadataStoreFake{due: []*database.ParseDecision{row}}
	client := &metadataClientFake{items: map[string]Item{
		"movie-3": {ID: "movie-3", Type: "Movie", Path: "/movies/Fresh Import (2023)/Fresh Import (2023).mkv"},
	}}
	reconciler := NewMetadataReconciler(client, store, MetadataRecoveryConfig{CorrectionEnabled: true, NeedsReviewAfter: 4})
	reconciler.now = func() time.Time { return now }
	reconciler.SetPathTranslator(NewPathTranslator([]PathMapping{{Jellyfin: "/movies", Daemon: "/mnt/STORAGE1/MOVIES"}}))

	corr := &fakeCorrector{decision: CorrectionDecision{Action: "correct", NewTitle: "Fixed", NewYear: "2023", TmdbID: "1"}}
	enq := &fakeEnqueuer{}
	reconciler.SetCorrector(corr)
	reconciler.SetEnqueuer(enq)

	_, err := reconciler.RunPassive(context.Background(), 25, nil)

	require.NoError(t, err)
	assert.Empty(t, enq.calls, "should not enqueue within grace period")
	assert.Empty(t, store.repairs)
}

func TestPassiveCorrectionSkipsAtAttemptCap(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	targetAt := now.Add(-72 * time.Hour)
	year := 2023
	row := &database.ParseDecision{
		ID:                  45,
		ParsedTitle:         "Capped Title",
		ParsedYear:          &year,
		MediaTypeGuessed:    "movie",
		TargetPath:          "/mnt/STORAGE1/MOVIES/Capped Title (2023)/Capped Title (2023).mkv",
		TargetAt:            &targetAt,
		OrganizeOutcome:     "success",
		JellyfinItemID:      "movie-4",
		JellyfinResolvedAt:  &targetAt,
		JellyfinIdentified:  boolPtr(false),
		MetadataRepairCount: 4, // == NeedsReviewAfter cap
	}
	store := &metadataStoreFake{due: []*database.ParseDecision{row}}
	client := &metadataClientFake{items: map[string]Item{
		"movie-4": {ID: "movie-4", Type: "Movie", Path: "/movies/Capped Title (2023)/Capped Title (2023).mkv"},
	}}
	reconciler := NewMetadataReconciler(client, store, MetadataRecoveryConfig{CorrectionEnabled: true, NeedsReviewAfter: 4})
	reconciler.now = func() time.Time { return now }
	reconciler.SetPathTranslator(NewPathTranslator([]PathMapping{{Jellyfin: "/movies", Daemon: "/mnt/STORAGE1/MOVIES"}}))

	corr := &fakeCorrector{decision: CorrectionDecision{Action: "correct", NewTitle: "Fixed", NewYear: "2023", TmdbID: "1"}}
	enq := &fakeEnqueuer{}
	reconciler.SetCorrector(corr)
	reconciler.SetEnqueuer(enq)

	_, err := reconciler.RunPassive(context.Background(), 25, nil)

	require.NoError(t, err)
	assert.Empty(t, enq.calls, "should not enqueue at attempt cap")
	assert.Empty(t, store.repairs)
}

// Jellyfin stores an air date as local midnight expressed as a UTC instant, so
// the calendar date must be read back in the local zone. Reading it as UTC
// shifts every episode a day earlier on any host east of UTC.
//
// The zoned fixtures are verbatim from a live Jellyfin 10.11.11 server running
// alongside a UTC+10 host: "…T14:00:00Z" is midnight Sydney the next day.
func TestParseJellyfinPremiereDate_LocalMidnightEncoding(t *testing.T) {
	orig := time.Local
	t.Cleanup(func() { time.Local = orig })

	tests := []struct {
		name string
		zone *time.Location
		raw  string
		want string
	}{
		{"UTC+10 local midnight", time.FixedZone("AEST", 10*3600), "2026-07-14T14:00:00.0000000Z", "2026-07-15"},
		{"UTC+10 local midnight RFC3339", time.FixedZone("AEST", 10*3600), "2026-07-29T14:00:00Z", "2026-07-30"},
		{"UTC host reads the same instant as written", time.UTC, "2026-07-14T14:00:00.0000000Z", "2026-07-14"},
		{"UTC-5 host", time.FixedZone("EST", -5*3600), "2026-07-15T04:00:00.0000000Z", "2026-07-14"},
		{"explicit offset is honoured", time.FixedZone("AEST", 10*3600), "2026-07-15T00:00:00+10:00", "2026-07-15"},
		// Zoneless layouts are already wall-clock dates and must not shift.
		{"date only is taken as written", time.FixedZone("AEST", 10*3600), "2026-07-15", "2026-07-15"},
		{"zoneless timestamp is taken as written", time.FixedZone("AEST", 10*3600), "2026-07-15T09:30:00", "2026-07-15"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			time.Local = tc.zone
			got, ok := parseJellyfinPremiereDate(tc.raw)
			require.True(t, ok, "parse %q", tc.raw)
			assert.Equal(t, tc.want, got.Format("2006-01-02"))
		})
	}
}

// The live Daily Show rows: a UTC+10 host must validate the file whose name
// matches the local air date, and still reject its neighbours.
func TestIsValidatedDateEpisode_ProductionDailyShowFixture(t *testing.T) {
	orig := time.Local
	t.Cleanup(func() { time.Local = orig })
	time.Local = time.FixedZone("AEST", 10*3600)

	targetAt := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	item := &Item{
		ID:           "episode-daily-prod",
		Type:         "Episode",
		Path:         "/mnt/STORAGE5/TVSHOWS/The Daily Show (1996)/Season 2026/The Daily Show 2026-07-30.mkv",
		SeriesID:     "series-daily",
		PremiereDate: "2026-07-29T14:00:00.0000000Z", // midnight AEST on 2026-07-30
		Overview:     "Daily Show episode.",
	}

	row := dateEpisodeDecision(item.Path, targetAt, 2026, 730)
	assert.True(t, isValidatedDateEpisode(row, item), "the matching air date must validate")

	prev := dateEpisodeDecision(item.Path, targetAt, 2026, 729)
	assert.False(t, isValidatedDateEpisode(prev, item), "the previous day's file must not validate")

	next := dateEpisodeDecision(item.Path, targetAt, 2026, 731)
	assert.False(t, isValidatedDateEpisode(next, item), "the next day's file must not validate")
}
