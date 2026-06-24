package video

import "testing"

func TestIsVideoExt(t *testing.T) {
	videoExts := []string{".mkv", ".mp4", ".avi", ".mov", ".wmv", ".flv", ".webm", ".m4v", ".mpg", ".mpeg", ".m2ts", ".ts"}
	for _, ext := range videoExts {
		if !IsVideoExt(ext) {
			t.Errorf("IsVideoExt(%q) = false, want true", ext)
		}
	}

	nonVideo := []string{".srt", ".sub", ".idx", ".ass", ".ssa", ".vtt", ".smi", ".nfo", ".jpg", ".png", ".txt", ".json", ".xml", ".mp3", ".flac", ".zip", ".rar", ""}
	for _, ext := range nonVideo {
		if IsVideoExt(ext) {
			t.Errorf("IsVideoExt(%q) = true, want false", ext)
		}
	}
}

func TestIsVideoExt_CaseInsensitive(t *testing.T) {
	cases := []string{".MKV", ".MP4", ".Avi", ".MOV", ".Mkv", ".mKV"}
	for _, ext := range cases {
		if !IsVideoExt(ext) {
			t.Errorf("IsVideoExt(%q) = false, want true (case insensitive)", ext)
		}
	}
}

func TestIsVideo(t *testing.T) {
	paths := []string{
		"/mnt/STORAGE1/MOVIES/The Matrix (1999)/The Matrix (1999).mkv",
		"/data/TV/Show S01E01.mp4",
		"movie.avi",
		"/relative/path/video.m2ts",
	}
	for _, p := range paths {
		if !IsVideo(p) {
			t.Errorf("IsVideo(%q) = false, want true", p)
		}
	}

	nonVideo := []string{
		"/mnt/STORAGE1/MOVIES/The Matrix (1999)/poster.jpg",
		"/data/TV/Show S01E01.srt",
		"metadata.nfo",
		"/path/to/readme.txt",
		"/path/to/noext",
		"",
	}
	for _, p := range nonVideo {
		if IsVideo(p) {
			t.Errorf("IsVideo(%q) = true, want false", p)
		}
	}
}

func TestIsVideo_DottedFilename(t *testing.T) {
	if !IsVideo("2001.A.Space.Odyssey.1968.2160p.UHD.BluRay.mkv") {
		t.Error("dotted filename with .mkv should be video")
	}
	if IsVideo("2001.A.Space.Odyssey.1968.2160p.UHD.BluRay.nfo") {
		t.Error("dotted filename with .nfo should not be video")
	}
}
