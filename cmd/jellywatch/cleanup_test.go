package main

import (
	"testing"
)

func TestIsVideoFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"/path/to/movie.mkv", true},
		{"/path/to/movie.mp4", true},
		{"/path/to/movie.avi", true},
		{"/path/to/movie.MKV", true},
		{"/path/to/movie.nfo", false},
		{"/path/to/movie.jpg", false},
		{"/path/to/movie.srt", false},
		{"/path/to/movie.txt", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := isVideoFile(tt.path)
			if result != tt.expected {
				t.Errorf("isVideoFile(%q) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}
