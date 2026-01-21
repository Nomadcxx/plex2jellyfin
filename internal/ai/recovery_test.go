package ai

import "testing"

func TestExtractPartialResult_ValidJSON(t *testing.T) {
	response := `{"title": "The Matrix", "year": 1999, "type": "movie", "confidence": 0.95}`

	result, ok := ExtractPartialResult(response)
	if !ok {
		t.Fatal("expected extraction to succeed")
	}
	if result.Title != "The Matrix" {
		t.Errorf("expected title 'The Matrix', got '%s'", result.Title)
	}
	if result.Year == nil || *result.Year != 1999 {
		t.Errorf("expected year 1999, got %v", result.Year)
	}
}

func TestExtractPartialResult_BrokenJSON(t *testing.T) {
	response := `{"title": "The Matrix", "year": 1999, "type": "movie"`

	result, ok := ExtractPartialResult(response)
	if !ok {
		t.Fatal("expected partial extraction to succeed")
	}
	if result.Title != "The Matrix" {
		t.Errorf("expected title 'The Matrix', got '%s'", result.Title)
	}
}

func TestExtractPartialResult_GarbageResponse(t *testing.T) {
	response := `I'm sorry, I can't help with that request.`

	_, ok := ExtractPartialResult(response)
	if ok {
		t.Error("expected extraction to fail on garbage response")
	}
}

func TestGetNudgePrompt_NotEmpty(t *testing.T) {
	prompt := GetNudgePrompt()
	if prompt == "" {
		t.Fatal("expected nudge prompt to be non-empty")
	}

	if len(prompt) < 100 {
		t.Errorf("expected nudge prompt to be substantial, got %d characters", len(prompt))
	}
}

func TestGetNudgePrompt_ContainsKeyInstructions(t *testing.T) {
	prompt := GetNudgePrompt()

	expectedPhrases := []string{
		"valid JSON",
		"title",
		"type",
		"confidence",
		"movie",
		"tv",
	}

	for _, phrase := range expectedPhrases {
		if !contains(prompt, phrase) {
			t.Errorf("expected nudge prompt to contain '%s'", phrase)
		}
	}
}

func TestIsJSONError_WithJSONErrors(t *testing.T) {
	testCases := []struct {
		name   string
		errMsg string
		expect bool
	}{
		{"invalid character", "invalid character 'a' looking for beginning", true},
		{"unexpected end", "unexpected end of JSON input", true},
		{"failed to parse", "failed to parse AI response", true},
		{"unmarshal", "invalid character after top-level value", true},
		{"JSON keyword", "not valid JSON", true},
		{"unrelated error", "connection refused", false},
		{"empty error", "", false},
		{"nil error", "", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var err error
			if tc.errMsg != "" {
				err = &testError{msg: tc.errMsg}
			}

			result := isJSONError(err)
			if result != tc.expect {
				t.Errorf("expected %v for error '%s', got %v", tc.expect, tc.errMsg, result)
			}
		})
	}
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
