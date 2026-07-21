package correction

import (
	"context"
	"testing"

	"github.com/Nomadcxx/plex2jellyfin/internal/tmdb"
)

type fakeVerifier struct {
	exact map[string]*tmdb.Match
}

func (f *fakeVerifier) LookupExact(_ context.Context, _ tmdb.MediaKind, title, year string) *tmdb.Match {
	return f.exact[title+"|"+year]
}

func TestDecideCorrectsWrongName(t *testing.T) {
	f := &fakeVerifier{exact: map[string]*tmdb.Match{
		"Scary Movie|2026": {ID: "111", Title: "Scary Movie", Year: "2026"},
	}}
	c := NewCorrector(f)
	d := c.Decide(context.Background(), "Scary Movie Cut", "2026")
	if d.Action != "correct" || d.NewTitle != "Scary Movie" || d.TmdbID != "111" {
		t.Fatalf("Decide = %+v, want correct Scary Movie 111", d)
	}
}

func TestDecideLeavesWhenCurrentNameMatches(t *testing.T) {
	f := &fakeVerifier{exact: map[string]*tmdb.Match{
		"Final Cut|2022": {ID: "222", Title: "Final Cut", Year: "2022"},
	}}
	c := NewCorrector(f)
	d := c.Decide(context.Background(), "Final Cut", "2022")
	if d.Action != "leave" {
		t.Fatalf("Decide = %+v, want leave (current name matches)", d)
	}
}

func TestDecideLeavesWhenNothingMatches(t *testing.T) {
	c := NewCorrector(&fakeVerifier{exact: map[string]*tmdb.Match{}})
	d := c.Decide(context.Background(), "Totally Obscure New Release", "2026")
	if d.Action != "leave" {
		t.Fatalf("Decide = %+v, want leave (too new / unknown)", d)
	}
}