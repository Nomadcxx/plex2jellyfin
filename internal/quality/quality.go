// Package quality provides media quality detection and comparison.
// It parses quality indicators from filenames (resolution, source, HDR, audio)
// and provides scoring to determine which version of a file is higher quality.
package quality

import (
	"path/filepath"
	"strings"
)

// Source represents the media source type, ordered by quality.
type Source int

const (
	SourceUnknown Source = iota
	SourceCAM            // 5 - Cam recordings (lowest)
	SourceTS             // 10 - Telesync
	SourceTC             // 15 - Telecine
	SourceDVDScr         // 20 - DVD Screener
	SourceDVDRip         // 25 - DVD Rip
	SourceHDTV           // 30 - HDTV capture
	SourceWEBRip         // 35 - Web stream capture
	SourceWEBDL          // 40 - Web download (direct)
	SourceBluRay         // 50 - BluRay encode
	SourceREMUX          // 60 - BluRay remux (highest)
)

// Resolution represents video resolution.
type Resolution int

const (
	ResolutionUnknown Resolution = 0
	Resolution480p    Resolution = 480
	Resolution576p    Resolution = 576
	Resolution720p    Resolution = 720
	Resolution1080p   Resolution = 1080
	Resolution2160p   Resolution = 2160 // 4K
	Resolution4320p   Resolution = 4320 // 8K
)

// HDRFormat represents HDR format type.
type HDRFormat int

const (
	HDRNone HDRFormat = iota
	HDR10
	HDR10Plus
	DolbyVision
	HLG
)

// AudioCodec represents audio codec quality tier.
type AudioCodec int

const (
	AudioUnknown AudioCodec = iota
	AudioAAC                // Basic
	AudioAC3                // Dolby Digital
	AudioEAC3               // Dolby Digital Plus
	AudioDTS                // DTS
	AudioDTSHD              // DTS-HD
	AudioDTSHDMA            // DTS-HD Master Audio
	AudioDTSX               // DTS:X
	AudioTrueHD             // Dolby TrueHD
	AudioAtmos              // Dolby Atmos
)

// QualityInfo contains parsed quality information from a filename.
type QualityInfo struct {
	Source     Source
	Resolution Resolution
	HDR        HDRFormat
	Audio      AudioCodec
	Is3D       bool
	IsProper   bool   // PROPER/REPACK release
	IsExtended bool   // Extended/Director's cut
	Score      int    // Computed overall score
	Raw        string // Original filename for debugging
}

// Parse extracts quality information from a filename.
func Parse(filename string) *QualityInfo {
	baseName := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	upper := strings.ToUpper(baseName)

	info := &QualityInfo{
		Raw: filename,
	}

	info.Resolution = parseResolution(upper)
	info.Source = parseSource(upper)
	info.HDR = parseHDR(upper)
	info.Audio = parseAudio(upper)
	info.Is3D = parse3D(upper)
	info.IsProper = parseProper(upper)
	info.IsExtended = parseExtended(upper)

	info.Score = info.ComputeScore()

	return info
}

// ComputeScore calculates an overall quality score.
// Higher score = better quality.
func (q *QualityInfo) ComputeScore() int {
	score := 0

	// Source scoring (major factor)
	switch q.Source {
	case SourceCAM:
		score += 5
	case SourceTS:
		score += 10
	case SourceTC:
		score += 15
	case SourceDVDScr:
		score += 20
	case SourceDVDRip:
		score += 25
	case SourceHDTV:
		score += 30
	case SourceWEBRip:
		score += 35
	case SourceWEBDL:
		score += 40
	case SourceBluRay:
		score += 50
	case SourceREMUX:
		score += 60
	default:
		score += 20 // Unknown defaults to mid-low
	}

	// Resolution scoring (major factor)
	switch q.Resolution {
	case Resolution480p:
		score += 5
	case Resolution576p:
		score += 7
	case Resolution720p:
		score += 15
	case Resolution1080p:
		score += 25
	case Resolution2160p:
		score += 40
	case Resolution4320p:
		score += 50
	default:
		score += 10 // Unknown defaults to low
	}

	// HDR scoring (moderate factor)
	switch q.HDR {
	case HDR10:
		score += 10
	case HDR10Plus:
		score += 12
	case DolbyVision:
		score += 15
	case HLG:
		score += 8
	}

	// Audio scoring (minor factor)
	switch q.Audio {
	case AudioAAC:
		score += 2
	case AudioAC3:
		score += 4
	case AudioEAC3:
		score += 5
	case AudioDTS:
		score += 6
	case AudioDTSHD:
		score += 8
	case AudioDTSHDMA:
		score += 10
	case AudioDTSX:
		score += 11
	case AudioTrueHD:
		score += 12
	case AudioAtmos:
		score += 15
	}

	// Bonuses
	if q.IsProper {
		score += 3 // PROPER releases fix issues
	}
	if q.IsExtended {
		score += 2 // Extended versions preferred
	}

	return score
}

// Compare compares two QualityInfo instances.
// Returns:
//
//	-1 if q is lower quality than other
//	 0 if equal quality
//	+1 if q is higher quality than other
func (q *QualityInfo) Compare(other *QualityInfo) int {
	if other == nil {
		return 1
	}

	if q.Score > other.Score {
		return 1
	}
	if q.Score < other.Score {
		return -1
	}
	return 0
}

// IsBetterThan returns true if q is higher quality than other.
func (q *QualityInfo) IsBetterThan(other *QualityInfo) bool {
	return q.Compare(other) > 0
}

// String returns a human-readable quality description.
func (q *QualityInfo) String() string {
	parts := []string{}

	// Resolution
	switch q.Resolution {
	case Resolution480p:
		parts = append(parts, "480p")
	case Resolution576p:
		parts = append(parts, "576p")
	case Resolution720p:
		parts = append(parts, "720p")
	case Resolution1080p:
		parts = append(parts, "1080p")
	case Resolution2160p:
		parts = append(parts, "2160p")
	case Resolution4320p:
		parts = append(parts, "8K")
	}

	// Source
	if source := q.Source.String(); source != "Unknown" {
		parts = append(parts, source)
	}

	// HDR
	switch q.HDR {
	case HDR10:
		parts = append(parts, "HDR10")
	case HDR10Plus:
		parts = append(parts, "HDR10+")
	case DolbyVision:
		parts = append(parts, "DV")
	case HLG:
		parts = append(parts, "HLG")
	}

	if len(parts) == 0 {
		return "Unknown"
	}

	return strings.Join(parts, " ")
}

// SourceString returns the source as a string.
func (s Source) String() string {
	switch s {
	case SourceCAM:
		return "CAM"
	case SourceTS:
		return "TS"
	case SourceTC:
		return "TC"
	case SourceDVDScr:
		return "DVDScr"
	case SourceDVDRip:
		return "DVDRip"
	case SourceHDTV:
		return "HDTV"
	case SourceWEBRip:
		return "WEBRip"
	case SourceWEBDL:
		return "WEB-DL"
	case SourceBluRay:
		return "BluRay"
	case SourceREMUX:
		return "REMUX"
	default:
		return "Unknown"
	}
}
