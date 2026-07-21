package correction

import (
	"context"

	"github.com/Nomadcxx/plex2jellyfin/internal/tmdb"
)

var _ Verifier = (*tmdb.Verifier)(nil)

type Verifier interface {
	LookupExact(ctx context.Context, kind tmdb.MediaKind, title, year string) *tmdb.Match
}

type Decision struct {
	Action   string
	NewTitle string
	NewYear  string
	TmdbID   string
	Reason   string
}

type Corrector struct {
	verifier Verifier
}

func NewCorrector(v Verifier) *Corrector { return &Corrector{verifier: v} }

func (c *Corrector) Decide(ctx context.Context, currentTitle, year string) Decision {
	if c == nil || c.verifier == nil || currentTitle == "" || year == "" {
		return Decision{Action: "leave", Reason: "insufficient input"}
	}
	if m := c.verifier.LookupExact(ctx, tmdb.KindMovie, currentTitle, year); m != nil {
		return Decision{Action: "leave", Reason: "current name resolves; jellyfin behind"}
	}
	for _, cand := range GenerateCandidates(currentTitle) {
		if m := c.verifier.LookupExact(ctx, tmdb.KindMovie, cand, year); m != nil {
			return Decision{
				Action:   "correct",
				NewTitle: m.Title,
				NewYear:  m.Year,
				TmdbID:   m.ID,
				Reason:   "candidate resolves to different title",
			}
		}
	}
	return Decision{Action: "leave", Reason: "no candidate resolved"}
}