package daemon

import (
	"regexp"
	"strings"
)

type ChangeCategory string

const (
	ChangeNone           ChangeCategory = "none"
	ChangePunctuation    ChangeCategory = "punctuation"
	ChangeCasing         ChangeCategory = "casing"
	ChangeYearAdded      ChangeCategory = "year_added"
	ChangeYearCorrected  ChangeCategory = "year_corrected"
	ChangeYearRemoved    ChangeCategory = "year_removed"
	ChangeTitleDifferent ChangeCategory = "different_title"
	ChangeTypeDifferent  ChangeCategory = "type_change"
	// ChangeStructural is reserved for future use (e.g. article reordering).
	ChangeStructural ChangeCategory = "structural"
)

type ChangeClassification struct {
	Category      ChangeCategory
	Safe          bool
	MinConfidence float64
}

var punctuationRegex = regexp.MustCompile(`[^a-zA-Z0-9\s]`)

func ClassifyChange(regexTitle, aiTitle, regexYear, aiYear, regexMediaType, aiMediaType string) ChangeClassification {
	// Type change — always risky; no confidence level makes a type change safe
	if regexMediaType != aiMediaType {
		return ChangeClassification{Category: ChangeTypeDifferent, Safe: false, MinConfidence: 1.0}
	}

	// Year changes
	yearChanged := regexYear != aiYear
	yearAdded := regexYear == "" && aiYear != ""
	yearCorrected := regexYear != "" && aiYear != "" && regexYear != aiYear

	// Title comparison
	titlesIdentical := regexTitle == aiTitle
	titlesIdenticalIgnoreCase := strings.EqualFold(regexTitle, aiTitle)

	// Strip punctuation for similarity check
	regexClean := stripPunctuation(regexTitle)
	aiClean := stripPunctuation(aiTitle)
	titlesIdenticalNoPunct := strings.EqualFold(regexClean, aiClean)

	// No change at all
	if titlesIdentical && !yearChanged {
		return ChangeClassification{Category: ChangeNone, Safe: true, MinConfidence: 0}
	}

	// Year only changes (title identical or differs only in punctuation/casing)
	if titlesIdentical || titlesIdenticalIgnoreCase || titlesIdenticalNoPunct {
		if yearAdded {
			return ChangeClassification{Category: ChangeYearAdded, Safe: true, MinConfidence: 0.85}
		}
		if yearCorrected {
			return ChangeClassification{Category: ChangeYearCorrected, Safe: false, MinConfidence: 0.90}
		}
		if regexYear != "" && aiYear == "" {
			return ChangeClassification{Category: ChangeYearRemoved, Safe: false, MinConfidence: 0}
		}
	}

	// Casing change only
	if titlesIdenticalIgnoreCase && !yearChanged {
		return ChangeClassification{Category: ChangeCasing, Safe: true, MinConfidence: 0.80}
	}

	// Punctuation change (titles match after stripping punctuation)
	if titlesIdenticalNoPunct && !yearChanged {
		return ChangeClassification{Category: ChangePunctuation, Safe: true, MinConfidence: 0.80}
	}
	if titlesIdenticalNoPunct && yearAdded {
		return ChangeClassification{Category: ChangeYearAdded, Safe: true, MinConfidence: 0.85}
	}

	// Check Jaccard similarity for "different title" threshold
	similarity := jaccardWordSimilarity(regexTitle, aiTitle)
	if similarity < 0.70 {
		return ChangeClassification{Category: ChangeTitleDifferent, Safe: false}
	}

	// Similar but not identical — treat as punctuation/minor correction
	if yearAdded {
		return ChangeClassification{Category: ChangeYearAdded, Safe: true, MinConfidence: 0.85}
	}
	return ChangeClassification{Category: ChangePunctuation, Safe: true, MinConfidence: 0.80}
}

func stripPunctuation(s string) string {
	return punctuationRegex.ReplaceAllString(s, "")
}

func jaccardWordSimilarity(a, b string) float64 {
	wordsA := toWordSet(a)
	wordsB := toWordSet(b)

	if len(wordsA) == 0 && len(wordsB) == 0 {
		return 1.0
	}

	intersection := 0
	for w := range wordsA {
		if wordsB[w] {
			intersection++
		}
	}

	union := len(wordsA)
	for w := range wordsB {
		if !wordsA[w] {
			union++
		}
	}

	if union == 0 {
		return 1.0
	}
	return float64(intersection) / float64(union)
}

func toWordSet(s string) map[string]bool {
	s = strings.ToLower(s)
	s = punctuationRegex.ReplaceAllString(s, "")
	words := strings.Fields(s)
	set := make(map[string]bool, len(words))
	for _, w := range words {
		if w != "" {
			set[w] = true
		}
	}
	return set
}
