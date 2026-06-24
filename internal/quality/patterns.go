package quality

import (
	"path/filepath"
	"regexp"
	"strings"
)

var (
	// Resolution patterns
	resolution480p  = regexp.MustCompile(`(?i)\b480[pi]\b`)
	resolution576p  = regexp.MustCompile(`(?i)\b576[pi]\b`)
	resolution720p  = regexp.MustCompile(`(?i)\b720[pi]\b`)
	resolution1080p = regexp.MustCompile(`(?i)\b1080[pi]\b`)
	resolution2160p = regexp.MustCompile(`(?i)\b(2160[pi]|4K|UHD)\b`)
	resolution4320p = regexp.MustCompile(`(?i)\b(4320[pi]|8K)\b`)

	// Source patterns - order matters for matching
	remuxPattern  = regexp.MustCompile(`(?i)\bREMUX\b`)
	blurayPattern = regexp.MustCompile(`(?i)\b(BluRay|Blu-Ray|BDRip|BRRip|BD)\b`)
	webdlPattern  = regexp.MustCompile(`(?i)\b(WEB-DL|WEBDL|WEB\.DL)\b`)
	webripPattern = regexp.MustCompile(`(?i)\b(WEBRip|WEB-Rip|WEB)\b`)
	hdtvPattern   = regexp.MustCompile(`(?i)\b(HDTV|PDTV|DSR)\b`)
	dvdripPattern = regexp.MustCompile(`(?i)\b(DVDRip|DVD-Rip|DVD)\b`)
	dvdscrPattern = regexp.MustCompile(`(?i)\b(DVDScr|DVD-Scr|DVDSCREENER)\b`)
	tcPattern     = regexp.MustCompile(`(?i)\b(TC|TELECINE)\b`)
	tsPattern     = regexp.MustCompile(`(?i)\b(TS|TELESYNC|HDTS)\b`)
	camPattern    = regexp.MustCompile(`(?i)\b(CAM|HDCAM|CAMRip)\b`)

	// HDR patterns
	dolbyVisionPattern = regexp.MustCompile(`(?i)\b(DV|DoVi|Dolby\.?Vision)\b`)
	hdr10PlusPattern   = regexp.MustCompile(`(?i)(HDR10\+|HDR10Plus)`)
	hdr10Pattern       = regexp.MustCompile(`(?i)\b(HDR10|HDR)\b`)
	hlgPattern         = regexp.MustCompile(`(?i)\bHLG\b`)

	// Audio patterns
	atmosPattern   = regexp.MustCompile(`(?i)\bAtmos\b`)
	truehdPattern  = regexp.MustCompile(`(?i)\b(TrueHD|True-HD)\b`)
	dtsxPattern    = regexp.MustCompile(`(?i)\b(DTS-X|DTSX)\b`)
	dtshdmaPattern = regexp.MustCompile(`(?i)\b(DTS-HD\.?MA|DTS-HD\.Master\.Audio)\b`)
	dtshdPattern   = regexp.MustCompile(`(?i)\b(DTS-HD|DTSHD)\b`)
	dtsPattern     = regexp.MustCompile(`(?i)\bDTS\b`)
	eac3Pattern    = regexp.MustCompile(`(?i)(EAC3|E-AC-3|DD\+|DDP\d|Dolby\.?Digital\.?Plus)`)
	ac3Pattern     = regexp.MustCompile(`(?i)\b(AC3|AC-3|DD\b|Dolby\.?Digital)\b`)
	aacPattern     = regexp.MustCompile(`(?i)\bAAC\b`)

	// Other patterns
	threeDPattern   = regexp.MustCompile(`(?i)\b(3D|SBS|HSBS|OU|HOU)\b`)
	properPattern   = regexp.MustCompile(`(?i)\b(PROPER|REPACK|RERIP)\b`)
	extendedPattern = regexp.MustCompile(`(?i)\b(EXTENDED|UNCUT|UNRATED|DC|DIRECTORS\.?CUT)\b`)
)

func parseResolution(s string) Resolution {
	switch {
	case resolution4320p.MatchString(s):
		return Resolution4320p
	case resolution2160p.MatchString(s):
		return Resolution2160p
	case resolution1080p.MatchString(s):
		return Resolution1080p
	case resolution720p.MatchString(s):
		return Resolution720p
	case resolution576p.MatchString(s):
		return Resolution576p
	case resolution480p.MatchString(s):
		return Resolution480p
	default:
		return ResolutionUnknown
	}
}

func parseSource(s string) Source {
	switch {
	case remuxPattern.MatchString(s):
		return SourceREMUX
	case blurayPattern.MatchString(s):
		return SourceBluRay
	case webdlPattern.MatchString(s):
		return SourceWEBDL
	case webripPattern.MatchString(s):
		return SourceWEBRip
	case hdtvPattern.MatchString(s):
		return SourceHDTV
	case dvdripPattern.MatchString(s):
		return SourceDVDRip
	case dvdscrPattern.MatchString(s):
		return SourceDVDScr
	case tcPattern.MatchString(s):
		return SourceTC
	case tsPattern.MatchString(s):
		return SourceTS
	case camPattern.MatchString(s):
		return SourceCAM
	default:
		return SourceUnknown
	}
}

func parseHDR(s string) HDRFormat {
	switch {
	case dolbyVisionPattern.MatchString(s):
		return DolbyVision
	case hdr10PlusPattern.MatchString(s):
		return HDR10Plus
	case hdr10Pattern.MatchString(s):
		return HDR10
	case hlgPattern.MatchString(s):
		return HLG
	default:
		return HDRNone
	}
}

func parseAudio(s string) AudioCodec {
	switch {
	case atmosPattern.MatchString(s):
		return AudioAtmos
	case truehdPattern.MatchString(s):
		return AudioTrueHD
	case dtsxPattern.MatchString(s):
		return AudioDTSX
	case dtshdmaPattern.MatchString(s):
		return AudioDTSHDMA
	case dtshdPattern.MatchString(s):
		return AudioDTSHD
	case dtsPattern.MatchString(s):
		return AudioDTS
	case eac3Pattern.MatchString(s):
		return AudioEAC3
	case ac3Pattern.MatchString(s):
		return AudioAC3
	case aacPattern.MatchString(s):
		return AudioAAC
	default:
		return AudioUnknown
	}
}

func parse3D(s string) bool {
	return threeDPattern.MatchString(s)
}

func parseProper(s string) bool {
	return properPattern.MatchString(s)
}

func parseExtended(s string) bool {
	return extendedPattern.MatchString(s)
}

// CompareFiles compares quality of two filenames and returns which is better.
// Returns:
//
//	-1 if file1 is lower quality
//	 0 if equal
//	+1 if file1 is higher quality
func CompareFiles(file1, file2 string) int {
	q1 := Parse(file1)
	q2 := Parse(file2)
	return q1.Compare(q2)
}

// IsBetterFile returns true if newFile is higher quality than existingFile.
func IsBetterFile(newFile, existingFile string) bool {
	return CompareFiles(newFile, existingFile) > 0
}

// GetQualityString returns a human-readable quality string for a filename.
func GetQualityString(filename string) string {
	return Parse(filename).String()
}

// ParseFromPath extracts quality from a full file path.
func ParseFromPath(path string) *QualityInfo {
	// Check both filename and parent directory for quality info
	filename := strings.TrimSuffix(path, "."+getExt(path))

	info := Parse(filename)

	// If no source detected in filename, check parent dir
	if info.Source == SourceUnknown {
		parts := strings.Split(path, "/")
		if len(parts) > 1 {
			parentInfo := Parse(parts[len(parts)-2])
			if parentInfo.Source != SourceUnknown {
				info.Source = parentInfo.Source
				info.Score = info.ComputeScore()
			}
		}
	}

	return info
}

func getExt(path string) string {
	return strings.TrimPrefix(filepath.Ext(path), ".")
}
