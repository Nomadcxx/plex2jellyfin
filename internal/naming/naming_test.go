package naming

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseMovieName(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantTitle string
		wantYear  string
		wantErr   bool
	}{
		{
			name:      "Standard format",
			input:     "Dune Part Two (2024).mkv",
			wantTitle: "Dune Part Two",
			wantYear:  "2024",
			wantErr:   false,
		},
		{
			name:      "With release markers",
			input:     "Dune.Part.Two.2024.1080p.BluRay.x264-GROUP.mkv",
			wantTitle: "Dune Part Two",
			wantYear:  "2024",
			wantErr:   false,
		},
		{
			name:      "With space separated Blu ray marker",
			input:     "Is This Thing On Blu ray (2025).mp4",
			wantTitle: "Is This Thing On",
			wantYear:  "2025",
			wantErr:   false,
		},
		{
			name:      "With hyphenated Blu-ray marker after bare year",
			input:     "Is.This.Thing.On.2025.2160p.UHD.Blu-ray.Remux.DV.HDR.HEVC.TrueHD.Atmos.7.1-CiNEPHiLES.mp4",
			wantTitle: "Is This Thing On",
			wantYear:  "2025",
			wantErr:   false,
		},
		{
			name:      "Strips language tags (iTA-ENG)",
			input:     "Nightbitch.2024.iTA-ENG.WEB-DL.1080p.x264-CYBER.mkv",
			wantTitle: "Nightbitch",
			wantYear:  "2024",
			wantErr:   false,
		},
		{
			name:      "Strips DCP marker",
			input:     "The.Devil.Wears.Prada.2.2026.1080p.DCP.WEBRIP.AC3.x264-AOC.mkv",
			wantTitle: "The Devil Wears Prada 2",
			wantYear:  "2026",
			wantErr:   false,
		},
		{
			name:      "Strips full HDR10 plus marker",
			input:     "Tom.Clancys.Jack.Ryan.Ghost.War.2026.2160p.AMZN.WEB-DL.HDR10+.H.265.10bit.DDP5.1.Atmos-UBWEB.mkv",
			wantTitle: "Tom Clancys Jack Ryan Ghost War",
			wantYear:  "2026",
			wantErr:   false,
		},
		{
			name:      "Strips HDR10Plus marker",
			input:     "Highest.2.Lowest.2025.2160p.BDRip.TrueHD.7.1.Atmos.DV.HDR10Plus.x265.10bit-MarkII.mkv",
			wantTitle: "Highest 2 Lowest",
			wantYear:  "2025",
			wantErr:   false,
		},
		{
			name:      "Strips HDRip source marker",
			input:     "Masters.of.the.Universe.2026.iNTERNAL.1080p.10bit.HDRip.2CH.x265.HEVC-PSA.mkv",
			wantTitle: "Masters of the Universe",
			wantYear:  "2026",
			wantErr:   false,
		},
		{
			name:      "Preserves roman numeral sequel",
			input:     "Mortal.Kombat.II.2026.1080p.WEBRip.x264.AAC-LAMA.mp4",
			wantTitle: "Mortal Kombat II",
			wantYear:  "2026",
			wantErr:   false,
		},
		{
			name:      "Strips HMAX service marker",
			input:     "Final.Destination.5.2011.NORDiC.1080p.HMAX.WEB-DL.H.265.DDP5.1-NoTrace.mkv",
			wantTitle: "Final Destination 5",
			wantYear:  "2011",
			wantErr:   false,
		},
		{
			name:      "Strips remastered edition marker",
			input:     "Eraser.1996.REMASTERED.1080p.BluRay.HEVC.x265.5.1-BONE.mkv",
			wantTitle: "Eraser",
			wantYear:  "1996",
			wantErr:   false,
		},
		{
			name:      "Strips RoDubbed language marker",
			input:     "Ratatouille.2007.720p.BluRay.RoDubbed.DD.5.1.x264-SPHD.mkv",
			wantTitle: "Ratatouille",
			wantYear:  "2007",
			wantErr:   false,
		},
		{
			name:      "Strips dotted Dolby Vision marker",
			input:     "Kraven.the.Hunter.2024.2160p.H.265.HDR.D.V.iTA.EnG.EAC3.Sub.iTA.EnG-MIRCrew.mkv",
			wantTitle: "Kraven the Hunter",
			wantYear:  "2024",
			wantErr:   false,
		},
		{
			name:      "Strips glued HDR10 language and release group markers",
			input:     "The.Island.2005.UpScaled.2160p.H265.10.bit.DV.HDR10ita.eng.AC3.5.1.sub.ita.eng.Licdom.mkv",
			wantTitle: "The Island",
			wantYear:  "2005",
			wantErr:   false,
		},
		{
			name:      "Strips spaced Blu ray and DTS MA marker",
			input:     "Look Away Blu ray MA (2018).mkv",
			wantTitle: "Look Away",
			wantYear:  "2018",
			wantErr:   false,
		},
		{
			name:      "Strips bare 1080 resolution marker",
			input:     "Concrete.Cowboy.2021.1080.NF.WEB-DL.DDP5.1.x264-CMRG.mkv",
			wantTitle: "Concrete Cowboy",
			wantYear:  "2021",
			wantErr:   false,
		},
		{
			name:      "Preserves all as title word",
			input:     "Bones.And.All.2022.2160p.4K.WEB.x265.10bit.AAC5.1-YTS.MX.mkv",
			wantTitle: "Bones And All",
			wantYear:  "2022",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseMovieName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseMovieName() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got.Title != tt.wantTitle {
				t.Errorf("ParseMovieName() title = %v, want %v", got.Title, tt.wantTitle)
			}
			if got.Year != tt.wantYear {
				t.Errorf("ParseMovieName() year = %v, want %v", got.Year, tt.wantYear)
			}
		})
	}
}

func TestParseTVShowName(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantTitle  string
		wantYear   string
		wantSeason int
		wantEp     int
		wantDate   string
		wantErr    bool
	}{
		{
			name:       "Standard format",
			input:      "Breaking Bad S01E01.mkv",
			wantTitle:  "Breaking Bad",
			wantYear:   "",
			wantSeason: 1,
			wantEp:     1,
			wantErr:    false,
		},
		{
			name:       "With release markers",
			input:      "Breaking.Bad.S01E01.1080p.WEB-DL.x264.mkv",
			wantTitle:  "Breaking Bad",
			wantYear:   "",
			wantSeason: 1,
			wantEp:     1,
			wantErr:    false,
		},
		{
			name:       "Release year after episode marker is not series year",
			input:      "Upload.S04E03.2025.1080p.Amazon.WEB-DL.AVC.DDP.5.1-DBTV.mkv",
			wantTitle:  "Upload",
			wantYear:   "",
			wantSeason: 4,
			wantEp:     3,
			wantErr:    false,
		},
		{
			name:       "Netflix release year after episode marker is not series year",
			input:      "Worst.Ex.Ever.S02E01.2026.1080p.NF.WEB-DL.DDP5.1.Atmos.H.264-HDSWEB.mkv",
			wantTitle:  "Worst Ex Ever",
			wantYear:   "",
			wantSeason: 2,
			wantEp:     1,
			wantErr:    false,
		},
		{
			name:       "Series year before episode marker is preserved",
			input:      "Show.2022.S01E01.1080p.WEB-DL.mkv",
			wantTitle:  "Show",
			wantYear:   "2022",
			wantSeason: 1,
			wantEp:     1,
			wantErr:    false,
		},
		{
			name:       "Date-based daily show episode",
			input:      "The.Daily.Show.2026.04.20.Annalena.Baerbock.1080p.WEB.h264-EDITH.mkv",
			wantTitle:  "The Daily Show",
			wantYear:   "",
			wantSeason: 2026,
			wantEp:     420,
			wantDate:   "2026-04-20",
			wantErr:    false,
		},
		{
			name:       "Absolute EP numbering",
			input:      "One.Piece.EP1156.Episode.1156.1080p.NF.WEB-DL.JPN.AAC2.0.H.264.MSubs-ToonsHub.mkv",
			wantTitle:  "One Piece",
			wantYear:   "",
			wantSeason: 1,
			wantEp:     1156,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTVShowName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseTVShowName() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got.Title != tt.wantTitle {
				t.Errorf("ParseTVShowName() title = %v, want %v", got.Title, tt.wantTitle)
			}
			if got.Year != tt.wantYear {
				t.Errorf("ParseTVShowName() year = %v, want %v", got.Year, tt.wantYear)
			}
			if got.Season != tt.wantSeason {
				t.Errorf("ParseTVShowName() season = %v, want %v", got.Season, tt.wantSeason)
			}
			if got.Episode != tt.wantEp {
				t.Errorf("ParseTVShowName() episode = %v, want %v", got.Episode, tt.wantEp)
			}
			if got.EpisodeDate != tt.wantDate {
				t.Errorf("ParseTVShowName() episode date = %v, want %v", got.EpisodeDate, tt.wantDate)
			}
		})
	}
}

func TestParseTVSeasonPackName(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantTitle  string
		wantYear   string
		wantSeason int
		wantErr    bool
	}{
		{
			name:       "dotted season pack",
			input:      "Supergirl.S03.1080p.BluRay.x264-YELLOWBiRD",
			wantTitle:  "Supergirl",
			wantSeason: 3,
		},
		{
			name:       "spaced season pack",
			input:      "Obliterated S01 1080p NF WEB-DL DDP5 1 Atmos H 264-FLUX. unpack",
			wantTitle:  "Obliterated",
			wantSeason: 1,
		},
		{
			name:       "season pack with year",
			input:      "Worst.Ex.Ever.2026.S02.1080p.NF.WEB-DL",
			wantTitle:  "Worst Ex Ever",
			wantYear:   "2026",
			wantSeason: 2,
		},
		{
			name:    "episode is not season pack",
			input:   "Supergirl.S03E01.1080p.BluRay.x264-YELLOWBiRD.mkv",
			wantErr: true,
		},
		{
			name:    "movie is not season pack",
			input:   "The.Mummy.2026.720p.WEB.h264-JFF.mkv",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, err := ParseTVSeasonPackNameVerbose(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseTVSeasonPackNameVerbose(%q) expected error, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseTVSeasonPackNameVerbose(%q) unexpected error: %v", tt.input, err)
			}
			if got.Title != tt.wantTitle {
				t.Errorf("Title = %q, want %q", got.Title, tt.wantTitle)
			}
			if got.Year != tt.wantYear {
				t.Errorf("Year = %q, want %q", got.Year, tt.wantYear)
			}
			if got.Season != tt.wantSeason {
				t.Errorf("Season = %d, want %d", got.Season, tt.wantSeason)
			}
		})
	}
}

func TestFormatTVEpisodeFilenameFromInfo_DateBased(t *testing.T) {
	info := &TVShowInfo{
		Title:       "The Daily Show",
		Season:      2026,
		Episode:     420,
		EpisodeDate: "2026-04-20",
	}

	got := FormatTVEpisodeFilenameFromInfo(info, "mkv")
	want := "The Daily Show 2026-04-20.mkv"
	if got != want {
		t.Errorf("FormatTVEpisodeFilenameFromInfo() = %q, want %q", got, want)
	}
}

func TestIsTVEpisodeFilename_DatePatterns(t *testing.T) {
	tests := []struct {
		filename string
		want     bool
	}{
		// Date patterns - should be TV
		{"The.Daily.Show.2026.01.22.Guest.Name.1080p.WEB.mkv", true},
		{"Colbert.2026-01-22.1080p.WEB.mkv", true},
		{"SNL.2026_01_22.Host.1080p.mkv", true},
		{"Last.Week.Tonight.2026.01.19.1080p.mkv", true},

		// Standard patterns still work
		{"Show.S01E05.mkv", true},
		{"Show.1x05.mkv", true},
		{"One.Piece.EP1156.Episode.1156.1080p.NF.WEB-DL.mkv", true},

		// Movies - should not be TV
		{"Movie.Name.2026.1080p.mkv", false},
		{"Another.Movie.2024.BluRay.mkv", false},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got := IsTVEpisodeFilename(tt.filename)
			if got != tt.want {
				t.Errorf("IsTVEpisodeFilename(%q) = %v, want %v", tt.filename, got, tt.want)
			}
		})
	}
}

func TestParseTVShowFromPath_AbsoluteEpisodeNotObfuscated(t *testing.T) {
	path := "/mnt/NVME3/Sabnzbd/complete/tv/One.Piece.EP1157.Episode.1157.1080p.CR.WEB-DL.JPN.AAC2.0.H.264.ESub-ToonsHub/One.Piece.EP1157.Episode.1157.1080p.CR.WEB-DL.JPN.AAC2.0.H.264.ESub-ToonsHub.mkv"
	filename := filepath.Base(path)

	if IsObfuscatedFilename(filename) {
		t.Fatalf("IsObfuscatedFilename(%q) = true, want false", filename)
	}

	got, err := ParseTVShowFromPath(path)
	if err != nil {
		t.Fatalf("ParseTVShowFromPath() unexpected error: %v", err)
	}
	if got.Title != "One Piece" {
		t.Errorf("ParseTVShowFromPath() title = %q, want %q", got.Title, "One Piece")
	}
	if got.Season != 1 {
		t.Errorf("ParseTVShowFromPath() season = %d, want 1", got.Season)
	}
	if got.Episode != 1157 {
		t.Errorf("ParseTVShowFromPath() episode = %d, want 1157", got.Episode)
	}
}

func TestParseTVShowFromPath_RejectsUnsupportedMultiEpisodeRange(t *testing.T) {
	path := "/downloads/tv/Maisy.S01E061-062-063-064.Fish-Hiccups-Ice-Puzzle.480p.WEB-DL.AAC2.0.x264/Maisy.S01E061-062-063-064.Fish-Hiccups-Ice-Puzzle.480p.WEB-DL.AAC2.0.x264.mkv"

	_, err := ParseTVShowFromPath(path)
	if err == nil {
		t.Fatal("ParseTVShowFromPath() expected error for unsupported multi-episode range, got nil")
	}
	if !errors.Is(err, ErrParseFailed) {
		t.Fatalf("ParseTVShowFromPath() error = %v, want ErrParseFailed", err)
	}
}

func TestIsTVEpisodeFromPath_SourceHint(t *testing.T) {
	tests := []struct {
		path string
		hint SourceHint
		want bool
	}{
		// SourceTV forces TV classification
		{"/downloads/movie.mkv", SourceTV, true},
		{"/downloads/no.pattern.mkv", SourceTV, true},

		// SourceMovie forces Movie classification
		{"/downloads/Show.S01E05.mkv", SourceMovie, false},
		{"/downloads/Daily.Show.2026.01.22.mkv", SourceMovie, false},

		// SourceUnknown uses filename detection
		{"/downloads/Show.S01E05.mkv", SourceUnknown, true},
		{"/downloads/Daily.Show.2026.01.22.mkv", SourceUnknown, true},
		{"/downloads/Movie.2024.mkv", SourceUnknown, false},
	}

	for _, tt := range tests {
		name := fmt.Sprintf("%s_hint_%v", filepath.Base(tt.path), tt.hint)
		t.Run(name, func(t *testing.T) {
			got := IsTVEpisodeFromPath(tt.path, tt.hint)
			if got != tt.want {
				t.Errorf("IsTVEpisodeFromPath(%q, %v) = %v, want %v", tt.path, tt.hint, got, tt.want)
			}
		})
	}
}

func TestStripReleaseMarkers_PreservesShortTitleWords(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"preserves Pitt", "The Pitt", "The Pitt"},
		{"preserves Hope", "Raising Hope", "Raising Hope"},
		{"preserves Deed", "No Good Deed", "No Good Deed"},
		{"preserves Rome", "Rome", "Rome"},
		{"still strips known groups", "The Pitt RARBG", "The Pitt"},
		{"still strips quality markers", "The Pitt 720p", "The Pitt"},
		{"still strips bracketed groups", "The Pitt [FLUX]", "The Pitt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := strings.TrimSpace(stripReleaseMarkers(tt.input))
			// Collapse multiple spaces
			for strings.Contains(got, "  ") {
				got = strings.ReplaceAll(got, "  ", " ")
			}
			if got != tt.expected {
				t.Errorf("stripReleaseMarkers(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// TestParseFailures_WrapErrParseFailed verifies that every parse-failure
// return path wraps ErrParseFailed, so callers can classify the failure as
// deterministic via errors.Is — this gates the log-spam fix in the handler
// and the retry-skip logic in the scanner.
func TestParseFailures_WrapErrParseFailed(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"ParseMovieName empty", mustErr(ParseMovieName("....mkv"))},
		{"ParseTVShowName no episode", mustErr(ParseTVShowName("random.garbage.no.episode.mkv"))},
		{"ParseTVShowFromPath obfuscated no markers", mustErr(ParseTVShowFromPath("/tmp/abcdef1234567890abcdef1234567890.mkv"))},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.err == nil {
				t.Fatal("expected error, got nil")
			}
			if !errors.Is(c.err, ErrParseFailed) {
				t.Errorf("error %q does not wrap ErrParseFailed", c.err)
			}
		})
	}
}

func mustErr[T any](_ *T, err error) error {
	return err
}

func TestParseTVShowNameVerbose(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantTitle  string
		wantSeason int
		wantEp     int
		wantTokens []string // subset that must be present
	}{
		{
			name:       "common release markers",
			input:      "Breaking.Bad.S01E01.1080p.WEB-DL.DDP5.1.H.264-FLUX.mkv",
			wantTitle:  "Breaking Bad",
			wantSeason: 1,
			wantEp:     1,
			wantTokens: []string{"1080p", "WEB-DL", "FLUX"},
		},
		{
			name:       "bluray release",
			input:      "Show.S02E03.2160p.BluRay.x265-GROUP.mkv",
			wantTitle:  "Show",
			wantSeason: 2,
			wantEp:     3,
			wantTokens: []string{"2160p", "BluRay"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, tokens, err := ParseTVShowNameVerbose(tt.input)
			if err != nil {
				t.Fatalf("ParseTVShowNameVerbose() unexpected error: %v", err)
			}
			if info.Title != tt.wantTitle {
				t.Errorf("title = %q, want %q", info.Title, tt.wantTitle)
			}
			if info.Season != tt.wantSeason {
				t.Errorf("season = %d, want %d", info.Season, tt.wantSeason)
			}
			if info.Episode != tt.wantEp {
				t.Errorf("episode = %d, want %d", info.Episode, tt.wantEp)
			}
			for _, want := range tt.wantTokens {
				found := false
				for _, tok := range tokens {
					if strings.EqualFold(tok, want) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected token %q in stripped tokens %v", want, tokens)
				}
			}
		})
	}
}

func TestParseMovieNameVerbose(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantTitle  string
		wantYear   string
		wantTokens []string
	}{
		{
			name:       "common release markers",
			input:      "Dune.Part.Two.2024.1080p.BluRay.x264-GROUP.mkv",
			wantTitle:  "Dune Part Two",
			wantYear:   "2024",
			wantTokens: []string{"1080p", "BluRay"},
		},
		{
			name:       "web-dl release",
			input:      "Inception.2010.2160p.WEB-DL.HDR.DTS-SPARKS.mkv",
			wantTitle:  "Inception",
			wantYear:   "2010",
			wantTokens: []string{"2160p", "WEB-DL", "SPARKS"},
		},
		{
			name:       "service and edition markers",
			input:      "Final.Destination.5.2011.NORDiC.1080p.HMAX.WEB-DL.H.265.DDP5.1-NoTrace.mkv",
			wantTitle:  "Final Destination 5",
			wantYear:   "2011",
			wantTokens: []string{"NORDiC", "1080p", "HMAX", "WEB-DL", "H.265", "DDP5.1", "NoTrace"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, tokens, err := ParseMovieNameVerbose(tt.input)
			if err != nil {
				t.Fatalf("ParseMovieNameVerbose() unexpected error: %v", err)
			}
			if info.Title != tt.wantTitle {
				t.Errorf("title = %q, want %q", info.Title, tt.wantTitle)
			}
			if info.Year != tt.wantYear {
				t.Errorf("year = %q, want %q", info.Year, tt.wantYear)
			}
			for _, want := range tt.wantTokens {
				found := false
				for _, tok := range tokens {
					if strings.EqualFold(tok, want) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected token %q in stripped tokens %v", want, tokens)
				}
			}
		})
	}
}

func TestOriginalFunctionsStillCompile(t *testing.T) {
	_, err := ParseTVShowName("Show.S01E01.mkv")
	if err != nil {
		t.Fatal(err)
	}
	_, err = ParseMovieName("Movie.2024.mkv")
	if err != nil {
		t.Fatal(err)
	}
}

func TestParseTVShowExtractsEpisodeTitle(t *testing.T) {
	cases := []struct {
		file string
		want string
	}{
		{"Lucky.2026.S01E01.No.Shortcuts.1080p.WEB.h264-ETHEL.mkv", "No Shortcuts"},
		{"Lucky.2026.S01E02.1080p.WEB.h264-ETHEL.mkv", ""},
		{"Lucky (2026) S01E01 - No Shortcuts.mkv", "No Shortcuts"}, // round-trip of our own output form
		{"The.New.Adventures.S03E05.Whats.the.Score.Pooh.1080p.DSNP.WEB-DL.AAC2.0.x264-AndreMor.mkv", "Whats the Score Pooh"},
		// A bare year after the episode marker is release metadata, never an
		// episode title (matches the parser's post-marker-year rule).
		{"Upload.S04E03.2025.1080p.Amazon.WEB-DL.AVC.DDP.5.1-DBTV.mkv", ""},
	}

	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			info, err := ParseTVShowName(c.file)
			if err != nil {
				t.Fatalf("ParseTVShowName(%q) unexpected error: %v", c.file, err)
			}
			if info.EpisodeTitle != c.want {
				t.Errorf("ParseTVShowName(%q) EpisodeTitle = %q, want %q", c.file, info.EpisodeTitle, c.want)
			}
		})
	}
}

func TestParseTVShowDateBasedHasNoEpisodeTitle(t *testing.T) {
	file := "The.Daily.Show.2026.04.20.Annalena.Baerbock.1080p.WEB.h264-EDITH.mkv"
	info, err := ParseTVShowName(file)
	if err != nil {
		t.Fatalf("ParseTVShowName(%q) unexpected error: %v", file, err)
	}
	if info.EpisodeTitle != "" {
		t.Errorf("EpisodeTitle = %q, want empty for date-based episode", info.EpisodeTitle)
	}
	got := FormatTVEpisodeFilenameFromInfo(info, "mkv")
	want := "The Daily Show 2026-04-20.mkv"
	if got != want {
		t.Errorf("FormatTVEpisodeFilenameFromInfo() = %q, want %q", got, want)
	}
}

func TestFormatTVEpisodeFilenameFromInfoWithEpisodeTitle(t *testing.T) {
	info := &TVShowInfo{Title: "Lucky", Year: "2026", Season: 1, Episode: 1, EpisodeTitle: "No Shortcuts"}
	got := FormatTVEpisodeFilenameFromInfo(info, "mkv")
	want := "Lucky (2026) S01E01 - No Shortcuts.mkv"
	if got != want {
		t.Errorf("FormatTVEpisodeFilenameFromInfo() = %q, want %q", got, want)
	}

	info.EpisodeTitle = ""
	got = FormatTVEpisodeFilenameFromInfo(info, "mkv")
	want = "Lucky (2026) S01E01.mkv"
	if got != want {
		t.Errorf("FormatTVEpisodeFilenameFromInfo() without title = %q, want %q", got, want)
	}

	info.EpisodeTitle = "No Shortcuts"
	info.Year = ""
	got = FormatTVEpisodeFilenameFromInfo(info, "mkv")
	want = "Lucky S01E01 - No Shortcuts.mkv"
	if got != want {
		t.Errorf("FormatTVEpisodeFilenameFromInfo() without year = %q, want %q", got, want)
	}
}
