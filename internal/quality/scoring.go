package quality

import "strings"

// CONDOR Quality Scoring System
// Based on jellysink's proven algorithm: Resolution > Source > Size
//
// This scoring system determines which file to keep when duplicates are found.
// Higher score = better quality = file to keep.

const (
	// Resolution scores (highest priority)
	ScoreResolution4320p = 500
	ScoreResolution2160p = 400
	ScoreResolution1080p = 300
	ScoreResolution720p  = 200
	ScoreResolution576p  = 150
	ScoreResolution480p  = 100

	// Source type scores (second priority)
	ScoreSourceREMUX  = 100
	ScoreSourceBluRay = 80
	ScoreSourceWEBDL  = 60
	ScoreSourceWEBRip = 50
	ScoreSourceHDTV   = 40
	ScoreSourceDVDRip = 20

	// Size bonus (per GB, capped)
	MaxSizeBonusGB   = 50 // Movies
	MaxSizeBonusGBTV = 10 // TV episodes
	SizeBonusPerGB   = 1

	// Unknown resolution gets size-weighted bonus
	UnknownResolutionMultiplier = 20

	// Minimum sizes for inclusion (bytes)
	MinMovieSize   = 500 * 1024 * 1024 // 500 MB
	MinEpisodeSize = 50 * 1024 * 1024  // 50 MB

	// Empty file penalty
	EmptyFilePenalty = -1000
)

// ScoreFile calculates a quality score for a media file using CONDOR algorithm.
// This combines resolution, source type, and file size.
//
// Algorithm:
//  1. Empty files always score -1000 (worst)
//  2. Resolution provides base score (400 for 4K, 300 for 1080p, etc.)
//  3. Source type adds points (100 for REMUX, 80 for BluRay, etc.)
//  4. File size adds 1 point per GB (capped at 50GB for movies, 10GB for episodes)
//  5. Unknown resolution gets size-weighted bonus (50x size multiplier)
//
// Parameters:
//   - info: Quality metadata from Parse()
//   - fileSize: File size in bytes
//   - isEpisode: true for TV episodes (affects size cap), false for movies
//
// Returns: Quality score (higher is better)
func ScoreFile(info *QualityInfo, fileSize int64, isEpisode bool) int {
	// Empty files are always worst
	if fileSize == 0 {
		return EmptyFilePenalty
	}

	score := 0
	sizeGB := fileSize / (1024 * 1024 * 1024)

	// Resolution scoring (highest weight)
	switch info.Resolution {
	case Resolution4320p:
		score += ScoreResolution4320p
	case Resolution2160p:
		score += ScoreResolution2160p
	case Resolution1080p:
		score += ScoreResolution1080p
	case Resolution720p:
		score += ScoreResolution720p
	case Resolution576p:
		score += ScoreResolution576p
	case Resolution480p:
		score += ScoreResolution480p
	case ResolutionUnknown:
		// For unknown resolution, heavily weight file size
		// This handles cases where filename lacks resolution marker
		// Example: 5GB file gets 5 * 50 = 250 points
		score += int(sizeGB) * UnknownResolutionMultiplier
	}

	// Source type scoring (second highest weight)
	switch info.Source {
	case SourceREMUX:
		score += ScoreSourceREMUX
	case SourceBluRay:
		score += ScoreSourceBluRay
	case SourceWEBDL:
		score += ScoreSourceWEBDL
	case SourceWEBRip:
		score += ScoreSourceWEBRip
	case SourceHDTV:
		score += ScoreSourceHDTV
	case SourceDVDRip:
		score += ScoreSourceDVDRip
	}

	// Size bonus (capped based on media type)
	maxBonus := MaxSizeBonusGB
	if isEpisode {
		maxBonus = MaxSizeBonusGBTV
	}

	if sizeGB > int64(maxBonus) {
		sizeGB = int64(maxBonus)
	}

	score += int(sizeGB) * SizeBonusPerGB

	return score
}

// ScoreMovie is a convenience wrapper for ScoreFile with isEpisode=false
func ScoreMovie(info *QualityInfo, fileSize int64) int {
	return ScoreFile(info, fileSize, false)
}

// ScoreEpisode is a convenience wrapper for ScoreFile with isEpisode=true
func ScoreEpisode(info *QualityInfo, fileSize int64) int {
	return ScoreFile(info, fileSize, true)
}

// ShouldIncludeMovie determines if a movie file meets minimum size threshold
func ShouldIncludeMovie(fileSize int64) bool {
	return fileSize >= MinMovieSize
}

// ShouldIncludeEpisode determines if an episode file meets minimum size threshold
func ShouldIncludeEpisode(fileSize int64) bool {
	return fileSize >= MinEpisodeSize
}

// ResolutionToString converts Resolution enum to database-friendly string
func ResolutionToString(r Resolution) string {
	switch r {
	case Resolution2160p:
		return "2160p"
	case Resolution1080p:
		return "1080p"
	case Resolution720p:
		return "720p"
	case Resolution480p:
		return "480p"
	case Resolution576p:
		return "576p"
	case Resolution4320p:
		return "4320p"
	default:
		return "unknown"
	}
}

// SourceToString converts Source enum to database-friendly string
func SourceToString(s Source) string {
	switch s {
	case SourceREMUX:
		return "REMUX"
	case SourceBluRay:
		return "BluRay"
	case SourceWEBDL:
		return "WEB-DL"
	case SourceWEBRip:
		return "WEBRip"
	case SourceHDTV:
		return "HDTV"
	case SourceDVDRip:
		return "DVDRip"
	case SourceDVDScr:
		return "DVDScr"
	case SourceTC:
		return "TC"
	case SourceTS:
		return "TS"
	case SourceCAM:
		return "CAM"
	default:
		return "unknown"
	}
}

// AudioToString converts AudioCodec enum to database-friendly string
func AudioToString(a AudioCodec) string {
	switch a {
	case AudioAtmos:
		return "Atmos"
	case AudioTrueHD:
		return "TrueHD"
	case AudioDTSX:
		return "DTS:X"
	case AudioDTSHDMA:
		return "DTS-HD MA"
	case AudioDTSHD:
		return "DTS-HD"
	case AudioDTS:
		return "DTS"
	case AudioEAC3:
		return "DD+"
	case AudioAC3:
		return "DD"
	case AudioAAC:
		return "AAC"
	default:
		return "unknown"
	}
}

// CodecToString extracts codec information from filename
func CodecToString(filename string) string {
	upper := strings.ToUpper(filename)

	if strings.Contains(upper, "AV1") {
		return "AV1"
	}
	if strings.Contains(upper, "HEVC") || strings.Contains(upper, "H.265") || strings.Contains(upper, "H265") || strings.Contains(upper, "X265") {
		return "x265"
	}
	if strings.Contains(upper, "H.264") || strings.Contains(upper, "H264") || strings.Contains(upper, "X264") || strings.Contains(upper, "AVC") {
		return "x264"
	}
	if strings.Contains(upper, "VP9") {
		return "VP9"
	}
	if strings.Contains(upper, "XVID") {
		return "XviD"
	}

	return "unknown"
}
