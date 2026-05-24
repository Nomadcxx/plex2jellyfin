package housekeeping

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Nomadcxx/jellywatch/internal/database"
	"github.com/stretchr/testify/require"
)

func openTestDB(t *testing.T) *database.MediaDB {
	t.Helper()
	db, err := database.OpenPath(filepath.Join(t.TempDir(), "media.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestDetectEnqueuesParserDriftMovieRename(t *testing.T) {
	db := openTestDB(t)
	lib := t.TempDir()

	oldDir := filepath.Join(lib, "Mortal Kombat (2026)")
	oldPath := filepath.Join(oldDir, "Mortal Kombat (2026).mp4")
	require.NoError(t, os.MkdirAll(oldDir, 0o755))
	require.NoError(t, os.WriteFile(oldPath, []byte("movie"), 0o644))

	now := time.Now().UTC()
	id, err := db.InsertDecision(database.ParseDecision{
		SourcePath:          "/watch/movies/Mortal.Kombat.II.2026.1080p.WEB-DL.mp4",
		SourceFilename:      "Mortal.Kombat.II.2026.1080p.WEB-DL.mp4",
		EventAt:             now,
		MediaTypeGuessed:    "movie",
		ParseMethod:         "regex",
		ParsedTitle:         "Mortal Kombat",
		OrganizeOutcome:     "success",
		ExistingMatchMethod: "",
	})
	require.NoError(t, err)
	targetAt := now
	require.NoError(t, db.UpdateOrganize(id, database.OrganizeUpdate{
		TargetPath:      oldPath,
		TargetAt:        &targetAt,
		OrganizeOutcome: "success",
	}))

	engine := NewEngine(Config{
		MovieLibraries:     []string{lib},
		MaxTasksPerCycle:   50,
		MaxConcurrentTasks: 1,
	}, db, nil)

	res, err := engine.Detect(t.Context())
	require.NoError(t, err)
	require.Equal(t, 1, res.ParserDriftRenames)
	require.Equal(t, 1, res.Enqueued)

	tasks, err := db.ListHousekeepingTasks(database.TaskStatusPending, 10)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	require.Equal(t, "parser_drift_rename", tasks[0].Kind)
	require.Equal(t, oldPath, tasks[0].Payload["src_path"])
	require.Equal(t, filepath.Join(lib, "Mortal Kombat II (2026)", "Mortal Kombat II (2026).mp4"), tasks[0].Payload["dst_path"])
}

func TestDrainExecutesParserDriftMovieRename(t *testing.T) {
	db := openTestDB(t)
	lib := t.TempDir()

	oldDir := filepath.Join(lib, "The Devil Wears Prada 2 DCP (2026)")
	oldPath := filepath.Join(oldDir, "The Devil Wears Prada 2 DCP (2026).mkv")
	newPath := filepath.Join(lib, "The Devil Wears Prada 2 (2026)", "The Devil Wears Prada 2 (2026).mkv")
	require.NoError(t, os.MkdirAll(oldDir, 0o755))
	require.NoError(t, os.WriteFile(oldPath, []byte("movie"), 0o644))

	now := time.Now().UTC()
	id, err := db.InsertDecision(database.ParseDecision{
		SourcePath:       "/watch/movies/The.Devil.Wears.Prada.2.2026.1080p.DCP.WEBRIP.AC3.x264-AOC.mkv",
		SourceFilename:   "The.Devil.Wears.Prada.2.2026.1080p.DCP.WEBRIP.AC3.x264-AOC.mkv",
		EventAt:          now,
		MediaTypeGuessed: "movie",
		ParseMethod:      "regex",
		ParsedTitle:      "The Devil Wears Prada 2 DCP",
	})
	require.NoError(t, err)
	targetAt := now
	require.NoError(t, db.UpdateOrganize(id, database.OrganizeUpdate{
		TargetPath:      oldPath,
		TargetAt:        &targetAt,
		OrganizeOutcome: "success",
	}))

	_, err = db.EnqueueHousekeepingTask("housekeeping.detect", database.TaskKindParserDriftRename, map[string]any{
		"parse_decision_id": id,
		"src_path":          oldPath,
		"dst_path":          newPath,
		"source_filename":   "The.Devil.Wears.Prada.2.2026.1080p.DCP.WEBRIP.AC3.x264-AOC.mkv",
	}, 70)
	require.NoError(t, err)

	engine := NewEngine(Config{
		MovieLibraries:     []string{lib},
		MaxConcurrentTasks: 1,
		MaxTasksPerCycle:   50,
		TaskRetryMax:       1,
	}, db, nil)

	require.NoError(t, engine.Drain(t.Context()))

	require.NoFileExists(t, oldPath)
	require.NoDirExists(t, oldDir)
	require.FileExists(t, newPath)

	decision, err := db.GetDecision(id)
	require.NoError(t, err)
	require.NotNil(t, decision)
	require.Equal(t, newPath, decision.TargetPath)
	require.Equal(t, "The Devil Wears Prada 2", decision.ParsedTitle)
	require.NotNil(t, decision.ParsedYear)
	require.Equal(t, 2026, *decision.ParsedYear)

	tasks, err := db.ListHousekeepingTasks(database.TaskStatusDone, 10)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
}

func TestDrainParserDriftRepairReconcilesAlreadyMovedFile(t *testing.T) {
	db := openTestDB(t)
	lib := t.TempDir()

	oldPath := filepath.Join(lib, "Tom Clancys Jack Ryan Ghost War + (2026)", "Tom Clancys Jack Ryan Ghost War + (2026).mkv")
	newDir := filepath.Join(lib, "Tom Clancys Jack Ryan Ghost War (2026)")
	newPath := filepath.Join(newDir, "Tom Clancys Jack Ryan Ghost War (2026).mkv")
	require.NoError(t, os.MkdirAll(newDir, 0o755))
	require.NoError(t, os.WriteFile(newPath, []byte("movie"), 0o644))

	now := time.Now().UTC()
	id, err := db.InsertDecision(database.ParseDecision{
		SourcePath:       "/watch/movies/Tom.Clancys.Jack.Ryan.Ghost.War.2026.2160p.WEB-DL.HDR10+.mkv",
		SourceFilename:   "Tom.Clancys.Jack.Ryan.Ghost.War.2026.2160p.WEB-DL.HDR10+.mkv",
		EventAt:          now,
		MediaTypeGuessed: "movie",
		ParseMethod:      "regex",
		ParsedTitle:      "Tom Clancys Jack Ryan Ghost War +",
	})
	require.NoError(t, err)
	targetAt := now
	require.NoError(t, db.UpdateOrganize(id, database.OrganizeUpdate{
		TargetPath:      oldPath,
		TargetAt:        &targetAt,
		OrganizeOutcome: "success",
	}))

	engine := NewEngine(Config{
		MovieLibraries:     []string{lib},
		MaxConcurrentTasks: 1,
		MaxTasksPerCycle:   50,
		TaskRetryMax:       1,
	}, db, nil)

	res, err := engine.Detect(t.Context())
	require.NoError(t, err)
	require.Equal(t, 1, res.ParserDriftRenames)
	require.NoError(t, engine.Drain(t.Context()))

	require.NoFileExists(t, oldPath)
	require.FileExists(t, newPath)

	decision, err := db.GetDecision(id)
	require.NoError(t, err)
	require.NotNil(t, decision)
	require.Equal(t, newPath, decision.TargetPath)
	require.Equal(t, "Tom Clancys Jack Ryan Ghost War", decision.ParsedTitle)
}
