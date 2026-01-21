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
