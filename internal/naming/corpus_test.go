package naming

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// corpusEntry matches one line in parse_decisions_corpus.jsonl.
type corpusEntry struct {
	SourcePath     string `json:"source_path"`
	SourceFilename string `json:"source_filename"`
	MediaType      string `json:"media_type"`
	ParsedTitle    string `json:"parsed_title"`
	ParsedYear     int    `json:"parsed_year"`
	ParsedSeason   int    `json:"parsed_season"`
	ParsedEpisode  int    `json:"parsed_episode"`
}

func TestParserCorpusRegression(t *testing.T) {
	f, err := os.Open(filepath.Join("testdata", "parse_decisions_corpus.jsonl"))
	if err != nil {
		t.Fatalf("open corpus: %v", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		lineNum++

		var entry corpusEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Errorf("line %d: unmarshal: %v", lineNum, err)
			continue
		}

		t.Run(entry.SourceFilename, func(t *testing.T) {
			switch entry.MediaType {
			case "tv":
				info, err := ParseTVShowFromPath(entry.SourcePath)
				if err != nil {
					t.Fatalf("ParseTVShowFromPath(%q): %v", entry.SourcePath, err)
				}
				if info.Title != entry.ParsedTitle {
					t.Errorf("title = %q, want %q", info.Title, entry.ParsedTitle)
				}
				if entry.ParsedYear != 0 {
					wantYear := strconv.Itoa(entry.ParsedYear)
					if info.Year != wantYear {
						t.Errorf("year = %q, want %q", info.Year, wantYear)
					}
				} else {
					// l6: a corpus entry with parsed_year == 0 means "no year
					// in the source path".  The parser represents the missing
					// year as an empty string, so assert that explicitly to
					// distinguish from a literal "year zero".
					if info.Year != "" {
						t.Errorf("year = %q, want empty string for missing year", info.Year)
					}
				}
				if info.Season != entry.ParsedSeason {
					t.Errorf("season = %d, want %d", info.Season, entry.ParsedSeason)
				}
				if info.Episode != entry.ParsedEpisode {
					t.Errorf("episode = %d, want %d", info.Episode, entry.ParsedEpisode)
				}
			case "movie":
				info, err := ParseMovieFromPath(entry.SourcePath)
				if err != nil {
					t.Fatalf("ParseMovieFromPath(%q): %v", entry.SourcePath, err)
				}
				if info.Title != entry.ParsedTitle {
					t.Errorf("title = %q, want %q", info.Title, entry.ParsedTitle)
				}
				if entry.ParsedYear != 0 {
					wantYear := strconv.Itoa(entry.ParsedYear)
					if info.Year != wantYear {
						t.Errorf("year = %q, want %q", info.Year, wantYear)
					}
				} else {
					// l6: see comment above — missing year ⇒ empty string.
					if info.Year != "" {
						t.Errorf("year = %q, want empty string for missing year", info.Year)
					}
				}
			default:
				t.Errorf("unknown media_type %q in corpus entry", entry.MediaType)
			}
		})
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scanner: %v", err)
	}
	if lineNum == 0 {
		t.Fatal("corpus file was empty")
	}
}
