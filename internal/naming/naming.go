package naming

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// ErrParseFailed is returned when filename/path parsing cannot extract the
// information needed for organization (e.g. no episode markers, no title).
// Retry loops treat failures wrapping this sentinel as deterministic — not
// worth re-attempting until the source file changes.
var ErrParseFailed = errors.New("parse failed")

type MovieInfo struct {
	Title string
	Year  string
}

type TVShowInfo struct {
	Title       string
	Year        string
	Season      int
	Episode     int
	EpisodeDate string
}

var (
	yearRegex      = regexp.MustCompile(`\b(19|20)\d{2}\b`)
	yearParenRegex = regexp.MustCompile(`\((\d{4})\)`)
	episodeSERegex = regexp.MustCompile(`[Ss](\d{1,2})[Ee](\d{1,4})`)
	episodeXRegex  = regexp.MustCompile(`(\d{1,2})x(\d{1,2})`)
	episodeEPRegex = regexp.MustCompile(`(?i)\bEP[.\-_ ]?(\d{2,5})\b`)
	// Date-based episode pattern: YYYY.MM.DD, YYYY-MM-DD, YYYY_MM_DD
	episodeDateRegex = regexp.MustCompile(`\b((19|20)\d{2})[.\-_](0[1-9]|1[0-2])[.\-_](0[1-9]|[12]\d|3[01])\b`)
	releasePatterns  []*regexp.Regexp
)

func init() {
	patterns := []string{
		`\b\d{3,4}[pi]\b`,
		`\b(?:2160|1080|720|480)\b`,
		`\b(4K|UHD)\b`,
		`\bHDR10\+|\bHDR10(?:Plus)?(?:ITA|ENG)?\b|\b(HDR|DoVi|D[ .-]?V|SDR|HLG)\b`,
		// Codec glued to channel digits (must come BEFORE bare codec match so the
		// trailing channel digits get stripped together). Allows 1-3 digit
		// groups so all of these are stripped:
		//   AAC2.0, AAC5.1, DDP5.1, DD+5.1, MA5.1, AC3.5.1, EAC3.5.1
		//   EAC5.1, EAC7.1, EAC2.0 (TSRG-style shorthand: 3 dropped from EAC3)
		//   OPUS5.1, AV1.5.1, HEVC5.1 (release-group shorthand without separator)
		`\b(?:E?AC3?|AAC|DDP?|DD\+?|MA|OPUS|FLAC|AV1|HEVC|AVC|TrueHD|Atmos)\d(?:[. ]\d){0,2}\b`,
		`\b(DTS[ -]?HD|DTS[ -]?X|DTS|TrueHD|Atmos|AAC|AC3|EAC3|DD\+?|DDP|FLAC|OPUS)\b`,
		// Audio channels alone (require an explicit separator to avoid stripping
		// 2-digit standalone numbers like "28" in "28 Weeks Later").
		`\b\d[ .]\d\b`,
		// Channel-count tokens like "6CH", "8CH"
		`\b\d+CH\b`,
		`\bBlu[ .-]?Ray[ .-]+MA\b`,
		`\b(Blu[ .-]?Ray|BDRip|REMUX|WEB-DL|WEBDL|WEBRip|WEB|DCP)\b`,
		`\b(HDTV|HDRip|DVDRip|DVD)\b`,
		// Streaming rip sources (not release groups): Peacock=PCOK, etc.
		`\b(AMZN|NF|ATVP|HULU|DSNP|HMAX|MAX|PCOK|PMTP|TOD)\b`,
		`\b[xh][ .]?26[45]\b`,
		`\b(HEVC|AVC|AV1)\b`,
		`\b(PROPER|REPACK|iNTERNAL|LIMITED|EXTENDED|REMASTERED|REMASTER|Up[ .-]?Scaled)\b`,
		`\b(DUAL|DL|MULTI|DUB|DUBBED|RODUBBED|SUB|SUBS|VOSTFR|DCPRIP|HDLight|TrueFrench|VOST|VF)\b`,
		// Language/locale tags (e.g., iTA-ENG, FRE, MULTi). 3-letter codes
		// only — 2-letter codes (EN, NO, ES) collide too often with real
		// title words ("No Good Deed", "Es", "En").
		`\b(ITA|ENG|FRE|FRA|GER|DEU|ESP|SPA|POR|RUS|JPN|KOR|CHI|HIN|NORDiC|LATINO)\b`,
		`@\w+`,
		`\b(RARBG|YTS|YIFY|FLUX|ETHEL|Kitsune|NTb|CMRG|SPARKS|FGT|BZ|TSRG|LICDOM)\b`,
		`\bv\d+\b`,
		`\[.*?\]`,
		`\b(8|10|12)[ .]?bits?\b`,
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

	if hasUnsupportedMultiEpisodeRange(baseName) {
		return true
	}

	return findEpisodeMatch(baseName).found
}

func HasYearInParentheses(filename string) bool {
	return yearParenRegex.MatchString(filename)
}

func ParseMovieName(filename string) (*MovieInfo, error) {
	info, _, err := ParseMovieNameVerbose(filename)
	return info, err
}

// ParseMovieNameVerbose parses a movie filename and also returns the release
// metadata tokens (quality, codec, release group, etc.) that were stripped.
func ParseMovieNameVerbose(filename string) (*MovieInfo, []string, error) {
	baseName := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	tokens := collectStrippedTokens(baseName)
	info, err := parseMovieFromBaseName(baseName)
	return info, tokens, err
}

func parseMovieFromBaseName(baseName string) (*MovieInfo, error) {
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
		return nil, fmt.Errorf("%w: could not extract movie title from: %s", ErrParseFailed, baseName)
	}

	return &MovieInfo{
		Title: cleaned,
		Year:  year,
	}, nil
}

func parseMovieAdvanced(baseName string) (*MovieInfo, error) {
	cleaned := CleanMovieName(baseName)
	if cleaned == "" {
		return nil, fmt.Errorf("%w: could not extract movie title from: %s", ErrParseFailed, baseName)
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
	info, _, err := ParseTVShowNameVerbose(filename)
	return info, err
}

// ParseTVShowNameVerbose parses a TV episode filename and also returns the
// release metadata tokens (quality, codec, release group, etc.) that were stripped.
func ParseTVShowNameVerbose(filename string) (*TVShowInfo, []string, error) {
	baseName := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	tokens := collectStrippedTokens(baseName)
	info, err := parseTVShowFromBaseName(baseName, filename)
	return info, tokens, err
}

var seasonPackRegex = regexp.MustCompile(`(?i)(^|[._\s-])S(\d{1,2})($|[._\s-])`)

func IsTVSeasonPackName(name string) bool {
	_, err := ParseTVSeasonPackName(name)
	return err == nil
}

func ParseTVSeasonPackName(name string) (*TVSeasonPackInfo, error) {
	info, _, err := ParseTVSeasonPackNameVerbose(name)
	return info, err
}

func ParseTVSeasonPackNameVerbose(name string) (*TVSeasonPackInfo, []string, error) {
	baseName := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	tokens := collectStrippedTokens(baseName)

	if IsTVEpisodeFilename(baseName) {
		return nil, tokens, fmt.Errorf("%w: episode release is not a season pack: %s", ErrParseFailed, name)
	}

	match := seasonPackRegex.FindStringSubmatchIndex(baseName)
	if match == nil {
		return nil, tokens, fmt.Errorf("%w: no season-pack marker found in: %s", ErrParseFailed, name)
	}

	seasonText := baseName[match[4]:match[5]]
	season := 0
	if _, err := fmt.Sscanf(seasonText, "%d", &season); err != nil || season <= 0 {
		return nil, tokens, fmt.Errorf("%w: invalid season-pack marker in: %s", ErrParseFailed, name)
	}

	titlePart := baseName[:match[0]]
	titlePart = stripReleaseMarkers(titlePart)
	year := extractYear(baseName)
	if year != "" {
		titlePart = removeYear(titlePart, year)
		titlePart = strings.TrimSuffix(strings.TrimSpace(titlePart), year)
	}
	titlePart = normalizeSpaces(titlePart)
	titlePart = strings.TrimSpace(titlePart)
	if titlePart == "" || IsGarbageTitle(titlePart) {
		return nil, tokens, fmt.Errorf("%w: could not extract season-pack title from: %s", ErrParseFailed, name)
	}

	return &TVSeasonPackInfo{
		Title:  titlePart,
		Year:   year,
		Season: season,
	}, tokens, nil
}

func parseTVShowFromBaseName(baseName, filename string) (*TVShowInfo, error) {
	if hasUnsupportedMultiEpisodeRange(baseName) {
		return nil, fmt.Errorf("%w: unsupported multi-episode range in: %s", ErrParseFailed, filename)
	}

	episodeMatch := findEpisodeMatch(baseName)
	if !episodeMatch.found {
		return nil, fmt.Errorf("%w: no episode information found in: %s", ErrParseFailed, filename)
	}

	titlePart := baseName[:episodeMatch.loc[0]]

	titlePart = stripReleaseMarkers(titlePart)

	year := ""
	if episodeMatch.kind != "date" {
		year = extractYear(titlePart)
	}
	if year != "" {
		titlePart = removeYear(titlePart, year)
	}

	titlePart = normalizeSpaces(titlePart)
	titlePart = strings.TrimSpace(titlePart)

	if titlePart == "" {
		return nil, fmt.Errorf("%w: could not extract TV show title from: %s", ErrParseFailed, filename)
	}

	return &TVShowInfo{
		Title:       titlePart,
		Year:        year,
		Season:      episodeMatch.season,
		Episode:     episodeMatch.episode,
		EpisodeDate: episodeMatch.date,
	}, nil
}

func NormalizeMediaName(title, year string) string {
	title = titleCaseWithOrdinals(title)
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

func FormatTVEpisodeFilenameFromInfo(info *TVShowInfo, ext string) string {
	if info == nil {
		return ""
	}
	if info.EpisodeDate != "" {
		title := NormalizeMediaName(info.Title, info.Year)
		return fmt.Sprintf("%s %s.%s", title, info.EpisodeDate, ext)
	}
	return FormatTVEpisodeFilename(info.Title, info.Year, info.Season, info.Episode, ext)
}

// knownReleaseGroups is an exhaustive list of common release groups
// used to safely strip only known groups instead of greedy -Word$ patterns
var knownReleaseGroups = map[string]bool{
	"sparks": true, "postbot": true, "sm737": true, "flux": true, "ethel": true,
	"kitsune": true, "ntb": true, "cmrg": true, "fgt": true, "rarbg": true,
	"yts": true, "yify": true, "evo": true, "ion10": true, "tigole": true,
	"bz": true, "tsrg": true,
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
	"vostfr": true, "dcprip": true, "hmax": true, "remastered": true, "rodubbed": true,
	"hdlight": true, "truefrench": true, "french": true, "vost": true, "vf": true,
}

// qualityMarkerDetect detects if a string contains codec/quality markers,
// indicating that any trailing -Word is almost certainly a release group
var qualityMarkerDetect = regexp.MustCompile(`(?i)(x264|x265|h264|h265|hevc|avc|bluray|blu-ray|bdrip|remux|web-dl|webdl|webrip|\d{3,4}p|4k|uhd)`)

// releaseGroupSuffixToken matches the trailing segment after the last hyphen,
// e.g. "SPARKS", "postbot", or "Ben.The.Men".
var releaseGroupSuffixToken = regexp.MustCompile(`(?i)^[A-Za-z0-9]+(?:[._ ][A-Za-z0-9]+)*$`)
var spaceRegex = regexp.MustCompile(`\s+`)

func trailingReleaseGroupSuffix(s string) (string, bool) {
	idx := strings.LastIndex(s, "-")
	if idx < 0 || idx == len(s)-1 {
		return "", false
	}
	suffix := s[idx+1:]
	if !releaseGroupSuffixToken.MatchString(suffix) {
		return "", false
	}
	if yearRegex.MatchString(suffix) || qualityMarkerDetect.MatchString(suffix) {
		return "", false
	}
	return suffix, true
}

// collectStrippedTokens collects the release metadata tokens (quality, codec,
// release group, etc.) that stripReleaseMarkers would remove from baseName.
func collectStrippedTokens(baseName string) []string {
	var tokens []string
	seen := make(map[string]bool)

	add := func(tok string) {
		tok = strings.TrimSpace(tok)
		lower := strings.ToLower(tok)
		if tok != "" && !seen[lower] {
			seen[lower] = true
			tokens = append(tokens, tok)
		}
	}

	s := baseName

	// Phase 1: strip known release group suffixes from the right
	for {
		idx := strings.LastIndex(s, "-")
		if idx < 0 || idx == len(s)-1 {
			break
		}
		candidate := strings.ToLower(s[idx+1:])
		if knownReleaseGroups[candidate] {
			add(s[idx+1:])
			s = s[:idx]
		} else {
			break
		}
	}

	// Phase 2: if quality/codec markers are present, any remaining -Word$ suffix
	// is almost certainly a release group
	if qualityMarkerDetect.MatchString(s) {
		for {
			suffix, ok := trailingReleaseGroupSuffix(s)
			if !ok {
				break
			}
			add(suffix)
			newS := strings.TrimSuffix(s, "-"+suffix)
			if newS == s {
				break
			}
			s = newS
		}
	}

	// Apply release patterns to the original baseName to catch quality/codec tokens
	for _, re := range releasePatterns {
		for _, m := range re.FindAllString(baseName, -1) {
			add(m)
		}
	}

	return tokens
}

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
			suffix, ok := trailingReleaseGroupSuffix(s)
			if !ok {
				break
			}
			newS := strings.TrimSuffix(s, "-"+suffix)
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

type episodeMatch struct {
	season  int
	episode int
	loc     []int
	kind    string
	date    string
	found   bool
}

func hasUnsupportedMultiEpisodeRange(s string) bool {
	for _, loc := range episodeSERegex.FindAllStringSubmatchIndex(s, -1) {
		if len(loc) < 6 || loc[5] < 0 || loc[5] > len(s) {
			continue
		}
		if hasEpisodeRangeSuffix(s[loc[5]:]) {
			return true
		}
	}
	return false
}

func hasEpisodeRangeSuffix(s string) bool {
	if s == "" {
		return false
	}
	if s[0] == 'E' || s[0] == 'e' {
		_, ok := consumeEpisodeRangeNumber(s[1:])
		return ok
	}
	if s[0] != '-' {
		return false
	}
	rest := s[1:]
	if rest == "" {
		return false
	}
	if rest[0] == 'E' || rest[0] == 'e' {
		rest = rest[1:]
	}
	_, ok := consumeEpisodeRangeNumber(rest)
	return ok
}

func consumeEpisodeRangeNumber(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	end := 0
	for end < len(s) && end < 4 && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0, false
	}
	if end < len(s) && (s[end] == 'p' || s[end] == 'P' || s[end] == 'i' || s[end] == 'I') {
		return 0, false
	}
	episode, err := strconv.Atoi(s[:end])
	if err != nil || episode <= 0 {
		return 0, false
	}
	switch episode {
	case 480, 576, 720, 1080, 2160:
		return 0, false
	}
	return end, true
}

func findEpisodeMatch(s string) episodeMatch {
	match := episodeSERegex.FindStringSubmatch(s)
	if len(match) > 2 {
		season, _ := strconv.Atoi(match[1])
		episode, _ := strconv.Atoi(match[2])
		return episodeMatch{
			season:  season,
			episode: episode,
			loc:     episodeSERegex.FindStringIndex(s),
			kind:    "season_episode",
			found:   true,
		}
	}

	match = episodeXRegex.FindStringSubmatch(s)
	if len(match) > 2 {
		season, _ := strconv.Atoi(match[1])
		episode, _ := strconv.Atoi(match[2])
		return episodeMatch{
			season:  season,
			episode: episode,
			loc:     episodeXRegex.FindStringIndex(s),
			kind:    "x",
			found:   true,
		}
	}

	match = episodeEPRegex.FindStringSubmatch(s)
	if len(match) > 1 {
		episode, _ := strconv.Atoi(match[1])
		return episodeMatch{
			season:  1,
			episode: episode,
			loc:     episodeEPRegex.FindStringIndex(s),
			kind:    "absolute",
			found:   true,
		}
	}

	match = episodeDateRegex.FindStringSubmatch(s)
	if len(match) > 4 {
		season, _ := strconv.Atoi(match[1])
		month, _ := strconv.Atoi(match[3])
		day, _ := strconv.Atoi(match[4])
		return episodeMatch{
			season:  season,
			episode: month*100 + day,
			loc:     episodeDateRegex.FindStringIndex(s),
			kind:    "date",
			date:    fmt.Sprintf("%04d-%02d-%02d", season, month, day),
			found:   true,
		}
	}

	return episodeMatch{}
}

func extractEpisodeInfo(s string) (season, episode int, found bool) {
	match := findEpisodeMatch(s)
	return match.season, match.episode, match.found
}

func extractTitleBeforeEpisode(s string) string {
	match := findEpisodeMatch(s)
	if match.found {
		return s[:match.loc[0]]
	}

	return s
}

func normalizeSpaces(s string) string {
	return spaceRegex.ReplaceAllString(s, " ")
}
