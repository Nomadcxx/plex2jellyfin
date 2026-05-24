package database

import (
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
