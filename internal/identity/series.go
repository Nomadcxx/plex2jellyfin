package identity

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

type Verdict string

const (
	VerdictSame      Verdict = "same"
	VerdictDifferent Verdict = "different"
	VerdictUncertain Verdict = "uncertain"
)

type SeriesIdentity struct {
	Title string
	Year  int
	Path  string
}

type SeriesDecision struct {
	Verdict Verdict
	Reasons []string
}

var yearPattern = regexp.MustCompile(`\b(19|20)\d{2}\b`)

func CompareSeries(a, b SeriesIdentity) SeriesDecision {
	a = fillSeriesIdentity(a)
	b = fillSeriesIdentity(b)

	aTokens := titleTokens(a.Title)
	bTokens := titleTokens(b.Title)
	if len(aTokens) == 0 || len(bTokens) == 0 {
		return SeriesDecision{Verdict: VerdictUncertain, Reasons: []string{"missing series title evidence"}}
	}

	if a.Year > 0 && b.Year > 0 && a.Year != b.Year {
		return SeriesDecision{
			Verdict: VerdictDifferent,
			Reasons: []string{fmt.Sprintf("year mismatch: %d vs %d", a.Year, b.Year)},
		}
	}

	aRegion := regionMarker(aTokens)
	bRegion := regionMarker(bTokens)
	if aRegion != "" && bRegion != "" && aRegion != bRegion {
		return SeriesDecision{
			Verdict: VerdictDifferent,
			Reasons: []string{fmt.Sprintf("region mismatch: %s vs %s", aRegion, bRegion)},
		}
	}

	aBase := tokensWithoutRegions(aTokens)
	bBase := tokensWithoutRegions(bTokens)
	if strings.Join(aBase, " ") == strings.Join(bBase, " ") {
		if aRegion != bRegion {
			// ponytail: deterministic first-pass guardrail; promote uncertain
			// cases to manual review rather than growing an alias database here.
			return SeriesDecision{Verdict: VerdictUncertain, Reasons: []string{"region marker differs or is missing"}}
		}
		return SeriesDecision{Verdict: VerdictSame}
	}

	if aRegion != bRegion && oneBaseContainsTheOther(aBase, bBase) {
		return SeriesDecision{Verdict: VerdictDifferent, Reasons: []string{"regional title family mismatch"}}
	}

	return SeriesDecision{Verdict: VerdictUncertain, Reasons: []string{"series title evidence does not match exactly"}}
}

func fillSeriesIdentity(id SeriesIdentity) SeriesIdentity {
	base := strings.TrimSuffix(filepath.Base(filepath.Clean(id.Path)), filepath.Ext(id.Path))
	if strings.TrimSpace(id.Title) == "" {
		id.Title = stripYear(base)
		if id.Year == 0 {
			id.Year = extractYear(base)
		}
	}
	return id
}

func extractYear(s string) int {
	match := yearPattern.FindString(s)
	if match == "" {
		return 0
	}
	year, _ := strconv.Atoi(match)
	return year
}

func stripYear(s string) string {
	return strings.TrimSpace(yearPattern.ReplaceAllString(s, ""))
}

func titleTokens(title string) []string {
	title = stripYear(title)
	title = strings.ReplaceAll(title, "'", "")
	var b strings.Builder
	for _, r := range strings.ToLower(title) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			continue
		}
		b.WriteByte(' ')
	}
	return strings.Fields(b.String())
}

func regionMarker(tokens []string) string {
	for _, token := range tokens {
		switch token {
		case "au", "australian":
			return "au"
		case "uk", "gb", "british":
			return "uk"
		case "us", "usa", "american":
			return "us"
		}
	}
	return ""
}

func tokensWithoutRegions(tokens []string) []string {
	out := make([]string, 0, len(tokens))
	for _, token := range tokens {
		switch token {
		case "au", "australian", "uk", "gb", "british", "us", "usa", "american":
			continue
		default:
			out = append(out, token)
		}
	}
	return out
}

func oneBaseContainsTheOther(a, b []string) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	aText := strings.Join(a, " ")
	bText := strings.Join(b, " ")
	return strings.Contains(aText, bText) || strings.Contains(bText, aText)
}
