package naming

import (
	"testing"
)

func TestIsObfuscatedFilename(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     bool
	}{
		{
			name:     "hex MD5 hash",
			filename: "30e2dc4173fc4798bbe5fd40137ed621.mkv",
			want:     true,
		},
		{
			name:     "hex SHA1 hash",
			filename: "da39a3ee5e6b4b0d3255bfef95601890afd80709.mkv",
			want:     true,
		},
		{
			name:     "UUID format",
			filename: "675d7595-3e9b-4602-9464-6424b664c6d7.mkv",
			want:     true,
		},
		{
			name:     "UUID without dashes",
			filename: "675d75953e9b460294646424b664c6d7.mkv",
			want:     true,
		},
		{
			name:     "random alphanumeric long",
			filename: "RTVA3rFvM11jjtr6pdNPpUDg2.mkv",
			want:     true,
		},
		{
			name:     "base64-like string",
			filename: "vHQWSwxqQTXWDmRKTpZJraJ94mukwa1VnFu1.mkv",
			want:     true,
		},
		{
			name:     "legitimate TV episode",
			filename: "The.White.Lotus.S02E07.mkv",
			want:     false,
		},
		{
			name:     "legitimate TV episode with quality",
			filename: "The.White.Lotus.S02E07.Arrivederci.1080p.HMAX.WEB-DL.DDP5.1.x264-NTb.mkv",
			want:     false,
		},
		{
			name:     "legitimate movie with year",
			filename: "Inception.2010.1080p.BluRay.mkv",
			want:     false,
		},
		{
			name:     "short name not obfuscated",
			filename: "movie.mkv",
			want:     false,
		},
		{
			name:     "short generic name",
			filename: "video.mkv",
			want:     false,
		},
		{
			name:     "episode marker x format",
			filename: "Show.1x01.mkv",
			want:     false,
		},
		{
			name:     "movie with dots and year",
			filename: "The.Matrix.1999.mkv",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsObfuscatedFilename(tt.filename)
			if got != tt.want {
				t.Errorf("IsObfuscatedFilename(%q) = %v, want %v", tt.filename, got, tt.want)
			}
		})
	}
}

func TestParseTVShowFromPath(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		wantTitle   string
		wantSeason  int
		wantEpisode int
		wantErr     bool
	}{
		{
			name:        "obfuscated file with valid folder",
			path:        "/downloads/The.White.Lotus.S02E07.Arrivederci.1080p.HMAX.WEB-DL.DDP5.1.x264-NTb/30e2dc4173fc4798bbe5fd40137ed621.mkv",
			wantTitle:   "The White Lotus",
			wantSeason:  2,
			wantEpisode: 7,
			wantErr:     false,
		},
		{
			name:        "non-obfuscated file",
			path:        "/downloads/The.White.Lotus.S02E07.mkv",
			wantTitle:   "The White Lotus",
			wantSeason:  2,
			wantEpisode: 7,
			wantErr:     false,
		},
		{
			name:        "obfuscated in nested folder",
			path:        "/downloads/tv/Breaking.Bad.S01E01.720p.BluRay/random123abc.mkv",
			wantTitle:   "Breaking Bad",
			wantSeason:  1,
			wantEpisode: 1,
			wantErr:     false,
		},
		{
			name:    "obfuscated with no valid parent",
			path:    "/downloads/random/30e2dc4173fc4798bbe5fd40137ed621.mkv",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := ParseTVShowFromPath(tt.path)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseTVShowFromPath(%q) expected error, got nil", tt.path)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseTVShowFromPath(%q) unexpected error: %v", tt.path, err)
				return
			}
			if info.Title != tt.wantTitle {
				t.Errorf("ParseTVShowFromPath(%q) Title = %q, want %q", tt.path, info.Title, tt.wantTitle)
			}
			if info.Season != tt.wantSeason {
				t.Errorf("ParseTVShowFromPath(%q) Season = %d, want %d", tt.path, info.Season, tt.wantSeason)
			}
			if info.Episode != tt.wantEpisode {
				t.Errorf("ParseTVShowFromPath(%q) Episode = %d, want %d", tt.path, info.Episode, tt.wantEpisode)
			}
		})
	}
}

func TestParseMovieFromPath(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		wantTitle string
		wantYear  string
		wantErr   bool
	}{
		{
			name:      "obfuscated file with valid folder",
			path:      "/downloads/Inception.2010.1080p.BluRay.x264-GROUP/30e2dc4173fc4798bbe5fd40137ed621.mkv",
			wantTitle: "Inception",
			wantYear:  "2010",
			wantErr:   false,
		},
		{
			name:      "non-obfuscated file",
			path:      "/downloads/Inception.2010.mkv",
			wantTitle: "Inception",
			wantYear:  "2010",
			wantErr:   false,
		},
		{
			name:      "obfuscated in nested folder with year in folder name",
			path:      "/downloads/movies/The.Matrix.1999.BluRay/30e2dc4173fc4798bbe5fd40137ed621.mkv",
			wantTitle: "The Matrix",
			wantYear:  "1999",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := ParseMovieFromPath(tt.path)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseMovieFromPath(%q) expected error, got nil", tt.path)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseMovieFromPath(%q) unexpected error: %v", tt.path, err)
				return
			}
			if info.Year != tt.wantYear {
				t.Errorf("ParseMovieFromPath(%q) Year = %q, want %q", tt.path, info.Year, tt.wantYear)
			}
		})
	}
}

func TestIsTVEpisodeFromPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{
			name: "obfuscated TV in folder with hex name",
			path: "/downloads/The.White.Lotus.S02E07.1080p/30e2dc4173fc4798bbe5fd40137ed621.mkv",
			want: true,
		},
		{
			name: "non-obfuscated TV file",
			path: "/downloads/Show.S01E01.mkv",
			want: true,
		},
		{
			name: "obfuscated movie in folder",
			path: "/downloads/Inception.2010.1080p/30e2dc4173fc4798bbe5fd40137ed621.mkv",
			want: false,
		},
		{
			name: "non-obfuscated movie",
			path: "/downloads/Movie.2020.mkv",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsTVEpisodeFromPath(tt.path, SourceUnknown)
			if got != tt.want {
				t.Errorf("IsTVEpisodeFromPath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestHasHighEntropy(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want bool
	}{
		{
			name: "random hex string",
			s:    "30e2dc4173fc4798bbe5fd40137ed621",
			want: true,
		},
		{
			name: "repeated pattern low entropy",
			s:    "aaaabbbbccccdddd",
			want: false,
		},
		{
			name: "short string",
			s:    "abc",
			want: false,
		},
		{
			name: "mixed alphanumeric high entropy",
			s:    "RTVA3rFvM11jjtr6pdNPpUDg2abc",
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasHighEntropy(tt.s)
			if got != tt.want {
				t.Errorf("hasHighEntropy(%q) = %v, want %v", tt.s, got, tt.want)
			}
		})
	}
}
