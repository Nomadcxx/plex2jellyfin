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

// GetNudgePrompt returns a corrective prompt to help the AI fix malformed JSON responses.
// This is a necessary public API function for the recovery mechanism.
func GetNudgePrompt() string {
	return "Your previous response was not valid JSON. Please fix it by following these rules:\n\n" +
		"1. Return ONLY valid JSON, nothing else\n" +
		"2. No markdown code blocks (no ```json or ```)\n" +
		"3. No extra text before or after the JSON\n" +
		"4. All strings must be properly quoted\n" +
		"5. All numbers must be valid integers or floats\n" +
		"6. No trailing commas\n" +
		"7. Ensure all required fields are present:\n" +
		"   - title (string)\n" +
		"   - type (either \"movie\" or \"tv\")\n" +
		"   - confidence (number between 0 and 1)\n\n" +
		"Optional fields (include if applicable):\n" +
		"- year (integer, e.g., 2024)\n" +
		"- season (integer for TV shows)\n" +
		"- episodes (array of integers for TV episodes, e.g., [1] or [1, 2])\n" +
		"- absolute_episode (integer for absolute numbered shows)\n" +
		"- air_date (string in YYYY-MM-DD format)\n\n" +
		"Example valid response:\n" +
		"{\n" +
		"  \"title\": \"Example Movie\",\n" +
		"  \"year\": 2024,\n" +
		"  \"type\": \"movie\",\n" +
		"  \"confidence\": 0.95\n" +
		"}\n\n" +
		"Try again and return ONLY valid JSON."
}
