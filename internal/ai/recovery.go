package ai

import (
	"encoding/json"
	"regexp"
	"strconv"
)

var (
	titlePattern = regexp.MustCompile(`"title"\s*:\s*"([^"]+)"`)
	yearPattern  = regexp.MustCompile(`"year"\s*:\s*(\d{4})`)
	typePattern  = regexp.MustCompile(`"type"\s*:\s*"(movie|tv)"`)
	confPattern  = regexp.MustCompile(`"confidence"\s*:\s*([\d.]+)`)
)

func ExtractPartialResult(response string) (*Result, bool) {
	var result Result
	if err := json.Unmarshal([]byte(response), &result); err == nil {
		return &result, true
	}

	titleMatch := titlePattern.FindStringSubmatch(response)
	if titleMatch == nil {
		return nil, false
	}

	result = Result{
		Title:      titleMatch[1],
		Confidence: 0.7,
	}

	if yearMatch := yearPattern.FindStringSubmatch(response); yearMatch != nil {
		if year, err := strconv.Atoi(yearMatch[1]); err == nil {
			result.Year = &year
		}
	}

	if typeMatch := typePattern.FindStringSubmatch(response); typeMatch != nil {
		result.Type = typeMatch[1]
	}

	if confMatch := confPattern.FindStringSubmatch(response); confMatch != nil {
		if conf, err := strconv.ParseFloat(confMatch[1], 64); err == nil {
			result.Confidence = min(conf, 0.8)
		}
	}

	return &result, true
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
