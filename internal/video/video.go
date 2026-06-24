package video

import (
	"path/filepath"
	"strings"
)

var Extensions = map[string]bool{
	".mkv": true, ".mp4": true, ".avi": true, ".mov": true,
	".wmv": true, ".flv": true, ".webm": true, ".m4v": true,
	".mpg": true, ".mpeg": true, ".m2ts": true, ".ts": true,
}

func IsVideo(path string) bool {
	return Extensions[strings.ToLower(filepath.Ext(path))]
}

func IsVideoExt(ext string) bool {
	return Extensions[strings.ToLower(ext)]
}
