package housekeeping

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/database"
	"github.com/Nomadcxx/plex2jellyfin/internal/transfer"
	"github.com/stretchr/testify/require"
)

type fakeMoveTransferer struct{}

func (fakeMoveTransferer) Move(src, dst string, opts transfer.TransferOptions) (*transfer.TransferResult, error) {
	data, err := os.ReadFile(src)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return nil, err
	}
	if err := os.Remove(src); err != nil {
		return nil, err
	}
	return &transfer.TransferResult{Success: true, SourceRemoved: true, BytesCopied: int64(len(data))}, nil
}

func (fakeMoveTransferer) Copy(src, dst string, opts transfer.TransferOptions) (*transfer.TransferResult, error) {
	return nil, nil
}

func (fakeMoveTransferer) CanResume() bool { return false }
func (fakeMoveTransferer) Name() string    { return "fake" }

func openTestDB(t *testing.T) *database.MediaDB {
	t.Helper()
	db, err := database.OpenPath(filepath.Join(t.TempDir(), "media.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestMoveDirWithFallbackMovesFilesAndRemovesSourceTree(t *testing.T) {
	root := t.TempDir()
	srcDir := filepath.Join(root, "Movie Old (2026)")
	dstDir := filepath.Join(root, "Movie New (2026)")
	require.NoError(t, os.MkdirAll(filepath.Join(srcDir, "Extras"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "Movie Old (2026).mkv"), []byte("movie"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "Extras", "sidecar.nfo"), []byte("sidecar"), 0o644))

	engine := &Engine{transferer: fakeMoveTransferer{}}
	require.NoError(t, engine.moveDirWithFallback(srcDir, dstDir))

	require.NoDirExists(t, srcDir)
	require.FileExists(t, filepath.Join(dstDir, "Movie Old (2026).mkv"))
	require.FileExists(t, filepath.Join(dstDir, "Extras", "sidecar.nfo"))
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

func TestDetectEnqueuesParserDriftMovieRenameWhenCanonicalDirIsEmpty(t *testing.T) {
	db := openTestDB(t)
	lib := t.TempDir()

	oldDir := filepath.Join(lib, "Is This Thing On Blu ray (2025)")
	oldPath := filepath.Join(oldDir, "Is This Thing On Blu ray (2025).mp4")
	emptyCanonicalDir := filepath.Join(lib, "Is This Thing On (2025)")
	require.NoError(t, os.MkdirAll(oldDir, 0o755))
	require.NoError(t, os.WriteFile(oldPath, []byte("movie"), 0o644))
	require.NoError(t, os.MkdirAll(emptyCanonicalDir, 0o755))

	now := time.Now().UTC()
	id, err := db.InsertDecision(database.ParseDecision{
		SourcePath:       "/watch/movies/Is.This.Thing.On.2025.2160p.UHD.Blu-ray.Remux.DV.HDR.HEVC.TrueHD.Atmos.7.1-CiNEPHiLES.mp4",
		SourceFilename:   "Is.This.Thing.On.2025.2160p.UHD.Blu-ray.Remux.DV.HDR.HEVC.TrueHD.Atmos.7.1-CiNEPHiLES.mp4",
		EventAt:          now,
		MediaTypeGuessed: "movie",
		ParseMethod:      "regex",
		ParsedTitle:      "Is This Thing On Blu ray",
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

	tasks, err := db.ListHousekeepingTasks(database.TaskStatusPending, 10)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	require.Equal(t, oldPath, tasks[0].Payload["src_path"])
	require.Equal(t, filepath.Join(emptyCanonicalDir, "Is This Thing On (2025).mp4"), tasks[0].Payload["dst_path"])
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

	events, err := db.ListRepairEventsSince(now.Add(-time.Hour), 10)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, database.TaskKindParserDriftRename, events[0].Action)
	require.Equal(t, "auto_safe", events[0].SafetyClass)
	require.Equal(t, "success", events[0].Outcome)
	require.Equal(t, oldPath, events[0].SourcePath)
	require.Equal(t, newPath, events[0].TargetPath)
}

func TestDrainParserDriftMovieRenameFailureWritesRepairEvent(t *testing.T) {
	db := openTestDB(t)
	lib := t.TempDir()

	srcPath := filepath.Join(lib, "Missing Polluted (2026)", "Missing Polluted (2026).mkv")
	dstPath := filepath.Join(lib, "Missing (2026)", "Missing (2026).mkv")

	_, err := db.EnqueueHousekeepingTask("housekeeping.detect", database.TaskKindParserDriftRename, map[string]any{
		"src_path":        srcPath,
		"dst_path":        dstPath,
		"source_filename": "Missing.2026.1080p.WEB-DL.mkv",
	}, 70)
	require.NoError(t, err)

	engine := NewEngine(Config{
		MovieLibraries:     []string{lib},
		MaxConcurrentTasks: 1,
		MaxTasksPerCycle:   50,
		TaskRetryMax:       1,
		TaskPauseBetween:   0,
	}, db, nil)

	since := time.Now().UTC().Add(-time.Hour)
	require.NoError(t, engine.Drain(t.Context()))

	events, err := db.ListRepairEventsSince(since, 10)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, database.TaskKindParserDriftRename, events[0].Action)
	require.Equal(t, "failed", events[0].Outcome)
	require.NotEmpty(t, events[0].Error)
	require.Equal(t, srcPath, events[0].SourcePath)
	require.Equal(t, dstPath, events[0].TargetPath)
}

func TestDrainParserDriftRenameUsesExistingEmptyCanonicalDir(t *testing.T) {
	db := openTestDB(t)
	lib := t.TempDir()

	oldDir := filepath.Join(lib, "Is This Thing On Blu ray (2025)")
	oldPath := filepath.Join(oldDir, "Is This Thing On Blu ray (2025).mp4")
	newDir := filepath.Join(lib, "Is This Thing On (2025)")
	newPath := filepath.Join(newDir, "Is This Thing On (2025).mp4")
	require.NoError(t, os.MkdirAll(oldDir, 0o755))
	require.NoError(t, os.WriteFile(oldPath, []byte("movie"), 0o644))
	require.NoError(t, os.MkdirAll(newDir, 0o755))

	now := time.Now().UTC()
	id, err := db.InsertDecision(database.ParseDecision{
		SourcePath:       "/watch/movies/Is.This.Thing.On.2025.2160p.UHD.Blu-ray.Remux.DV.HDR.HEVC.TrueHD.Atmos.7.1-CiNEPHiLES.mp4",
		SourceFilename:   "Is.This.Thing.On.2025.2160p.UHD.Blu-ray.Remux.DV.HDR.HEVC.TrueHD.Atmos.7.1-CiNEPHiLES.mp4",
		EventAt:          now,
		MediaTypeGuessed: "movie",
		ParseMethod:      "regex",
		ParsedTitle:      "Is This Thing On Blu ray",
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
		"source_filename":   "Is.This.Thing.On.2025.2160p.UHD.Blu-ray.Remux.DV.HDR.HEVC.TrueHD.Atmos.7.1-CiNEPHiLES.mp4",
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
	require.Equal(t, "Is This Thing On", decision.ParsedTitle)
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

func TestDetectEnqueuesParserDriftTVRenameForReleaseYearAfterEpisodeMarker(t *testing.T) {
	db := openTestDB(t)
	lib := t.TempDir()

	oldDir := filepath.Join(lib, "Upload (2025)", "Season 04")
	oldPath := filepath.Join(oldDir, "Upload (2025) S04E03.mkv")
	require.NoError(t, os.MkdirAll(oldDir, 0o755))
	require.NoError(t, os.WriteFile(oldPath, []byte("episode"), 0o644))

	now := time.Now().UTC().Add(-25 * 24 * time.Hour)
	parsedYear := 2025
	id, err := db.InsertDecision(database.ParseDecision{
		SourcePath:       "/watch/tv/Upload.S04E03.2025.1080p.Amazon.WEB-DL.AVC.DDP.5.1-DBTV.mkv",
		SourceFilename:   "Upload.S04E03.2025.1080p.Amazon.WEB-DL.AVC.DDP.5.1-DBTV.mkv",
		EventAt:          now,
		MediaTypeGuessed: "tv",
		ParseMethod:      "regex",
		ParsedTitle:      "Upload",
		ParsedYear:       &parsedYear,
	})
	require.NoError(t, err)
	targetAt := now
	require.NoError(t, db.UpdateOrganize(id, database.OrganizeUpdate{
		TargetPath:      oldPath,
		TargetAt:        &targetAt,
		OrganizeOutcome: "success",
	}))
	require.NoError(t, db.UpdateMetadataCheckState(id, "series_unidentified", "repair candidate", nil))

	engine := NewEngine(Config{
		TVLibraries:        []string{lib},
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
	require.Equal(t, database.TaskKindParserDriftTVRename, tasks[0].Kind)
	require.Equal(t, oldPath, tasks[0].Payload["src_path"])
	require.Equal(t, filepath.Join(lib, "Upload", "Season 04", "Upload S04E03.mkv"), tasks[0].Payload["dst_path"])
}

func TestDetectSkipsTVParserDriftWhenTargetYearIsNotReleaseYearAfterEpisodeMarker(t *testing.T) {
	db := openTestDB(t)
	lib := t.TempDir()

	oldDir := filepath.Join(lib, "For All Mankind (2019)", "Season 05")
	oldPath := filepath.Join(oldDir, "For All Mankind S05E08.mkv")
	require.NoError(t, os.MkdirAll(oldDir, 0o755))
	require.NoError(t, os.WriteFile(oldPath, []byte("episode"), 0o644))

	now := time.Now().UTC()
	parsedYear := 2019
	id, err := db.InsertDecision(database.ParseDecision{
		SourcePath:       "/watch/tv/For.All.Mankind.S05E08.720p.HEVC.x265-MeGusta.mkv",
		SourceFilename:   "For.All.Mankind.S05E08.720p.HEVC.x265-MeGusta.mkv",
		EventAt:          now,
		MediaTypeGuessed: "tv",
		ParseMethod:      "regex",
		ParsedTitle:      "For All Mankind",
		ParsedYear:       &parsedYear,
	})
	require.NoError(t, err)
	targetAt := now
	require.NoError(t, db.UpdateOrganize(id, database.OrganizeUpdate{
		TargetPath:      oldPath,
		TargetAt:        &targetAt,
		OrganizeOutcome: "success",
	}))

	engine := NewEngine(Config{
		TVLibraries:        []string{lib},
		MaxTasksPerCycle:   50,
		MaxConcurrentTasks: 1,
	}, db, nil)

	res, err := engine.Detect(t.Context())
	require.NoError(t, err)
	require.Equal(t, 0, res.ParserDriftRenames)

	tasks, err := db.ListHousekeepingTasks(database.TaskStatusPending, 10)
	require.NoError(t, err)
	require.Empty(t, tasks)
}

func TestDetectSkipsTVParserDriftWhenAlreadyIdentified(t *testing.T) {
	db := openTestDB(t)
	lib := t.TempDir()

	oldDir := filepath.Join(lib, "The Abandons (2025)", "Season 01")
	oldPath := filepath.Join(oldDir, "The Abandons (2025) S01E01.mkv")
	require.NoError(t, os.MkdirAll(oldDir, 0o755))
	require.NoError(t, os.WriteFile(oldPath, []byte("episode"), 0o644))

	now := time.Now().UTC().Add(-25 * 24 * time.Hour)
	parsedYear := 2025
	id, err := db.InsertDecision(database.ParseDecision{
		SourcePath:       "/watch/tv/The.Abandons.S01E01.2025.1080p.Netflix.WEB-DL.AVC.DDP.5.1-DBTV.mkv",
		SourceFilename:   "The.Abandons.S01E01.2025.1080p.Netflix.WEB-DL.AVC.DDP.5.1-DBTV.mkv",
		EventAt:          now,
		MediaTypeGuessed: "tv",
		ParseMethod:      "regex",
		ParsedTitle:      "The Abandons",
		ParsedYear:       &parsedYear,
	})
	require.NoError(t, err)
	targetAt := now
	require.NoError(t, db.UpdateOrganize(id, database.OrganizeUpdate{
		TargetPath:      oldPath,
		TargetAt:        &targetAt,
		OrganizeOutcome: "success",
	}))
	identified := true
	require.NoError(t, db.UpdateOutcome(id, database.OutcomeUpdate{
		JellyfinItemID:     "episode-1",
		JellyfinResolvedAt: &targetAt,
		JellyfinIdentified: &identified,
	}))

	engine := NewEngine(Config{
		TVLibraries:        []string{lib},
		MaxTasksPerCycle:   50,
		MaxConcurrentTasks: 1,
	}, db, nil)

	res, err := engine.Detect(t.Context())
	require.NoError(t, err)
	require.Equal(t, 0, res.ParserDriftRenames)

	tasks, err := db.ListHousekeepingTasks(database.TaskStatusPending, 10)
	require.NoError(t, err)
	require.Empty(t, tasks)
}

func TestDrainParserDriftTVRenameMovesMultipleEpisodeFiles(t *testing.T) {
	db := openTestDB(t)
	lib := t.TempDir()

	oldDir := filepath.Join(lib, "Upload (2025)", "Season 04")
	oldPath1 := filepath.Join(oldDir, "Upload (2025) S04E01.mkv")
	oldPath2 := filepath.Join(oldDir, "Upload (2025) S04E02.mkv")
	newPath1 := filepath.Join(lib, "Upload", "Season 04", "Upload S04E01.mkv")
	newPath2 := filepath.Join(lib, "Upload", "Season 04", "Upload S04E02.mkv")
	require.NoError(t, os.MkdirAll(oldDir, 0o755))
	require.NoError(t, os.WriteFile(oldPath1, []byte("episode1"), 0o644))
	require.NoError(t, os.WriteFile(oldPath2, []byte("episode2"), 0o644))

	now := time.Now().UTC()
	insert := func(sourceFilename, oldPath string) int64 {
		t.Helper()
		parsedYear := 2025
		id, err := db.InsertDecision(database.ParseDecision{
			SourcePath:       filepath.Join("/watch/tv", sourceFilename),
			SourceFilename:   sourceFilename,
			EventAt:          now,
			MediaTypeGuessed: "tv",
			ParseMethod:      "regex",
			ParsedTitle:      "Upload",
			ParsedYear:       &parsedYear,
		})
		require.NoError(t, err)
		targetAt := now
		require.NoError(t, db.UpdateOrganize(id, database.OrganizeUpdate{
			TargetPath:      oldPath,
			TargetAt:        &targetAt,
			OrganizeOutcome: "success",
		}))
		require.NoError(t, db.UpdateMetadataCheckState(id, "series_unidentified", "repair candidate", nil))
		return id
	}

	id1 := insert("Upload.S04E01.2025.1080p.Amazon.WEB-DL.AVC.DDP.5.1-DBTV.mkv", oldPath1)
	id2 := insert("Upload.S04E02.2025.1080p.Amazon.WEB-DL.AVC.DDP.5.1-DBTV.mkv", oldPath2)

	_, err := db.EnqueueHousekeepingTask("housekeeping.detect", database.TaskKindParserDriftTVRename, map[string]any{
		"parse_decision_id": id1,
		"src_path":          oldPath1,
		"dst_path":          newPath1,
		"source_filename":   "Upload.S04E01.2025.1080p.Amazon.WEB-DL.AVC.DDP.5.1-DBTV.mkv",
	}, 70)
	require.NoError(t, err)
	_, err = db.EnqueueHousekeepingTask("housekeeping.detect", database.TaskKindParserDriftTVRename, map[string]any{
		"parse_decision_id": id2,
		"src_path":          oldPath2,
		"dst_path":          newPath2,
		"source_filename":   "Upload.S04E02.2025.1080p.Amazon.WEB-DL.AVC.DDP.5.1-DBTV.mkv",
	}, 70)
	require.NoError(t, err)

	engine := NewEngine(Config{
		TVLibraries:        []string{lib},
		MaxConcurrentTasks: 1,
		MaxTasksPerCycle:   50,
		TaskRetryMax:       1,
		TaskPauseBetween:   0,
	}, db, nil)

	require.NoError(t, engine.Drain(t.Context()))

	require.NoFileExists(t, oldPath1)
	require.NoFileExists(t, oldPath2)
	require.NoDirExists(t, filepath.Join(lib, "Upload (2025)"))
	require.FileExists(t, newPath1)
	require.FileExists(t, newPath2)

	decision1, err := db.GetDecision(id1)
	require.NoError(t, err)
	require.NotNil(t, decision1)
	require.Equal(t, newPath1, decision1.TargetPath)
	require.Equal(t, "Upload", decision1.ParsedTitle)
	require.Nil(t, decision1.ParsedYear)
	require.NotNil(t, decision1.ParsedSeason)
	require.Equal(t, 4, *decision1.ParsedSeason)
	require.NotNil(t, decision1.ParsedEpisode)
	require.Equal(t, 1, *decision1.ParsedEpisode)

	decision2, err := db.GetDecision(id2)
	require.NoError(t, err)
	require.NotNil(t, decision2)
	require.Equal(t, newPath2, decision2.TargetPath)
	require.Equal(t, "Upload", decision2.ParsedTitle)
	require.Nil(t, decision2.ParsedYear)
	require.NotNil(t, decision2.ParsedEpisode)
	require.Equal(t, 2, *decision2.ParsedEpisode)
}

func TestDrainMergeMoveUpdatesParseDecisionTargetPaths(t *testing.T) {
	db := openTestDB(t)
	srcRoot := filepath.Join(t.TempDir(), "Upload")
	dstRoot := filepath.Join(t.TempDir(), "Upload (2020)")
	srcPath := filepath.Join(srcRoot, "Season 04", "Upload S04E01.mkv")
	dstPath := filepath.Join(dstRoot, "Season 04", "Upload S04E01.mkv")
	require.NoError(t, os.MkdirAll(filepath.Dir(srcPath), 0o755))
	require.NoError(t, os.MkdirAll(dstRoot, 0o755))
	require.NoError(t, os.WriteFile(srcPath, []byte("episode"), 0o644))

	now := time.Now().UTC()
	id, err := db.InsertDecision(database.ParseDecision{
		SourcePath:       "/watch/tv/Upload.S04E01.2025.mkv",
		SourceFilename:   "Upload.S04E01.2025.mkv",
		EventAt:          now,
		MediaTypeGuessed: "tv",
		ParseMethod:      "regex",
		ParsedTitle:      "Upload",
	})
	require.NoError(t, err)
	targetAt := now
	require.NoError(t, db.UpdateOrganize(id, database.OrganizeUpdate{
		TargetPath:      srcPath,
		TargetAt:        &targetAt,
		OrganizeOutcome: "success",
	}))

	_, err = db.EnqueueHousekeepingTask("housekeeping.detect", database.TaskKindNoYearMerge, map[string]any{
		"src_path": srcRoot,
		"dst_path": dstRoot,
		"src_lib":  filepath.Dir(srcRoot),
		"dst_lib":  filepath.Dir(dstRoot),
		"kind":     "tv",
	}, 50)
	require.NoError(t, err)

	engine := NewEngine(Config{
		MaxConcurrentTasks: 1,
		MaxTasksPerCycle:   50,
		TaskRetryMax:       1,
		TaskPauseBetween:   0,
	}, db, nil)
	require.NoError(t, engine.Drain(t.Context()))

	require.NoFileExists(t, srcPath)
	require.FileExists(t, dstPath)
	decision, err := db.GetDecision(id)
	require.NoError(t, err)
	require.NotNil(t, decision)
	require.Equal(t, dstPath, decision.TargetPath)
}

type fakeRescanner struct{ paths []string }

func (f *fakeRescanner) NotifyMediaUpdated(p []string) error {
	f.paths = append(f.paths, p...)
	return nil
}

func insertDecisionForDriftTest(t *testing.T, db *database.MediaDB, targetPath string, targetAt time.Time) int64 {
	t.Helper()
	id, err := db.InsertDecision(database.ParseDecision{
		SourcePath:       "/downloads/scary.movie.extended.cut.2026/scary.movie.extended.cut.2026.mkv",
		SourceFilename:   "scary.movie.extended.cut.2026.1080p.x265.mkv",
		EventAt:          targetAt,
		MediaTypeGuessed: "movie",
		ParseMethod:      "regex",
		ParsedTitle:      "scary movie cut",
		TargetPath:       targetPath,
		TargetAt:         &targetAt,
		OrganizeOutcome:  "success",
	})
	require.NoError(t, err)
	return id
}

func TestParserDriftRenamePreservesTargetAt(t *testing.T) {
	db := openTestDB(t)

	movieLib := t.TempDir()
	srcDir := filepath.Join(movieLib, "Scary Movie Cut (2026)")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))
	srcFile := filepath.Join(srcDir, "Scary Movie Cut (2026).mkv")
	require.NoError(t, os.WriteFile(srcFile, []byte("x"), 0o644))
	dstFile := filepath.Join(movieLib, "Scary Movie (2026)", "Scary Movie (2026).mkv")

	originalTargetAt := time.Now().UTC().Add(-72 * time.Hour).Truncate(time.Second)
	id := insertDecisionForDriftTest(t, db, srcFile, originalTargetAt)

	e := NewEngine(Config{MovieLibraries: []string{movieLib}}, db, nil)
	rescanner := &fakeRescanner{}
	e.SetMediaRescanner(rescanner)

	task := &database.HousekeepingTask{
		Kind: database.TaskKindParserDriftRename,
		Payload: map[string]any{
			"parse_decision_id": float64(id),
			"src_path":          srcFile,
			"dst_path":          dstFile,
			"source_filename":   "scary.movie.extended.cut.2026.1080p.x265.mkv",
		},
	}
	require.NoError(t, e.execParserDriftRename(task))

	got, err := db.GetDecision(id)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotNil(t, got.TargetAt, "TargetAt should be preserved")
	require.True(t, got.TargetAt.Equal(originalTargetAt), "TargetAt = %v, want preserved %v", got.TargetAt, originalTargetAt)
	require.NotEmpty(t, rescanner.paths, "expected a targeted rescan, got none")
}

func TestVerifierRenameWritesVerifierTitleAndStamps(t *testing.T) {
	db := openTestDB(t)

	movieLib := t.TempDir()
	srcDir := filepath.Join(movieLib, "Scary Movie Cut (2026)")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))
	srcFile := filepath.Join(srcDir, "Scary Movie Cut (2026).mkv")
	require.NoError(t, os.WriteFile(srcFile, []byte("x"), 0o644))
	dstFile := filepath.Join(movieLib, "Scary Movie (2026)", "Scary Movie (2026).mkv")

	originalTargetAt := time.Now().UTC().Add(-72 * time.Hour).Truncate(time.Second)
	id := insertDecisionForDriftTest(t, db, srcFile, originalTargetAt)

	e := NewEngine(Config{MovieLibraries: []string{movieLib}}, db, nil)
	rescanner := &fakeRescanner{}
	e.SetMediaRescanner(rescanner)

	task := &database.HousekeepingTask{
		Kind: database.TaskKindVerifierRename,
		Payload: map[string]any{
			"parse_decision_id": float64(id),
			"src_path":          srcFile,
			"dst_path":          dstFile,
			"new_title":         "Scary Movie",
			"new_year":          "2026",
			"tmdb_id":           "111",
		},
	}
	require.NoError(t, e.execVerifierRename(task))

	got, err := db.GetDecision(id)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "Scary Movie", got.ParsedTitle, "ParsedTitle should be verifier title, not re-parsed")
	require.Equal(t, "verifier", got.ExistingMatchMethod, "ExistingMatchMethod should be stamped verifier")
	require.NotNil(t, got.TargetAt, "TargetAt should be preserved")
	require.True(t, got.TargetAt.Equal(originalTargetAt), "TargetAt = %v, want preserved %v", got.TargetAt, originalTargetAt)
	require.NotEmpty(t, rescanner.paths, "expected a targeted rescan, got none")
}

func TestParserDriftSkipsVerifierCorrectedRows(t *testing.T) {
	db := openTestDB(t)
	movieLib := t.TempDir()
	srcDir := filepath.Join(movieLib, "Corrected Title (2026)")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))
	srcPath := filepath.Join(srcDir, "Corrected Title (2026).mkv")
	require.NoError(t, os.WriteFile(srcPath, []byte("x"), 0o644))
	e := NewEngine(Config{MovieLibraries: []string{movieLib}}, db, nil)

	d := &database.ParseDecision{
		SourceFilename:      "some.obscure.release.2026.1080p.x265.mkv",
		TargetPath:          srcPath,
		ExistingMatchMethod: "verifier",
	}
	_, _, ok := e.parserDriftMovieRename(d)
	require.False(t, ok, "parserDriftMovieRename returned ok=true for a verifier-corrected row; want skipped")
}
