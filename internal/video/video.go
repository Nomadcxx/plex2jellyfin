package video

import (
	"path/filepath"
	"strings"
	"unicode"
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

// IsSampleVideo identifies sample videos by a Sample/Samples parent directory
// or a filename ending in the sample token.
func IsSampleVideo(path string) bool {
	if !IsVideo(path) {
		return false
	}

	parts := strings.Split(filepath.Clean(path), string(filepath.Separator))
	for _, part := range parts[:len(parts)-1] {
		if strings.EqualFold(strings.TrimSpace(part), "sample") || strings.EqualFold(strings.TrimSpace(part), "samples") {
			return true
		}
	}

	// ponytail: suffix-only matching avoids valid titles containing "sample";
	// if observed releases put sample markers before quality tags, recognize those tags explicitly.
	stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	tokens := strings.FieldsFunc(stem, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	if len(tokens) == 0 {
		return false
	}
	last := tokens[len(tokens)-1]
	return strings.EqualFold(last, "sample") || strings.EqualFold(last, "samples")
}
