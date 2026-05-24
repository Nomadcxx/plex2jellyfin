package daemon

import (
	"os"
	"path/filepath"
	"strings"
)

type ingestAction int

const (
	ingestReject ingestAction = iota
	ingestDefer
	ingestAccept
)

func normalizeEventPath(rawPath string, watchRoots []string) (string, string, ingestAction) {
	path := strings.TrimSpace(rawPath)
	if path == "" {
		return "", "empty_path", ingestReject
	}

	path = filepath.Clean(path)
	if !filepath.IsAbs(path) {
		return "", "not_absolute", ingestReject
	}

	if isSABTransientUnpackPath(path) {
		return path, "transient_unpack_path", ingestDefer
	}

	if len(watchRoots) == 0 {
		return path, "", ingestAccept
	}

	for _, root := range watchRoots {
		cleanRoot := filepath.Clean(strings.TrimSpace(root))
		if cleanRoot == "" || !filepath.IsAbs(cleanRoot) {
			continue
		}
		if path == cleanRoot || strings.HasPrefix(path, cleanRoot+string(os.PathSeparator)) {
			return path, "", ingestAccept
		}
	}

	if filepath.Dir(path) == string(os.PathSeparator) {
		return "", "truncated_root_path", ingestReject
	}

	return "", "outside_watch_roots", ingestReject
}

func isSABTransientUnpackPath(path string) bool {
	cleaned := filepath.Clean(path)
	parts := strings.Split(cleaned, string(os.PathSeparator))
	for i, part := range parts {
		if part == "" {
			continue
		}
		if i == len(parts)-1 && filepath.Ext(part) != "" {
			continue
		}
		lower := strings.ToLower(part)
		if strings.HasPrefix(lower, "_unpack_") {
			return true
		}
		tokens := strings.FieldsFunc(lower, func(r rune) bool {
			return r == '.' || r == '_' || r == '-' || r == ' '
		})
		for _, token := range tokens {
			if token == "unpack" {
				return true
			}
		}
	}
	return false
}
