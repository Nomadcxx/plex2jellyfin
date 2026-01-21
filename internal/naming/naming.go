package naming

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type MovieInfo struct {
	Title string
	Year  string
}

type TVShowInfo struct {
	Title   string
	Year    string
	Season  int
	Episode int
}

var (
	yearRegex       = regexp.MustCompile(`\b(19|20)\d{2}\b`)
	yearParenRegex  = regexp.MustCompile(`\((\d{4})\)`)
	episodeSERegex  = regexp.MustCompile(`[Ss](\d{1,2})[Ee](\d{1,2})`)
	episodeXRegex   = regexp.MustCompile(`(\d{1,2})x(\d{1,2})`)
	releasePatterns []*regexp.Regexp
)

func init() {
	patterns := []string{
		`\b\d{3,4}[pi]\b`,
		`\b(4K|UHD)\b`,
		`\b(HDR10\+?|HDR|DoVi|DV)\b`,
		`\b(DTS[ -]?HD|DTS[ -]?X|DTS|TrueHD|Atmos|AAC|AC3|DD\+?|DDP|FLAC)\b`,
		`\bDDP?\d[ .]?\d\b`,
		`\b\d[ .]?\d\b`,
		`\b(BluRay|Blu-ray|BDRip|REMUX|WEB-DL|WEBDL|WEBRip|WEB)\b`,
		`\b(HDTV|DVDRip|DVD)\b`,
		`\b(AMZN|NF|ATVP|HULU|DSNP|MAX|PMTP)\b`,
		`\b[xh][ .]?264\b`,
		`\b[xh][ .]?265\b`,
		`\b(HEVC|AVC|AV1)\b`,
		`\b(PROPER|REPACK|iNTERNAL|LIMITED|EXTENDED)\b`,
		`\b(DUAL|DL|MULTI|DUB|SUB|SUBS)\b`,
		`\b(RARBG|YTS|YIFY|FLUX|ETHEL|Kitsune|NTb|CMRG|SPARKS|FGT)\b`,
		`\bv\d+\b`,
		`\[.*?\]`,
		`\b(8bit|10bit|12bit)\b`,
		// Note: release group suffix (-SPARKS, -postbot) handled separately in stripReleaseMarkers
		`\b[A-Z]{2,5}\d*$`,
	}

	releasePatterns = make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		releasePatterns = append(releasePatterns, regexp.MustCompile(`(?i)`+pattern))
	}
}

func IsMovieFilename(filename string) bool {
	baseName := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	return !episodeSERegex.MatchString(baseName) && !episodeXRegex.MatchString(baseName)
}

func IsTVEpisodeFilename(filename string) bool {
	return !IsMovieFilename(filename)
}

func HasYearInParentheses(filename string) bool {
	return yearParenRegex.MatchString(filename)
}

func ParseMovieName(filename string) (*MovieInfo, error) {
	baseName := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))

	info, err := parseMovieSimple(baseName)
	if err != nil {
		return parseMovieAdvanced(baseName)
	}

	if IsGarbageTitle(info.Title) {
		return parseMovieAdvanced(baseName)
	}

	return info, nil
}

func parseMovieSimple(baseName string) (*MovieInfo, error) {
	cleaned := stripReleaseMarkers(baseName)

	year := extractYear(baseName)

	if year != "" {
		cleaned = removeYear(cleaned, year)
	}

	cleaned = normalizeSpaces(cleaned)
	cleaned = strings.TrimSpace(cleaned)

	if cleaned == "" {
		return nil, fmt.Errorf("could not extract movie title from: %s", baseName)
	}

	return &MovieInfo{
		Title: cleaned,
		Year:  year,
	}, nil
}

func parseMovieAdvanced(baseName string) (*MovieInfo, error) {
	cleaned := CleanMovieName(baseName)
	if cleaned == "" {
		return nil, fmt.Errorf("could not extract movie title from: %s", baseName)
	}

	year := ExtractYearAdvanced(baseName)

	title := cleaned
	if year != "" && strings.HasSuffix(cleaned, " ("+year+")") {
		title = strings.TrimSuffix(cleaned, " ("+year+")")
	}

	return &MovieInfo{
		Title: title,
		Year:  year,
	}, nil
}

func ParseTVShowName(filename string) (*TVShowInfo, error) {
	baseName := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))

	season, episode, found := extractEpisodeInfo(baseName)
	if !found {
		return nil, fmt.Errorf("no episode information found in: %s", filename)
	}

	titlePart := extractTitleBeforeEpisode(baseName)

	titlePart = stripReleaseMarkers(titlePart)

	year := extractYear(baseName)
	if year != "" {
		titlePart = removeYear(titlePart, year)
	}

	titlePart = normalizeSpaces(titlePart)
	titlePart = strings.TrimSpace(titlePart)

	if titlePart == "" {
		return nil, fmt.Errorf("could not extract TV show title from: %s", filename)
	}

	return &TVShowInfo{
		Title:   titlePart,
		Year:    year,
		Season:  season,
		Episode: episode,
	}, nil
}

func NormalizeMovieName(title, year string) string {
	if year != "" {
		return fmt.Sprintf("%s (%s)", title, year)
	}
	return title
}

func NormalizeTVShowName(title, year string) string {
	if year != "" {
		return fmt.Sprintf("%s (%s)", title, year)
	}
	return title
}

func FormatMovieFilename(title, year, ext string) string {
	if year != "" {
		return fmt.Sprintf("%s (%s).%s", title, year, ext)
	}
	return fmt.Sprintf("%s.%s", title, ext)
}

func FormatSeasonFolder(season int) string {
	return fmt.Sprintf("Season %02d", season)
}

func FormatTVEpisodeFilename(title, year string, season, episode int, ext string) string {
	if year != "" {
		return fmt.Sprintf("%s (%s) S%02dE%02d.%s", title, year, season, episode, ext)
	}
	return fmt.Sprintf("%s S%02dE%02d.%s", title, season, episode, ext)
}

// releaseGroupSuffix matches release group tags at end of filename like "-SPARKS", "-postbot", "-SM737"
var releaseGroupSuffix = regexp.MustCompile(`(?i)-[A-Za-z0-9]+$`)

func stripReleaseMarkers(s string) string {
	// First, strip trailing release group suffix BEFORE replacing hyphens
	// This catches patterns like "-SPARKS", "-postbot", "-SM737", etc.
	// Keep stripping until no more match (handles chained groups like "-SPARKS-postbot")
	for {
		newS := releaseGroupSuffix.ReplaceAllString(s, "")
		if newS == s {
			break
		}
		s = newS
	}

	// Now replace separators with spaces
	s = strings.ReplaceAll(s, ".", " ")
	s = strings.ReplaceAll(s, "_", " ")
	s = strings.ReplaceAll(s, "-", " ")

	// Apply other release patterns
	for _, re := range releasePatterns {
		s = re.ReplaceAllString(s, " ")
	}

	return s
}

func extractYear(s string) string {
	match := yearParenRegex.FindStringSubmatch(s)
	if len(match) > 1 {
		return match[1]
	}

	matches := yearRegex.FindAllString(s, -1)
	if len(matches) > 0 {
		for i := len(matches) - 1; i >= 0; i-- {
			year := matches[i]
			if year >= "1900" && year <= "2099" {
				if year != "2160" && year != "1920" && year != "1440" && year != "1280" {
					return year
				}
			}
		}
	}

	return ""
}

func removeYear(s, year string) string {
	if year == "" {
		return s
	}

	s = strings.ReplaceAll(s, "("+year+")", " ")
	s = strings.ReplaceAll(s, "["+year+"]", " ")
	s = strings.ReplaceAll(s, " "+year+" ", " ")
	s = strings.ReplaceAll(s, "."+year+".", " ")

	return s
}

func extractEpisodeInfo(s string) (season, episode int, found bool) {
	match := episodeSERegex.FindStringSubmatch(s)
	if len(match) > 2 {
		season, _ = strconv.Atoi(match[1])
		episode, _ = strconv.Atoi(match[2])
		return season, episode, true
	}

	match = episodeXRegex.FindStringSubmatch(s)
	if len(match) > 2 {
		season, _ = strconv.Atoi(match[1])
		episode, _ = strconv.Atoi(match[2])
		return season, episode, true
	}

	return 0, 0, false
}

func extractTitleBeforeEpisode(s string) string {
	loc := episodeSERegex.FindStringIndex(s)
	if loc != nil {
		return s[:loc[0]]
	}

	loc = episodeXRegex.FindStringIndex(s)
	if loc != nil {
		return s[:loc[0]]
	}

	return s
}

func normalizeSpaces(s string) string {
	spaceRegex := regexp.MustCompile(`\s+`)
	return spaceRegex.ReplaceAllString(s, " ")
}
