package ai

import (
	"encoding/json"
	"strconv"
	"strings"
)

// FlexInt handles JSON that may be int, string, or null
type FlexInt struct {
	Value *int
}

// Int returns the int pointer for database operations
func (f *FlexInt) Int() *int {
	if f == nil {
		return nil
	}
	return f.Value
}

// NewFlexInt creates a FlexInt from an *int
func NewFlexInt(v *int) *FlexInt {
	if v == nil {
		return nil
	}
	return &FlexInt{Value: v}
}

func (f *FlexInt) UnmarshalJSON(data []byte) error {
	// Handle null
	if string(data) == "null" {
		f.Value = nil
		return nil
	}
	// Try int first
	var i int
	if err := json.Unmarshal(data, &i); err == nil {
		f.Value = &i
		return nil
	}
	// Try string
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		s = strings.TrimSpace(s)
		if parsed, err := strconv.Atoi(s); err == nil {
			f.Value = &parsed
			return nil
		}
	}
	f.Value = nil
	return nil
}

// MarshalJSON implements json.Marshaler
func (f FlexInt) MarshalJSON() ([]byte, error) {
	if f.Value == nil {
		return []byte("null"), nil
	}
	return json.Marshal(*f.Value)
}

// FlexIntSlice handles episodes that may be int array or string array
type FlexIntSlice []int

func (f *FlexIntSlice) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*f = nil
		return nil
	}
	// Try int array
	var ints []int
	if err := json.Unmarshal(data, &ints); err == nil {
		*f = ints
		return nil
	}
	// Try string array (like ["S01E06"])
	var strs []string
	if err := json.Unmarshal(data, &strs); err == nil {
		result := make([]int, 0, len(strs))
		for _, s := range strs {
			if num := extractEpisodeNumber(s); num > 0 {
				result = append(result, num)
			}
		}
		*f = result
		return nil
	}
	*f = nil
	return nil
}

func extractEpisodeNumber(s string) int {
	s = strings.ToUpper(strings.TrimSpace(s))
	if idx := strings.Index(s, "E"); idx >= 0 {
		numStr := s[idx+1:]
		numStr = strings.TrimLeft(numStr, "0")
		for i, c := range numStr {
			if c < '0' || c > '9' {
				numStr = numStr[:i]
				break
			}
		}
		if numStr == "" {
			numStr = "0"
		}
		if num, err := strconv.Atoi(numStr); err == nil {
			return num
		}
	}
	if num, err := strconv.Atoi(s); err == nil {
		return num
	}
	return 0
}
