package database

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeDecision() ParseDecision {
	return ParseDecision{
		SourcePath:     "/media/downloads/show.S01E01.mkv",
		SourceFilename: "show.S01E01.mkv",
		EventAt:        time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
	}
}

func TestParseDecisionInsertGet(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	d := makeDecision()
	id, err := db.InsertDecision(d)
	require.NoError(t, err)
	require.Greater(t, id, int64(0))

	got, err := db.GetDecision(id)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, id, got.ID)
	assert.Equal(t, d.SourcePath, got.SourcePath)
	assert.Equal(t, d.SourceFilename, got.SourceFilename)
	assert.True(t, d.EventAt.Equal(got.EventAt))
	assert.Equal(t, "", got.ParseMethod)
	assert.Nil(t, got.ParsedYear)
	assert.Nil(t, got.ParsedSeason)
	assert.Nil(t, got.ParsedEpisode)
	assert.Equal(t, "", got.MetadataState)
	assert.Equal(t, 0, got.MetadataCheckCount)
	assert.Equal(t, 0, got.MetadataRepairCount)
}

func TestParseDecisionsMetadataRecoveryColumns(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	rows, err := db.SQL().Query(`PRAGMA table_info(parse_decisions)`)
	require.NoError(t, err)
	defer rows.Close()

	columns := map[string]bool{}
	for rows.Next() {
		var (
			cid        int
			name       string
			columnType string
			notNull    int
			defaultVal any
			pk         int
		)
		require.NoError(t, rows.Scan(&cid, &name, &columnType, &notNull, &defaultVal, &pk))
		columns[name] = true
	}
	require.NoError(t, rows.Err())

	for _, name := range []string{
		"metadata_state",
		"metadata_error",
		"metadata_check_count",
		"metadata_repair_count",
		"last_metadata_check_at",
		"next_metadata_check_at",
		"last_metadata_repair_at",
	} {
		assert.True(t, columns[name], "missing column %s", name)
	}
}

func TestParseDecisionGetNotFound(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	got, err := db.GetDecision(9999)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestParseDecisionUpdateParse(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	id, err := db.InsertDecision(makeDecision())
	require.NoError(t, err)

	year := 2023
	season := 1
	episode := 2
	u := ParseUpdate{
		ParseMethod:          "regex",
		ParsedTitle:          "My Show",
		ParsedYear:           &year,
		ParsedSeason:         &season,
		ParsedEpisode:        &episode,
		ParserStrippedTokens: `["1080p","WEB-DL"]`,
		MediaTypeGuessed:     "tv",
	}
	require.NoError(t, db.UpdateParse(id, u))

	got, err := db.GetDecision(id)
	require.NoError(t, err)
	assert.Equal(t, "regex", got.ParseMethod)
	assert.Equal(t, "My Show", got.ParsedTitle)
	require.NotNil(t, got.ParsedYear)
	assert.Equal(t, 2023, *got.ParsedYear)
	require.NotNil(t, got.ParsedSeason)
	assert.Equal(t, 1, *got.ParsedSeason)
	require.NotNil(t, got.ParsedEpisode)
	assert.Equal(t, 2, *got.ParsedEpisode)
	assert.Equal(t, `["1080p","WEB-DL"]`, got.ParserStrippedTokens)
	assert.Equal(t, "tv", got.MediaTypeGuessed)
}

func TestParseDecisionUpdateOrganize(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	id, err := db.InsertDecision(makeDecision())
	require.NoError(t, err)

	now := time.Now().UTC().Truncate(time.Second)
	u := OrganizeUpdate{
		TargetPath:          "/library/TV/show/S01E01.mkv",
		TargetAt:            &now,
		ExistingMatchMethod: "none",
		OrganizeOutcome:     "success",
		OrganizeError:       "",
	}
	require.NoError(t, db.UpdateOrganize(id, u))

	got, err := db.GetDecision(id)
	require.NoError(t, err)
	assert.Equal(t, "/library/TV/show/S01E01.mkv", got.TargetPath)
	assert.Equal(t, "success", got.OrganizeOutcome)
	assert.Equal(t, "", got.OrganizeError)
	require.NotNil(t, got.TargetAt)
	assert.True(t, now.Equal(*got.TargetAt))
}

func TestParseDecisionUpdateOrganizeFailed(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	id, err := db.InsertDecision(makeDecision())
	require.NoError(t, err)

	u := OrganizeUpdate{
		OrganizeOutcome: "failed",
		OrganizeError:   "no match found",
	}
	require.NoError(t, db.UpdateOrganize(id, u))

	got, err := db.GetDecision(id)
	require.NoError(t, err)
	assert.Equal(t, "failed", got.OrganizeOutcome)
	assert.Equal(t, "no match found", got.OrganizeError)
}

func TestGetRecentDeterministicFailures_IncludesSeasonPackSkipped(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	d := makeDecision()
	d.SourcePath = "/downloads/tv/Supergirl.S03.1080p.BluRay.x264-YELLOWBiRD/Supergirl.S03.1080p.BluRay.x264-YELLOWBiRD.mkv"
	d.SourceFilename = "Supergirl.S03.1080p.BluRay.x264-YELLOWBiRD.mkv"
	d.EventAt = time.Now().UTC()
	d.ParseMethod = "season_pack"
	d.MediaTypeGuessed = "tv"
	d.OrganizeOutcome = "skipped"
	d.OrganizeError = "season_pack_unresolved: /downloads/tv/Supergirl.S03.1080p.BluRay.x264-YELLOWBiRD"
	_, err := db.InsertDecision(d)
	require.NoError(t, err)

	rows, err := db.GetRecentDeterministicFailures(7 * 24 * time.Hour)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, d.SourcePath, rows[0].SourcePath)
	assert.Equal(t, 1, rows[0].Failures)
	assert.Equal(t, d.OrganizeError, rows[0].LastError)
	assert.False(t, rows[0].LastAt.IsZero())
}

func TestParseDecisionUpdateOutcome(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	id, err := db.InsertDecision(makeDecision())
	require.NoError(t, err)

	now := time.Now().UTC().Truncate(time.Second)
	u := OutcomeUpdate{
		JellyfinItemID:     "item-123",
		JellyfinImdbID:     "tt1234567",
		JellyfinTmdbID:     "tmdb-456",
		JellyfinTvdbID:     "tvdb-789",
		JellyfinResolvedAt: &now,
	}
	require.NoError(t, db.UpdateOutcome(id, u))

	got, err := db.GetDecision(id)
	require.NoError(t, err)
	assert.Equal(t, "item-123", got.JellyfinItemID)
	assert.Equal(t, "tt1234567", got.JellyfinImdbID)
	assert.Equal(t, "tmdb-456", got.JellyfinTmdbID)
	assert.Equal(t, "tvdb-789", got.JellyfinTvdbID)
	require.NotNil(t, got.JellyfinResolvedAt)
	assert.True(t, now.Equal(*got.JellyfinResolvedAt))
}

func TestParseDecisionUpdateAutoLabel(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	id, err := db.InsertDecision(makeDecision())
	require.NoError(t, err)

	require.NoError(t, db.UpdateAutoLabel(id, "mislabeled"))

	got, err := db.GetDecision(id)
	require.NoError(t, err)
	assert.Equal(t, "mislabeled", got.AutoLabel)
}

func TestParseDecisionUpdateHumanOverride(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	id, err := db.InsertDecision(makeDecision())
	require.NoError(t, err)

	require.NoError(t, db.UpdateHumanOverride(id, "correct"))

	got, err := db.GetDecision(id)
	require.NoError(t, err)
	assert.Equal(t, "correct", got.HumanLabelOverride)
}

func TestListDueMetadataChecks(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	now := time.Now().UTC().Truncate(time.Second)

	dueID, err := db.InsertDecision(makeDecision())
	require.NoError(t, err)
	require.NoError(t, db.UpdateOrganize(dueID, OrganizeUpdate{
		TargetPath:      "/library/show/Season 01/show S01E02.mkv",
		TargetAt:        &now,
		OrganizeOutcome: "success",
	}))
	identified := false
	require.NoError(t, db.UpdateOutcome(dueID, OutcomeUpdate{
		JellyfinItemID:      "item-due",
		JellyfinResolvedAt:  &now,
		JellyfinIdentified:  &identified,
		JellyfinFirstSeenAt: &now,
	}))

	futureID, err := db.InsertDecision(makeDecision())
	require.NoError(t, err)
	require.NoError(t, db.UpdateOrganize(futureID, OrganizeUpdate{
		TargetPath:      "/library/show/Season 01/show S01E03.mkv",
		TargetAt:        &now,
		OrganizeOutcome: "success",
	}))
	require.NoError(t, db.UpdateOutcome(futureID, OutcomeUpdate{
		JellyfinItemID:      "item-future",
		JellyfinResolvedAt:  &now,
		JellyfinIdentified:  &identified,
		JellyfinFirstSeenAt: &now,
	}))
	future := now.Add(time.Hour)
	require.NoError(t, db.UpdateMetadataCheckState(futureID, "recent_import_waiting", "", &future))

	identifiedID, err := db.InsertDecision(makeDecision())
	require.NoError(t, err)
	require.NoError(t, db.UpdateOrganize(identifiedID, OrganizeUpdate{
		TargetPath:      "/library/show/Season 01/show S01E01.mkv",
		TargetAt:        &now,
		OrganizeOutcome: "success",
	}))
	isIdentified := true
	require.NoError(t, db.UpdateOutcome(identifiedID, OutcomeUpdate{
		JellyfinItemID:      "item-identified",
		JellyfinResolvedAt:  &now,
		JellyfinIdentified:  &isIdentified,
		JellyfinFirstSeenAt: &now,
	}))

	rows, err := db.ListDueMetadataChecks(now, 25)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, dueID, rows[0].ID)
}

func TestUpdateMetadataCheckState(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	id, err := db.InsertDecision(makeDecision())
	require.NoError(t, err)

	next := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	require.NoError(t, db.UpdateMetadataCheckState(id, "missing_episode_numbers", "waiting", &next))

	got, err := db.GetDecision(id)
	require.NoError(t, err)
	assert.Equal(t, "missing_episode_numbers", got.MetadataState)
	assert.Equal(t, "waiting", got.MetadataError)
	assert.Equal(t, 1, got.MetadataCheckCount)
	require.NotNil(t, got.LastMetadataCheckAt)
	require.NotNil(t, got.NextMetadataCheckAt)
	assert.True(t, next.Equal(*got.NextMetadataCheckAt))
}

func TestUpdateMetadataRepairState(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	id, err := db.InsertDecision(makeDecision())
	require.NoError(t, err)

	next := time.Now().UTC().Add(6 * time.Hour).Truncate(time.Second)
	repairedAt := time.Now().UTC().Truncate(time.Second)
	require.NoError(t, db.UpdateMetadataRepairState(id, "identified", "", &next, &repairedAt))

	got, err := db.GetDecision(id)
	require.NoError(t, err)
	assert.Equal(t, "identified", got.MetadataState)
	assert.Equal(t, 1, got.MetadataRepairCount)
	require.NotNil(t, got.LastMetadataRepairAt)
	assert.True(t, repairedAt.Equal(*got.LastMetadataRepairAt))

	require.NoError(t, db.UpdateMetadataRepairState(id, "needs_review", "still stale", &next, nil))
	got, err = db.GetDecision(id)
	require.NoError(t, err)
	assert.Equal(t, "needs_review", got.MetadataState)
	assert.Equal(t, "still stale", got.MetadataError)
	assert.Equal(t, 1, got.MetadataRepairCount)
}

func TestQueryDecisions_OrganizeOutcome(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	d1 := makeDecision()
	d1.SourceFilename = "success.mkv"
	id1, err := db.InsertDecision(d1)
	require.NoError(t, err)
	require.NoError(t, db.UpdateOrganize(id1, OrganizeUpdate{OrganizeOutcome: "success"}))

	d2 := makeDecision()
	d2.SourceFilename = "failed.mkv"
	id2, err := db.InsertDecision(d2)
	require.NoError(t, err)
	require.NoError(t, db.UpdateOrganize(id2, OrganizeUpdate{OrganizeOutcome: "failed", OrganizeError: "no match"}))

	results, err := db.QueryDecisions(QueryFilter{OrganizeOutcome: "success"})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, id1, results[0].ID)
}

func TestQueryRecentSuccessfulMovieImports(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	now := time.Now().UTC().Truncate(time.Second)
	targetAt := now

	recentMovie := makeDecision()
	recentMovie.SourcePath = "/watch/movies/Mortal.Kombat.II.2026.1080p.WEB-DL.mp4"
	recentMovie.SourceFilename = "Mortal.Kombat.II.2026.1080p.WEB-DL.mp4"
	recentMovie.EventAt = now
	recentMovie.MediaTypeGuessed = "movie"
	id, err := db.InsertDecision(recentMovie)
	require.NoError(t, err)
	require.NoError(t, db.UpdateOrganize(id, OrganizeUpdate{
		TargetPath:      "/library/MOVIES/Mortal Kombat (2026)/Mortal Kombat (2026).mp4",
		TargetAt:        &targetAt,
		OrganizeOutcome: "success",
	}))

	oldMovie := makeDecision()
	oldMovie.SourcePath = "/watch/movies/Old.Movie.2020.mkv"
	oldMovie.SourceFilename = "Old.Movie.2020.mkv"
	oldMovie.EventAt = now.Add(-10 * 24 * time.Hour)
	oldMovie.MediaTypeGuessed = "movie"
	oldID, err := db.InsertDecision(oldMovie)
	require.NoError(t, err)
	require.NoError(t, db.UpdateOrganize(oldID, OrganizeUpdate{
		TargetPath:      "/library/MOVIES/Old Movie (2020)/Old Movie (2020).mkv",
		TargetAt:        &targetAt,
		OrganizeOutcome: "success",
	}))

	tv := makeDecision()
	tv.SourceFilename = "Show.S01E01.mkv"
	tv.EventAt = now
	tv.MediaTypeGuessed = "tv"
	tvID, err := db.InsertDecision(tv)
	require.NoError(t, err)
	require.NoError(t, db.UpdateOrganize(tvID, OrganizeUpdate{
		TargetPath:      "/library/TV/Show/Season 01/Show - S01E01.mkv",
		TargetAt:        &targetAt,
		OrganizeOutcome: "success",
	}))

	failedMovie := makeDecision()
	failedMovie.SourceFilename = "Failed.Movie.2026.mkv"
	failedMovie.EventAt = now
	failedMovie.MediaTypeGuessed = "movie"
	failedID, err := db.InsertDecision(failedMovie)
	require.NoError(t, err)
	require.NoError(t, db.UpdateOrganize(failedID, OrganizeUpdate{
		OrganizeOutcome: "failed",
		OrganizeError:   "boom",
	}))

	rows, err := db.QueryRecentSuccessfulMovieImports(7*24*time.Hour, 50)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, id, rows[0].ID)
	assert.Equal(t, recentMovie.SourceFilename, rows[0].SourceFilename)
	assert.Equal(t, "/library/MOVIES/Mortal Kombat (2026)/Mortal Kombat (2026).mp4", rows[0].TargetPath)
}

func TestQueryRecentSuccessfulTVImports(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	now := time.Now().UTC().Truncate(time.Second)
	targetAt := now

	recentTV := makeDecision()
	recentTV.SourcePath = "/watch/tv/Upload.S04E03.2025.1080p.WEB-DL.mkv"
	recentTV.SourceFilename = "Upload.S04E03.2025.1080p.WEB-DL.mkv"
	recentTV.EventAt = now
	recentTV.MediaTypeGuessed = "tv"
	id, err := db.InsertDecision(recentTV)
	require.NoError(t, err)
	require.NoError(t, db.UpdateOrganize(id, OrganizeUpdate{
		TargetPath:      "/library/TV/Upload (2025)/Season 04/Upload (2025) S04E03.mkv",
		TargetAt:        &targetAt,
		OrganizeOutcome: "success",
	}))

	oldTV := makeDecision()
	oldTV.SourcePath = "/watch/tv/Old.Show.S01E01.mkv"
	oldTV.SourceFilename = "Old.Show.S01E01.mkv"
	oldTV.EventAt = now.Add(-10 * 24 * time.Hour)
	oldTV.MediaTypeGuessed = "tv"
	oldID, err := db.InsertDecision(oldTV)
	require.NoError(t, err)
	require.NoError(t, db.UpdateOrganize(oldID, OrganizeUpdate{
		TargetPath:      "/library/TV/Old Show/Season 01/Old Show S01E01.mkv",
		TargetAt:        &targetAt,
		OrganizeOutcome: "success",
	}))

	movie := makeDecision()
	movie.SourceFilename = "Movie.2026.mkv"
	movie.EventAt = now
	movie.MediaTypeGuessed = "movie"
	movieID, err := db.InsertDecision(movie)
	require.NoError(t, err)
	require.NoError(t, db.UpdateOrganize(movieID, OrganizeUpdate{
		TargetPath:      "/library/MOVIES/Movie (2026)/Movie (2026).mkv",
		TargetAt:        &targetAt,
		OrganizeOutcome: "success",
	}))

	failedTV := makeDecision()
	failedTV.SourceFilename = "Failed.Show.S01E01.mkv"
	failedTV.EventAt = now
	failedTV.MediaTypeGuessed = "tv"
	failedID, err := db.InsertDecision(failedTV)
	require.NoError(t, err)
	require.NoError(t, db.UpdateOrganize(failedID, OrganizeUpdate{
		OrganizeOutcome: "failed",
		OrganizeError:   "boom",
	}))

	rows, err := db.QueryRecentSuccessfulTVImports(7*24*time.Hour, 50)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, id, rows[0].ID)
	assert.Equal(t, recentTV.SourceFilename, rows[0].SourceFilename)
	assert.Equal(t, "/library/TV/Upload (2025)/Season 04/Upload (2025) S04E03.mkv", rows[0].TargetPath)
}

func TestQueryDecisions_AutoLabel(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	id, err := db.InsertDecision(makeDecision())
	require.NoError(t, err)
	require.NoError(t, db.UpdateAutoLabel(id, "wrong-folder"))

	// insert one without label
	_, err = db.InsertDecision(makeDecision())
	require.NoError(t, err)

	results, err := db.QueryDecisions(QueryFilter{AutoLabel: "wrong-folder"})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, id, results[0].ID)
}

func TestQueryDecisions_AutoLabelIsNull(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	id1, err := db.InsertDecision(makeDecision())
	require.NoError(t, err)
	require.NoError(t, db.UpdateAutoLabel(id1, "labelled"))

	id2, err := db.InsertDecision(makeDecision())
	require.NoError(t, err)

	results, err := db.QueryDecisions(QueryFilter{AutoLabelIsNull: true})
	require.NoError(t, err)
	ids := make([]int64, len(results))
	for i, r := range results {
		ids[i] = r.ID
	}
	assert.NotContains(t, ids, id1)
	assert.Contains(t, ids, id2)
}

func TestQueryDecisions_JellyfinUnresolved(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	id1, err := db.InsertDecision(makeDecision())
	require.NoError(t, err)

	now := time.Now().UTC()
	id2, err := db.InsertDecision(makeDecision())
	require.NoError(t, err)
	require.NoError(t, db.UpdateOutcome(id2, OutcomeUpdate{JellyfinResolvedAt: &now}))

	results, err := db.QueryDecisions(QueryFilter{JellyfinUnresolved: true})
	require.NoError(t, err)
	ids := make([]int64, len(results))
	for i, r := range results {
		ids[i] = r.ID
	}
	assert.Contains(t, ids, id1)
	assert.NotContains(t, ids, id2)
}

func TestQueryDecisions_TargetPathNotEmpty(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	id1, err := db.InsertDecision(makeDecision())
	require.NoError(t, err)

	id2, err := db.InsertDecision(makeDecision())
	require.NoError(t, err)
	require.NoError(t, db.UpdateOrganize(id2, OrganizeUpdate{TargetPath: "/lib/file.mkv", OrganizeOutcome: "success"}))

	results, err := db.QueryDecisions(QueryFilter{TargetPathNotEmpty: true})
	require.NoError(t, err)
	ids := make([]int64, len(results))
	for i, r := range results {
		ids[i] = r.ID
	}
	assert.NotContains(t, ids, id1)
	assert.Contains(t, ids, id2)
}

func TestQueryDecisions_EventAfterBefore(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	early := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	late := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	mid := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)

	d1 := makeDecision()
	d1.EventAt = early
	_, err := db.InsertDecision(d1)
	require.NoError(t, err)

	d2 := makeDecision()
	d2.EventAt = late
	id2, err := db.InsertDecision(d2)
	require.NoError(t, err)

	results, err := db.QueryDecisions(QueryFilter{EventAfter: &mid})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, id2, results[0].ID)
}

func TestQueryDecisions_SourceContains(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	d1 := makeDecision()
	d1.SourcePath = "/downloads/breaking.bad.s01e01.mkv"
	id1, err := db.InsertDecision(d1)
	require.NoError(t, err)

	d2 := makeDecision()
	d2.SourcePath = "/downloads/sopranos.s01e01.mkv"
	_, err = db.InsertDecision(d2)
	require.NoError(t, err)

	results, err := db.QueryDecisions(QueryFilter{SourceContains: "breaking"})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, id1, results[0].ID)
}

func TestQueryDecisions_Limit(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	for i := 0; i < 5; i++ {
		_, err := db.InsertDecision(makeDecision())
		require.NoError(t, err)
	}

	results, err := db.QueryDecisions(QueryFilter{Limit: 3})
	require.NoError(t, err)
	assert.Len(t, results, 3)
}

func TestGetUnresolvedDecisionByTargetPath(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	targetPath := "/library/TV/Show/S01/E01.mkv"
	now := time.Now().UTC().Truncate(time.Second)

	id1, err := db.InsertDecision(makeDecision())
	require.NoError(t, err)
	require.NoError(t, db.UpdateOrganize(id1, OrganizeUpdate{
		TargetPath:      targetPath,
		TargetAt:        &now,
		OrganizeOutcome: "success",
	}))

	got, err := db.GetUnresolvedDecisionByTargetPath(targetPath)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, id1, got.ID)
}

func TestGetUnresolvedDecisionByTargetPath_IgnoresResolved(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	targetPath := "/library/TV/Show/S01/E01.mkv"
	now := time.Now().UTC().Truncate(time.Second)

	id1, err := db.InsertDecision(makeDecision())
	require.NoError(t, err)
	require.NoError(t, db.UpdateOrganize(id1, OrganizeUpdate{
		TargetPath:      targetPath,
		TargetAt:        &now,
		OrganizeOutcome: "success",
	}))
	require.NoError(t, db.UpdateOutcome(id1, OutcomeUpdate{JellyfinResolvedAt: &now}))

	got, err := db.GetUnresolvedDecisionByTargetPath(targetPath)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestGetUnresolvedDecisionByTargetPath_IgnoresFailed(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	targetPath := "/library/TV/Show/S01/E01.mkv"

	id1, err := db.InsertDecision(makeDecision())
	require.NoError(t, err)
	require.NoError(t, db.UpdateOrganize(id1, OrganizeUpdate{
		TargetPath:      targetPath,
		OrganizeOutcome: "failed",
		OrganizeError:   "no match",
	}))

	got, err := db.GetUnresolvedDecisionByTargetPath(targetPath)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestGetUnresolvedDecisionByTargetPath_NotFound(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	got, err := db.GetUnresolvedDecisionByTargetPath("/nonexistent/path.mkv")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestIdentificationStatsEmptyTable(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	stats, err := db.IdentificationStats()
	require.NoError(t, err)
	assert.Equal(t, 0, stats.Total)
	assert.Equal(t, 0, stats.Resolved)
	assert.Equal(t, 0, stats.Identified)
	assert.Equal(t, 0, stats.Unidentified)
	assert.Equal(t, 0, stats.PendingNoSeen)
	assert.Equal(t, 0, stats.FailedAutoLabel)
}

func TestQueryDecisionsUnderFolder(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	folder := "/media/movies/Scary Movie Cut (2026)"
	mustInsert := func(target string, outcome string) {
		d := makeDecision()
		d.TargetPath = target
		d.OrganizeOutcome = outcome
		_, err := db.InsertDecision(d)
		require.NoError(t, err)
	}
	mustInsert(folder+"/Scary Movie Cut (2026).mkv", "success")
	mustInsert(folder+"/Scary Movie Cut (2026)-extra.mkv", "success")
	mustInsert(folder+"/sub/other.mkv", "success")
	mustInsert("/media/movies/Other (2025)/Other (2025).mkv", "success")
	mustInsert(folder+"-suffix/escape-trap.mkv", "success")
	mustInsert(folder+"/failed.mkv", "error")

	got, err := db.QueryDecisionsUnderFolder(folder)
	require.NoError(t, err)
	require.Len(t, got, 3, "only successful rows under the folder (incl. subdirs), no siblings or failures")
	for _, d := range got {
		assert.True(t, strings.HasPrefix(d.TargetPath, folder+string(filepath.Separator)),
			"row %s not under folder", d.TargetPath)
	}
}
