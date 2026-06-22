package postmortem

import "strings"

type SuspiciousItem struct {
	Category string `json:"category"`
	Marker   string `json:"marker,omitempty"`
	Name     string `json:"name,omitempty"`
	Path     string `json:"path,omitempty"`
	Reason   string `json:"reason"`
}

var pollutedMarkers = []string{
	"RoDubbed",
	"REMASTERED",
	"HMAX",
	"WEB-DL",
	"BluRay",
	"x264",
	"x265",
	"HEVC",
	"HDR",
	"HDR10",
	"HDR10+",
	"Dolby Vision",
	"DV",
	"DDP",
	"DDP5",
	"ATMOS",
	"TrueHD",
	"DTS-HD",
	"DTS",
	"AAC",
	"AC3",
	"EAC3",
	"FLAC",
	"PROPER",
	"REPACK",
	"INTERNAL",
	"AMZN",
	"NF",
	"DSNP",
	"HULU",
	"ATVP",
	"MAX",
	"PCOK",
	"iT",
	"VOSTFR",
	"DCPRIP",
	"HDLight",
	"TrueFrench",
	"VOST",
	"VF",
	"VO",
	"French",
}

func ClassifySuspiciousName(name, path string) SuspiciousItem {
	lowerName := strings.ToLower(name)
	for _, marker := range pollutedMarkers {
		if strings.Contains(lowerName, strings.ToLower(marker)) {
			return SuspiciousItem{
				Category: "polluted_name",
				Marker:   marker,
				Name:     name,
				Path:     path,
				Reason:   "visible title contains release or language marker",
			}
		}
	}
	return SuspiciousItem{}
}

func ClassifyPathMismatch(targetPath, jellyfinPath string) SuspiciousItem {
	t := normalizeStoragePath(targetPath)
	j := normalizeStoragePath(jellyfinPath)
	if t != "" && t == j && targetPath != jellyfinPath {
		return SuspiciousItem{
			Category: "path_translation_false_positive",
			Path:     targetPath,
			Reason:   "daemon path and jellyfin path differ only by configured storage mount alias",
		}
	}
	return SuspiciousItem{}
}

func normalizeStoragePath(path string) string {
	repls := map[string]string{
		"/mnt/STORAGE1/":  "/storage1/",
		"/mnt/STORAGE2/":  "/storage2/",
		"/mnt/STORAGE3/":  "/storage3/",
		"/mnt/STORAGE4/":  "/storage4/",
		"/mnt/STORAGE5/":  "/storage5/",
		"/mnt/STORAGE6/":  "/storage6/",
		"/mnt/STORAGE7/":  "/storage7/",
		"/mnt/STORAGE8/":  "/storage8/",
		"/mnt/STORAGE10/": "/storage10/",
		"/tv1/":           "/storage1/TVSHOWS/",
		"/tv2/":           "/storage2/TVSHOWS/",
		"/tv3/":           "/storage3/TVSHOWS/",
		"/tv4/":           "/storage4/TVSHOWS/",
		"/tv5/":           "/storage5/TVSHOWS/",
		"/tv6/":           "/storage6/TVSHOWS/",
		"/tv7/":           "/storage7/TVSHOWS/",
		"/tv8/":           "/storage8/TVSHOWS/",
		"/tv10/":          "/storage10/TVSHOWS/",
		"/movies1/":       "/storage1/MOVIES/",
		"/movies2/":       "/storage2/MOVIES/",
		"/movies3/":       "/storage3/MOVIES/",
		"/movies4/":       "/storage4/MOVIES/",
		"/movies5/":       "/storage5/MOVIES/",
		"/movies6/":       "/storage6/MOVIES/",
		"/movies7/":       "/storage7/MOVIES/",
		"/movies8/":       "/storage8/MOVIES/",
		"/movies10/":      "/storage10/MOVIES/",
	}
	for from, to := range repls {
		if strings.HasPrefix(path, from) {
			return strings.Replace(path, from, to, 1)
		}
	}
	return path
}
