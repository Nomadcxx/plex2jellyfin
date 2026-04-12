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
	yearRegex      = regexp.MustCompile(`\b(19|20)\d{2}\b`)
	yearParenRegex = regexp.MustCompile(`\((\d{4})\)`)
	episodeSERegex = regexp.MustCompile(`[Ss](\d{1,2})[Ee](\d{1,2})`)
	episodeXRegex  = regexp.MustCompile(`(\d{1,2})x(\d{1,2})`)
	// Date-based episode pattern: YYYY.MM.DD, YYYY-MM-DD, YYYY_MM_DD
	episodeDateRegex = regexp.MustCompile(`(19|20)\d{2}[.\-_](0[1-9]|1[0-2])[.\-_](0[1-9]|[12]\d|3[01])`)
	releasePatterns  []*regexp.Regexp
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
        // REMOVED: `\b[A-Z]{2,5}\d*$` - too broad, strips last word of short titles (e.g., "Pitt" from "The Pitt")
        // Release groups are already handled by the explicit group list above and releaseGroupSuffix regex
	}

	releasePatterns = make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		releasePatterns = append(releasePatterns, regexp.MustCompile(`(?i)`+pattern))
	}
}

func IsMovieFilename(filename string) bool {
	return !IsTVEpisodeFilename(filename)
}

func IsTVEpisodeFilename(filename string) bool {
	baseName := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))

	// Standard SxxExx pattern
	if episodeSERegex.MatchString(baseName) {
		return true
	}

	// NxN pattern
	if episodeXRegex.MatchString(baseName) {
		return true
	}

	// Date-based pattern (daily shows like The Daily Show, Colbert, SNL)
	if episodeDateRegex.MatchString(baseName) {
		return true
	}

	return false
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

// knownReleaseGroups is an exhaustive list of common release groups
// used to safely strip only known groups instead of greedy -Word$ patterns
var knownReleaseGroups = map[string]bool{
	"sparks": true, "postbot": true, "sm737": true, "flux": true, "ethel": true,
	"kitsune": true, "ntb": true, "cmrg": true, "fgt": true, "rarbg": true,
	"yts": true, "yify": true, "evo": true, "ion10": true, "tigole": true,
	"geckos": true, "stuttershit": true, "sadpanda": true, "grym": true,
	"demand": true, "nogrp": true, "mzabi": true, "hone": true, "cakes": true,
	"sujaidr": true, "ggez": true, "ggwp": true, "tbs": true, "lol": true,
	"dimension": true, "asap": true, "immerse": true, "avs": true, "savvy": true,
	"memento": true, "megusta": true, "fleet": true, "mtb": true, "deflate": true,
	"morpheus": true, "aac": true, "pahe": true, "rocketman": true, "nuked": true,
	"strife": true, "phoenix": true, "ghosts": true, "edith": true, "sonarr": true,
	"scene": true, "monkee": true, "ctl": true, "kings": true, "lazy": true,
	"pcok": true, "playready": true, "decibel": true, "epsilon": true,
	"sigma": true, "tepes": true, "truffle": true, "nero": true, "group": true,
	"amiable": true, "markii": true, "tabular": true, "tabularía": true, "tabularasa": true,
}

// qualityMarkerDetect detects if a string contains codec/quality markers,
// indicating that any trailing -Word is almost certainly a release group
var qualityMarkerDetect = regexp.MustCompile(`(?i)(x264|x265|h264|h265|hevc|avc|bluray|blu-ray|bdrip|remux|web-dl|webdl|webrip|\d{3,4}p|4k|uhd)`)

// releaseGroupSuffix matches release group tags at end of filename like "-SPARKS", "-postbot"
var releaseGroupSuffix = regexp.MustCompile(`(?i)-[A-Za-z0-9]+$`)

func stripReleaseMarkers(s string) string {
	// Phase 1: Strip known release group suffixes (safe — always strip these)
	for {
		idx := strings.LastIndex(s, "-")
		if idx < 0 || idx == len(s)-1 {
			break
		}
		candidate := strings.ToLower(s[idx+1:])
		if knownReleaseGroups[candidate] {
			s = s[:idx]
		} else {
			break
		}
	}

	// Phase 2: If quality/codec markers are present, any remaining -Word$ suffix
	// is almost certainly a release group, so strip it too.
	// This is safe because titles like "Doctor-Who" won't have codec markers after them.
	if qualityMarkerDetect.MatchString(s) {
		for {
			newS := releaseGroupSuffix.ReplaceAllString(s, "")
			if newS == s {
				break
			}
			s = newS
		}
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
