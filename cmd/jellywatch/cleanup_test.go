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

func TestIsLibraryRoot(t *testing.T) {
	roots := []string{
		"/mnt/STORAGE1/TVSHOWS",
		"/mnt/STORAGE2/MOVIES",
	}

	tests := []struct {
		path     string
		expected bool
	}{
		{"/mnt/STORAGE1/TVSHOWS", true},
		{"/mnt/STORAGE2/MOVIES", true},
		{"/mnt/STORAGE1/TVSHOWS/Show Name", false},
		{"/mnt/STORAGE3/OTHER", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := isLibraryRoot(tt.path, roots)
			if result != tt.expected {
				t.Errorf("isLibraryRoot(%q) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestIsInsideLibrary(t *testing.T) {
	roots := []string{
		"/mnt/STORAGE1/TVSHOWS",
		"/mnt/STORAGE2/MOVIES",
	}

	tests := []struct {
		path     string
		expected bool
	}{
		{"/mnt/STORAGE1/TVSHOWS/Show/Season 01", true},
		{"/mnt/STORAGE2/MOVIES/Movie (2020)", true},
		{"/mnt/STORAGE1/TVSHOWS", true},
		{"/mnt/STORAGE3/OTHER/file.mkv", false},
		{"/home/user/file.mkv", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := isInsideLibrary(tt.path, roots)
			if result != tt.expected {
				t.Errorf("isInsideLibrary(%q) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}
