package ai

import (
	"encoding/json"
	"testing"
)

func TestFlexInt_UnmarshalInt(t *testing.T) {
	data := []byte("1999")
	var f FlexInt
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("Failed to unmarshal int: %v", err)
	}
	if f.Value == nil {
		t.Fatal("Expected non-nil Value")
	}
	if *f.Value != 1999 {
		t.Errorf("Expected 1999, got %d", *f.Value)
	}
}

func TestFlexInt_UnmarshalString(t *testing.T) {
	data := []byte("\"2018\"")
	var f FlexInt
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("Failed to unmarshal string: %v", err)
	}
	if f.Value == nil {
		t.Fatal("Expected non-nil Value")
	}
	if *f.Value != 2018 {
		t.Errorf("Expected 2018, got %d", *f.Value)
	}
}

func TestFlexInt_UnmarshalNull(t *testing.T) {
	data := []byte("null")
	var f FlexInt
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("Failed to unmarshal null: %v", err)
	}
	if f.Value != nil {
		t.Errorf("Expected nil Value, got %d", *f.Value)
	}
}

func TestFlexInt_UnmarshalStringWithSpaces(t *testing.T) {
	data := []byte("\"  2020  \"")
	var f FlexInt
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("Failed to unmarshal string with spaces: %v", err)
	}
	if f.Value == nil {
		t.Fatal("Expected non-nil Value")
	}
	if *f.Value != 2020 {
		t.Errorf("Expected 2020, got %d", *f.Value)
	}
}

func TestFlexInt_UnmarshalInvalidString(t *testing.T) {
	data := []byte("\"not-a-number\"")
	var f FlexInt
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("Failed to unmarshal invalid string: %v", err)
	}
	if f.Value != nil {
		t.Errorf("Expected nil Value for invalid string, got %d", *f.Value)
	}
}

func TestFlexInt_Marshal(t *testing.T) {
	val := 2020
	f := FlexInt{Value: &val}
	data, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}
	if string(data) != "2020" {
		t.Errorf("Expected 2020, got %s", string(data))
	}
}

func TestFlexInt_MarshalNull(t *testing.T) {
	f := FlexInt{Value: nil}
	data, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("Failed to marshal null: %v", err)
	}
	if string(data) != "null" {
		t.Errorf("Expected null, got %s", string(data))
	}
}

func TestFlexInt_Int(t *testing.T) {
	val := 2020
	f := &FlexInt{Value: &val}
	if f.Int() == nil || *f.Int() != 2020 {
		t.Errorf("Int() returned wrong value")
	}

	var nilFlex *FlexInt
	if nilFlex.Int() != nil {
		t.Errorf("Int() on nil FlexInt should return nil")
	}
}

func TestNewFlexInt(t *testing.T) {
	val := 2020
	f := NewFlexInt(&val)
	if f == nil || f.Value == nil || *f.Value != 2020 {
		t.Errorf("NewFlexInt returned wrong value")
	}

	nilFlex := NewFlexInt(nil)
	if nilFlex != nil {
		t.Errorf("NewFlexInt(nil) should return nil")
	}
}

func TestFlexIntSlice_UnmarshalIntArray(t *testing.T) {
	data := []byte("[1, 2, 3]")
	var f FlexIntSlice
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("Failed to unmarshal int array: %v", err)
	}
	if len(f) != 3 || f[0] != 1 || f[1] != 2 || f[2] != 3 {
		t.Errorf("Expected [1, 2, 3], got %v", f)
	}
}

func TestFlexIntSlice_UnmarshalStringArray(t *testing.T) {
	data := []byte(`["S01E06", "S01E07"]`)
	var f FlexIntSlice
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("Failed to unmarshal string array: %v", err)
	}
	if len(f) != 2 || f[0] != 6 || f[1] != 7 {
		t.Errorf("Expected [6, 7], got %v", f)
	}
}

func TestFlexIntSlice_UnmarshalNull(t *testing.T) {
	data := []byte("null")
	var f FlexIntSlice
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("Failed to unmarshal null: %v", err)
	}
	if f != nil {
		t.Errorf("Expected nil, got %v", f)
	}
}

func TestFlexIntSlice_UnmarshalEmptyArray(t *testing.T) {
	data := []byte("[]")
	var f FlexIntSlice
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("Failed to unmarshal empty array: %v", err)
	}
	if len(f) != 0 {
		t.Errorf("Expected empty slice, got %v", f)
	}
}

func TestFlexIntSlice_UnmarshalMixedStringFormats(t *testing.T) {
	data := []byte(`["S01E01", "E02", "3", "S02E10"]`)
	var f FlexIntSlice
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("Failed to unmarshal mixed formats: %v", err)
	}
	if len(f) != 4 || f[0] != 1 || f[1] != 2 || f[2] != 3 || f[3] != 10 {
		t.Errorf("Expected [1, 2, 3, 10], got %v", f)
	}
}

func TestExtractEpisodeNumber(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"S01E06", 6},
		{"s01e06", 6},
		{"S01E10", 10},
		{"S01E01", 1},
		{"S12E345", 345},
		{"E05", 5},
		{"E001", 1},
		{"5", 5},
		{"10", 10},
		{"", 0},
		{"S01", 0},
		{"episode", 0},
		{"S01EXX", 0},
		{"S01E01E02", 1}, // Takes first number after E
	}

	for _, tt := range tests {
		result := extractEpisodeNumber(tt.input)
		if result != tt.expected {
			t.Errorf("extractEpisodeNumber(%q) = %d, want %d", tt.input, result, tt.expected)
		}
	}
}

func TestFlexInt_InResultStruct(t *testing.T) {
	jsonData := `{"title": "Test", "year": "2018", "type": "movie", "confidence": 0.95}`
	var result Result
	if err := json.Unmarshal([]byte(jsonData), &result); err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}
	if result.Year == nil || result.Year.Value == nil {
		t.Fatal("Expected year to be parsed")
	}
	if *result.Year.Value != 2018 {
		t.Errorf("Expected year 2018, got %d", *result.Year.Value)
	}
}

func TestFlexIntSlice_InResultStruct(t *testing.T) {
	jsonData := `{"title": "Test", "type": "tv", "episodes": ["S01E01", "S01E02"], "confidence": 0.95}`
	var result Result
	if err := json.Unmarshal([]byte(jsonData), &result); err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}
	if len(result.Episodes) != 2 || result.Episodes[0] != 1 || result.Episodes[1] != 2 {
		t.Errorf("Expected episodes [1, 2], got %v", result.Episodes)
	}
}
