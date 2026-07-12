package setup

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// NormalizeScanFrequency accepts a Go duration (5m, 1h) or a bare positive
// integer as minutes (10 -> 10m). Returns the canonical duration string.
func NormalizeScanFrequency(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", fmt.Errorf("empty scan frequency")
	}
	if n, err := strconv.Atoi(s); err == nil {
		if n <= 0 {
			return "", fmt.Errorf("scan frequency must be positive")
		}
		s = fmt.Sprintf("%dm", n)
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return "", fmt.Errorf("must be a positive duration such as 5m")
	}
	return s, nil
}
