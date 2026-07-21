// Package tmdb provides movie/TV title verification used by the
// housekeeping engine to distinguish legitimate remakes from genuine
// metadata duplicates before merging cross-volume folders.
//
// The verifier consults sources in order:
//
//  1. Local DB (movies.tmdb_id) — instant, no network, populated by
//     Radarr sync or Jellyfin sweep.
//  2. Jellyfin RemoteSearch — uses already-configured Jellyfin
//     credentials, hits Jellyfin's TMDB metadata provider. No extra
//     credentials needed.
//  3. TMDB direct (themoviedb.org/3) — requires user-supplied API key
//     in [tmdb] config. Fallback for users without Jellyfin.
//
// All non-local results are cached in tmdb_lookups (per migration 19)
// so verification is cheap to re-run.
package tmdb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Nomadcxx/plex2jellyfin/internal/database"
	"github.com/Nomadcxx/plex2jellyfin/internal/jellyfin"
)

// MediaKind narrows search to movies or series.
type MediaKind string

const (
	KindMovie  MediaKind = "movie"
	KindSeries MediaKind = "series"
)

// Match is a single hit from any provider. ID is the TMDB integer ID
// stringified for portability; Year is "YYYY" (may be empty if the
// provider has no release date).
type Match struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Year  string `json:"year"`
}

// Verifier resolves a folder's (title, year) tuple to a TMDB match by
// consulting sources in order until one returns a usable answer.
type Verifier struct {
	db          *database.MediaDB
	jelly       *jellyfin.Client
	tmdbAPIKey  string
	httpClient  *http.Client
	cacheMaxAge time.Duration
}

// NewVerifier wires up a verifier. Any of jelly/tmdbAPIKey may be empty;
// the verifier silently skips disabled tiers.
func NewVerifier(db *database.MediaDB, jelly *jellyfin.Client, tmdbAPIKey string) *Verifier {
	return &Verifier{
		db:          db,
		jelly:       jelly,
		tmdbAPIKey:  strings.TrimSpace(tmdbAPIKey),
		httpClient:  &http.Client{Timeout: 10 * time.Second},
		cacheMaxAge: 30 * 24 * time.Hour,
	}
}

// Available reports whether the verifier has at least one network tier
// configured. Local-DB-only verification still works without this.
func (v *Verifier) Available() bool {
	return v.jelly != nil || v.tmdbAPIKey != ""
}

// VerifyResult describes the outcome of comparing two folders that the
// detector classified as a year_mismatch group.
type VerifyResult struct {
	// Verdict is one of "distinct" (legitimate remakes — different TMDB
	// IDs), "duplicate" (same TMDB ID — should be merged), or "unknown"
	// (verification failed; flag for human review).
	Verdict  string `json:"verdict"`
	SrcMatch *Match `json:"src_match,omitempty"`
	DstMatch *Match `json:"dst_match,omitempty"`
	Source   string `json:"source"`
	Reason   string `json:"reason,omitempty"`
}

// Verify compares two folder candidates.
func (v *Verifier) Verify(ctx context.Context, kind MediaKind, srcTitle, srcYear, dstTitle, dstYear string) *VerifyResult {
	src := v.lookup(ctx, kind, srcTitle, srcYear)
	dst := v.lookup(ctx, kind, dstTitle, dstYear)

	r := &VerifyResult{SrcMatch: src, DstMatch: dst}
	switch {
	case src == nil || dst == nil:
		r.Verdict = "unknown"
		r.Reason = "one or both folders unresolved"
	case src.ID == dst.ID:
		r.Verdict = "duplicate"
		r.Source = "tmdb"
		r.Reason = fmt.Sprintf("both resolve to TMDB id=%s", src.ID)
	default:
		r.Verdict = "distinct"
		r.Source = "tmdb"
		r.Reason = fmt.Sprintf("src=%s dst=%s — different works", src.ID, dst.ID)
	}
	return r
}

func (v *Verifier) lookup(ctx context.Context, kind MediaKind, title, year string) *Match {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil
	}
	if m := v.lookupLocal(kind, title, year); m != nil {
		return m
	}
	matches := v.lookupCached(kind, title)
	if matches == nil {
		matches = v.fetchAll(ctx, kind, title)
	}
	// Narrow to exact-title hits when present so close-but-distinct works
	// like "Hostel Dissected" don't pollute year-tolerance picking.
	if exact := filterExactTitle(matches, title); len(exact) > 0 {
		matches = exact
	}
	if m := pickByYear(matches, year); m != nil {
		return m
	}
	// Fallback: if there is exactly one canonical exact-title candidate
	// across all years, treat the folder as resolving to it (the user's
	// year is just wrong). When there are several real candidates the
	// year disambiguator was needed and we stay "unresolved".
	if len(matches) == 1 {
		return &matches[0]
	}
	return nil
}

// LookupExact resolves a title to a TMDB match ONLY when there is an
// exact-title candidate whose year integer-equals the requested year. Unlike
// lookup it applies no ±1-year tolerance and no single-candidate fallback, so
// it is safe to drive auto-apply renames (a mis-parse like "Avatar Fire and
// Ash" trimmed to "Avatar" will not resolve to Avatar (2009) at the wrong
// year). Returns nil when title or year is empty.
func (v *Verifier) LookupExact(ctx context.Context, kind MediaKind, title, year string) *Match {
	title = strings.TrimSpace(title)
	year = strings.TrimSpace(year)
	if title == "" || year == "" {
		return nil
	}
	if m := v.lookupLocal(kind, title, year); m != nil && m.Year == year {
		return m
	}
	matches := v.lookupCached(kind, title)
	if matches == nil {
		matches = v.fetchAll(ctx, kind, title)
	}
	exact := filterExactTitle(matches, title)
	for i := range exact {
		if exact[i].Year == year {
			return &exact[i]
		}
	}
	return nil
}

func filterExactTitle(matches []Match, title string) []Match {
	want := strings.ToLower(strings.TrimSpace(title))
	out := make([]Match, 0, len(matches))
	for _, m := range matches {
		if strings.ToLower(strings.TrimSpace(m.Title)) == want {
			out = append(out, m)
		}
	}
	return out
}

func (v *Verifier) lookupLocal(kind MediaKind, title, year string) *Match {
	if v.db == nil || kind != KindMovie {
		return nil
	}
	rows, err := v.db.DB().Query(`
		SELECT title, year, tmdb_id
		  FROM movies
		 WHERE LOWER(title) = LOWER(?)
		   AND tmdb_id IS NOT NULL`, title)
	if err != nil {
		return nil
	}
	defer rows.Close()
	for rows.Next() {
		var t string
		var y *int
		var id *int
		if err := rows.Scan(&t, &y, &id); err != nil {
			continue
		}
		if id == nil {
			continue
		}
		yearStr := ""
		if y != nil {
			yearStr = fmt.Sprintf("%d", *y)
		}
		if year != "" && yearStr == year {
			return &Match{ID: fmt.Sprintf("%d", *id), Title: t, Year: yearStr}
		}
	}
	return nil
}

func (v *Verifier) lookupCached(kind MediaKind, title string) []Match {
	if v.db == nil {
		return nil
	}
	var resultsJSON string
	var fetchedAt time.Time
	err := v.db.DB().QueryRow(`
		SELECT results_json, fetched_at
		  FROM tmdb_lookups
		 WHERE normalized_title = ? AND media_kind = ?`,
		strings.ToLower(title), string(kind)).Scan(&resultsJSON, &fetchedAt)
	if err != nil {
		return nil
	}
	if v.cacheMaxAge > 0 && time.Since(fetchedAt) > v.cacheMaxAge {
		return nil
	}
	var matches []Match
	if err := json.Unmarshal([]byte(resultsJSON), &matches); err != nil {
		return nil
	}
	return matches
}

func (v *Verifier) cacheStore(kind MediaKind, title string, matches []Match) {
	if v.db == nil {
		return
	}
	b, err := json.Marshal(matches)
	if err != nil {
		return
	}
	_, _ = v.db.DB().Exec(`
		INSERT INTO tmdb_lookups (normalized_title, media_kind, source, results_json, fetched_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(normalized_title, media_kind) DO UPDATE SET
		    source = excluded.source,
		    results_json = excluded.results_json,
		    fetched_at = excluded.fetched_at`,
		strings.ToLower(title), string(kind), "merged", string(b))
}

func (v *Verifier) fetchAll(ctx context.Context, kind MediaKind, title string) []Match {
	// TMDB first — its release/air dates are the disambiguator we rely on
	// in pickByYear. Jellyfin RemoteSearch is then used to add any IDs
	// TMDB missed (or to enrich entries that lack a year).
	byID := map[string]Match{}
	order := []string{}
	add := func(m Match) {
		if m.ID == "" {
			return
		}
		if existing, ok := byID[m.ID]; ok {
			// prefer the entry that has a year populated
			if existing.Year == "" && m.Year != "" {
				byID[m.ID] = m
			}
			return
		}
		byID[m.ID] = m
		order = append(order, m.ID)
	}
	for _, m := range v.fetchTMDB(ctx, kind, title) {
		add(m)
	}
	for _, m := range v.fetchJellyfin(ctx, kind, title) {
		add(m)
	}
	out := make([]Match, 0, len(order))
	for _, id := range order {
		out = append(out, byID[id])
	}
	if len(out) > 0 {
		v.cacheStore(kind, title, out)
	}
	return out
}

func (v *Verifier) fetchJellyfin(ctx context.Context, kind MediaKind, title string) []Match {
	if v.jelly == nil {
		return nil
	}
	matches, err := v.jelly.RemoteSearch(ctx, string(kind), title)
	if err != nil {
		return nil
	}
	out := make([]Match, 0, len(matches))
	for _, m := range matches {
		if m.TmdbID == "" {
			continue
		}
		year := ""
		if m.ProductionYear > 0 {
			year = fmt.Sprintf("%d", m.ProductionYear)
		}
		out = append(out, Match{ID: m.TmdbID, Title: m.Name, Year: year})
	}
	return out
}

func (v *Verifier) fetchTMDB(ctx context.Context, kind MediaKind, title string) []Match {
	if v.tmdbAPIKey == "" {
		return nil
	}
	endpoint := "search/movie"
	dateField := "release_date"
	titleField := "title"
	if kind == KindSeries {
		endpoint = "search/tv"
		dateField = "first_air_date"
		titleField = "name"
	}
	u, _ := url.Parse("https://api.themoviedb.org/3/" + endpoint)
	q := u.Query()
	q.Set("api_key", v.tmdbAPIKey)
	q.Set("query", title)
	q.Set("include_adult", "false")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil
	}
	resp, err := v.httpClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil
	}
	var body struct {
		Results []map[string]any `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil
	}
	out := make([]Match, 0, len(body.Results))
	for _, r := range body.Results {
		idF, _ := r["id"].(float64)
		t, _ := r[titleField].(string)
		date, _ := r[dateField].(string)
		year := ""
		if len(date) >= 4 {
			year = date[:4]
		}
		if idF == 0 || t == "" {
			continue
		}
		out = append(out, Match{ID: fmt.Sprintf("%d", int(idF)), Title: t, Year: year})
	}
	return out
}

func pickByYear(matches []Match, year string) *Match {
	if len(matches) == 0 {
		return nil
	}
	if year == "" {
		if len(matches) == 1 {
			return &matches[0]
		}
		return nil
	}
	// exact-year match wins
	for i := range matches {
		if matches[i].Year == year {
			return &matches[i]
		}
	}
	// Tolerance: TMDB's release_date / first_air_date and the user's
	// folder year occasionally disagree by 1 (e.g. festival run vs wide
	// release, or split-year shows). Allow ±1 only when a single match
	// is within tolerance — refuse to guess if multiple compete.
	yi := atoiSafe(year)
	if yi == 0 {
		return nil
	}
	var nearest *Match
	for i := range matches {
		mi := atoiSafe(matches[i].Year)
		if mi == 0 {
			continue
		}
		if mi == yi-1 || mi == yi+1 {
			if nearest != nil {
				return nil
			}
			nearest = &matches[i]
		}
	}
	return nearest
}

func atoiSafe(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}
