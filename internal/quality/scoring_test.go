package quality

import (
	"testing"
)

func TestScoreFile_EmptyFile(t *testing.T) {
	info := &QualityInfo{
		Resolution: Resolution1080p,
		Source:     SourceBluRay,
	}

	score := ScoreFile(info, 0, false)
	if score != EmptyFilePenalty {
		t.Errorf("expected empty file penalty %d, got %d", EmptyFilePenalty, score)
	}
}

func TestScoreFile_ResolutionDominance(t *testing.T) {
	// Test that resolution is the dominant factor
	// A 1080p file should beat a 720p file even if the 720p is much larger

	// 1080p, 2GB
	info1080p := &QualityInfo{
		Resolution: Resolution1080p,
		Source:     SourceWEBDL,
	}
	score1080p := ScoreFile(info1080p, 2*1024*1024*1024, false)

	// 720p, 10GB (much larger)
	info720p := &QualityInfo{
		Resolution: Resolution720p,
		Source:     SourceWEBDL,
	}
	score720p := ScoreFile(info720p, 10*1024*1024*1024, false)

	if score1080p <= score720p {
		t.Errorf("1080p should beat 720p regardless of size: 1080p=%d, 720p=%d", score1080p, score720p)
	}
}

func TestScoreFile_576pAnd4320pResolutions(t *testing.T) {
	size := int64(2 * 1024 * 1024 * 1024)

	score480p := ScoreFile(&QualityInfo{Resolution: Resolution480p}, size, false)
	score576p := ScoreFile(&QualityInfo{Resolution: Resolution576p}, size, false)
	score720p := ScoreFile(&QualityInfo{Resolution: Resolution720p}, size, false)
	score2160p := ScoreFile(&QualityInfo{Resolution: Resolution2160p}, size, false)
	score4320p := ScoreFile(&QualityInfo{Resolution: Resolution4320p}, size, false)

	if !(score480p < score576p && score576p < score720p) {
		t.Fatalf("576p score should sit between 480p and 720p: 480p=%d 576p=%d 720p=%d", score480p, score576p, score720p)
	}
	if score4320p <= score2160p {
		t.Fatalf("4320p score should exceed 2160p: 4320p=%d 2160p=%d", score4320p, score2160p)
	}
}

func TestScoreFile_SourceQuality(t *testing.T) {
	// Test that source type matters: REMUX > BluRay > WEB-DL > WEBRip

	tests := []struct {
		source      Source
		expectedMin int
		expectedMax int
	}{
		{SourceREMUX, 400, 420},  // Very high
		{SourceBluRay, 380, 400}, // High
		{SourceWEBDL, 360, 380},  // Medium-high
		{SourceWEBRip, 350, 370}, // Medium
		{SourceHDTV, 340, 360},   // Medium-low
		{SourceDVDRip, 325, 345}, // Low
	}

	for _, tt := range tests {
		info := &QualityInfo{
			Resolution: Resolution1080p,
			Source:     tt.source,
		}
		score := ScoreFile(info, 5*1024*1024*1024, false) // 5GB

		if score < tt.expectedMin || score > tt.expectedMax {
			t.Errorf("%s: expected score between %d and %d, got %d",
				tt.source.String(), tt.expectedMin, tt.expectedMax, score)
		}
	}
}

func TestScoreFile_SizeBonus(t *testing.T) {
	// Test that file size adds bonus points

	info := &QualityInfo{
		Resolution: Resolution1080p,
		Source:     SourceBluRay,
	}

	// 1GB file
	score1GB := ScoreFile(info, 1*1024*1024*1024, false)

	// 5GB file (should score higher)
	score5GB := ScoreFile(info, 5*1024*1024*1024, false)

	// 10GB file (should score even higher)
	score10GB := ScoreFile(info, 10*1024*1024*1024, false)

	if score5GB <= score1GB {
		t.Errorf("5GB file should score higher than 1GB: 5GB=%d, 1GB=%d", score5GB, score1GB)
	}

	if score10GB <= score5GB {
		t.Errorf("10GB file should score higher than 5GB: 10GB=%d, 5GB=%d", score10GB, score5GB)
	}
}

func TestScoreFile_SizeCap(t *testing.T) {
	// Test that size bonus is capped

	info := &QualityInfo{
		Resolution: Resolution1080p,
		Source:     SourceBluRay,
	}

	// 50GB file (at cap for movies)
	score50GB := ScoreFile(info, 50*1024*1024*1024, false)

	// 100GB file (should be same as 50GB due to cap)
	score100GB := ScoreFile(info, 100*1024*1024*1024, false)

	if score50GB != score100GB {
		t.Errorf("size bonus should be capped: 50GB=%d, 100GB=%d", score50GB, score100GB)
	}
}

func TestScoreFile_EpisodeVsMovie(t *testing.T) {
	// Test that episodes have lower size cap than movies

	info := &QualityInfo{
		Resolution: Resolution1080p,
		Source:     SourceWEBDL,
	}

	// 15GB episode (should hit cap at 10GB)
	scoreEpisode := ScoreFile(info, 15*1024*1024*1024, true)

	// 15GB movie (should get full 15 points)
	scoreMovie := ScoreFile(info, 15*1024*1024*1024, false)

	if scoreMovie <= scoreEpisode {
		t.Errorf("movie with same size should score higher due to higher cap: movie=%d, episode=%d",
			scoreMovie, scoreEpisode)
	}
}

func TestScoreFile_UnknownResolution(t *testing.T) {
	// Test that unknown resolution gets size-weighted bonus

	info := &QualityInfo{
		Resolution: ResolutionUnknown,
		Source:     SourceBluRay,
	}

	// 5GB file with unknown resolution
	// Should get 5 * 20 = 100 points from size
	score := ScoreFile(info, 5*1024*1024*1024, false)

	// Should be competitive with lower resolution files
	// 5GB unknown (~185) can compete with 720p but not 1080p
	if score < 150 {
		t.Errorf("unknown resolution with good size should be competitive, got %d", score)
	}
}

// Real-world test cases from user's library

func TestRealWorld_Her(t *testing.T) {
	// STORAGE1: Her (2013).mkv - 5.5GB, 1080p (likely WEB-DL)
	// STORAGE8: Her.2013.MULTI.1080p.BluRay.x264-Goatlove.mkv - 9.8GB, 1080p BluRay

	file1 := Parse("Her (2013).mkv")
	score1 := ScoreMovie(file1, 5500000000) // 5.5GB

	file2 := Parse("Her.2013.MULTI.1080p.BluRay.x264-Goatlove.mkv")
	score2 := ScoreMovie(file2, 9800000000) // 9.8GB

	if score2 <= score1 {
		t.Errorf("BluRay 9.8GB should beat WEB-DL 5.5GB: BluRay=%d, WEB-DL=%d", score2, score1)
	}

	t.Logf("Her: STORAGE1=%d, STORAGE8=%d (keep STORAGE8)", score1, score2)
}

func TestRealWorld_Interstellar(t *testing.T) {
	// Both are 3.5GB, both appear to be 1080p
	// STORAGE6: Interstellar (2014).mkv
	// STORAGE8: Interstellar 2014 IMAX Ed BluRay 1080p DD5.1 H265-d3g.mkv

	file1 := Parse("Interstellar (2014).mkv")
	score1 := ScoreMovie(file1, 3500000000)

	file2 := Parse("Interstellar 2014 IMAX Ed BluRay 1080p DD5.1 H265-d3g.mkv")
	score2 := ScoreMovie(file2, 3500000000)

	// STORAGE8 is explicitly marked BluRay with better audio, should win
	if score2 <= score1 {
		t.Errorf("IMAX BluRay should beat unknown source: IMAX=%d, unknown=%d", score2, score1)
	}

	t.Logf("Interstellar: STORAGE6=%d, STORAGE8=%d (keep STORAGE8)", score1, score2)
}

func TestRealWorld_TopGun(t *testing.T) {
	// Both are 9.0GB, both 1080p
	// STORAGE2: Top Gun (1986).mkv
	// STORAGE8: Top.Gun.1986.REMASTERED.HEVC.1080p.BluRay.DD+7.1.x265-LEGi0N.mkv

	file1 := Parse("Top Gun (1986).mkv")
	score1 := ScoreMovie(file1, 9000000000)

	file2 := Parse("Top.Gun.1986.REMASTERED.HEVC.1080p.BluRay.DD+7.1.x265-LEGi0N.mkv")
	score2 := ScoreMovie(file2, 9000000000)

	// STORAGE8 is BluRay with better audio (DD+ 7.1), should win
	if score2 <= score1 {
		t.Errorf("BluRay DD+7.1 should beat unknown: BluRay=%d, unknown=%d", score2, score1)
	}

	t.Logf("Top Gun: STORAGE2=%d, STORAGE8=%d (keep STORAGE8)", score1, score2)
}

func TestExtractMetadata(t *testing.T) {
	// Test metadata extraction for database storage

	tests := []struct {
		path       string
		size       int64
		isEpisode  bool
		wantRes    string
		wantSource string
	}{
		{
			path:       "/test/Her.2013.MULTI.1080p.BluRay.x264-Goatlove.mkv",
			size:       9800000000,
			isEpisode:  false,
			wantRes:    "1080p",
			wantSource: "BluRay",
		},
		{
			path:       "/test/Interstellar.2014.2160p.REMUX.mkv",
			size:       50000000000,
			isEpisode:  false,
			wantRes:    "2160p",
			wantSource: "REMUX",
		},
		{
			path:       "/test/Show.S01E01.720p.WEB-DL.mkv",
			size:       1200000000,
			isEpisode:  true,
			wantRes:    "720p",
			wantSource: "WEB-DL",
		},
	}

	for _, tt := range tests {
		meta := ExtractMetadata(tt.path, tt.size, tt.isEpisode)

		if meta.Resolution != tt.wantRes {
			t.Errorf("%s: expected resolution %s, got %s", tt.path, tt.wantRes, meta.Resolution)
		}

		if meta.SourceType != tt.wantSource {
			t.Errorf("%s: expected source %s, got %s", tt.path, tt.wantSource, meta.SourceType)
		}

		if meta.QualityScore == 0 {
			t.Errorf("%s: quality score should not be 0", tt.path)
		}
	}
}

func TestCompareWithSize(t *testing.T) {
	// Test file comparison with size included

	// Better file (1080p BluRay 10GB)
	path1 := "/test/Movie.2020.1080p.BluRay.mkv"
	size1 := int64(10 * 1024 * 1024 * 1024)

	// Worse file (720p WEB-DL 5GB)
	path2 := "/test/Movie.2020.720p.WEB-DL.mkv"
	size2 := int64(5 * 1024 * 1024 * 1024)

	result := CompareWithSize(path1, size1, path2, size2, false)

	if result != -1 {
		t.Errorf("expected file1 to be better (return -1), got %d", result)
	}
}

func TestFindBestFile(t *testing.T) {
	// Test finding best file from a map

	files := map[string]int64{
		"/test/Movie.720p.mkv":         3 * 1024 * 1024 * 1024, // 3GB 720p
		"/test/Movie.1080p.WEB-DL.mkv": 5 * 1024 * 1024 * 1024, // 5GB 1080p WEB-DL
		"/test/Movie.1080p.BluRay.mkv": 8 * 1024 * 1024 * 1024, // 8GB 1080p BluRay (best)
		"/test/Movie.1080p.WEBRip.mkv": 4 * 1024 * 1024 * 1024, // 4GB 1080p WEBRip
	}

	best := FindBestFile(files, false)

	expected := "/test/Movie.1080p.BluRay.mkv"
	if best != expected {
		t.Errorf("expected best file %s, got %s", expected, best)
	}
}

func TestShouldInclude(t *testing.T) {
	// Test minimum size thresholds

	// Movies: 500MB minimum
	if !ShouldIncludeMovie(600 * 1024 * 1024) {
		t.Error("600MB movie should be included")
	}

	if ShouldIncludeMovie(400 * 1024 * 1024) {
		t.Error("400MB movie should not be included (trailer/sample)")
	}

	// Episodes: 50MB minimum
	if !ShouldIncludeEpisode(100 * 1024 * 1024) {
		t.Error("100MB episode should be included")
	}

	if ShouldIncludeEpisode(40 * 1024 * 1024) {
		t.Error("40MB episode should not be included (sample)")
	}
}
