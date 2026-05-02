package quality

import "path/filepath"

// QualityMetadata contains all extracted quality information for database storage
type QualityMetadata struct {
	Resolution  string // "2160p", "1080p", "720p", "480p", "unknown"
	SourceType  string // "REMUX", "BluRay", "WEB-DL", "WEBRip", "HDTV", "DVDRip", "unknown"
	Codec       string // "x265", "x264", "AV1", "unknown"
	AudioFormat string // "Atmos", "TrueHD", "DTS-HD MA", "DD+", "AAC", "unknown"
	QualityScore int   // Computed score for comparison
}

// ExtractMetadata extracts all quality metadata from a file path and size
//
// This is the main entry point for quality analysis. It:
//  1. Parses the filename for quality indicators
//  2. Computes a quality score based on resolution, source, and size
//  3. Returns database-ready strings and the score
//
// Parameters:
//   - path: Full file path (filename is extracted)
//   - fileSize: File size in bytes
//   - isEpisode: true for TV episodes, false for movies
//
// Returns: QualityMetadata ready for database insertion
func ExtractMetadata(path string, fileSize int64, isEpisode bool) QualityMetadata {
	// Parse quality from filename
	info := Parse(filepath.Base(path))

	// Fallback: many releases store quality markers only in the parent
	// folder name, e.g. /MOVIES/Movie (2024) 1080p WEB-DL/Movie.mkv.
	// When the filename alone yields no resolution or source, retry
	// against the immediate parent (and grandparent) directory names.
	if info.Resolution == ResolutionUnknown || info.Source == SourceUnknown {
		dir := filepath.Dir(path)
		for i := 0; i < 2 && dir != "" && dir != "." && dir != "/"; i++ {
			parent := filepath.Base(dir)
			pinfo := Parse(parent)
			if info.Resolution == ResolutionUnknown && pinfo.Resolution != ResolutionUnknown {
				info.Resolution = pinfo.Resolution
			}
			if info.Source == SourceUnknown && pinfo.Source != SourceUnknown {
				info.Source = pinfo.Source
			}
			if info.HDR == HDRNone && pinfo.HDR != HDRNone {
				info.HDR = pinfo.HDR
			}
			if info.Resolution != ResolutionUnknown && info.Source != SourceUnknown {
				break
			}
			dir = filepath.Dir(dir)
		}
		// Recompute score with enriched info
		info.Score = info.ComputeScore()
	}

	// Compute CONDOR quality score (resolution + source + size)
	score := ScoreFile(info, fileSize, isEpisode)

	// Convert to database-friendly strings
	return QualityMetadata{
		Resolution:   ResolutionToString(info.Resolution),
		SourceType:   SourceToString(info.Source),
		Codec:        CodecToString(path),
		AudioFormat:  AudioToString(info.Audio),
		QualityScore: score,
	}
}

// ExtractMovieMetadata is a convenience wrapper for ExtractMetadata with isEpisode=false
func ExtractMovieMetadata(path string, fileSize int64) QualityMetadata {
	return ExtractMetadata(path, fileSize, false)
}

// ExtractEpisodeMetadata is a convenience wrapper for ExtractMetadata with isEpisode=true
func ExtractEpisodeMetadata(path string, fileSize int64) QualityMetadata {
	return ExtractMetadata(path, fileSize, true)
}

// CompareWithSize compares two files including size and returns which is better
// Returns:
//   -1 if file1 is better (higher quality)
//    0 if equal quality
//   +1 if file2 is better (higher quality)
func CompareWithSize(path1 string, size1 int64, path2 string, size2 int64, isEpisode bool) int {
	meta1 := ExtractMetadata(path1, size1, isEpisode)
	meta2 := ExtractMetadata(path2, size2, isEpisode)

	if meta1.QualityScore > meta2.QualityScore {
		return -1 // file1 is better
	}
	if meta1.QualityScore < meta2.QualityScore {
		return 1 // file2 is better
	}
	return 0 // equal
}

// FindBestFile finds the best quality file from a list
//
// Parameters:
//   - files: Map of path -> size
//   - isEpisode: true for TV episodes, false for movies
//
// Returns: path of the best quality file, or empty string if no files
func FindBestFile(files map[string]int64, isEpisode bool) string {
	if len(files) == 0 {
		return ""
	}

	var bestPath string
	var bestScore int = EmptyFilePenalty - 1 // Start lower than worst possible score

	for path, size := range files {
		meta := ExtractMetadata(path, size, isEpisode)
		if meta.QualityScore > bestScore {
			bestScore = meta.QualityScore
			bestPath = path
		}
	}

	return bestPath
}
