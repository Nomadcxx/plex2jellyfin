package main

import (
	"fmt"
	"github.com/Nomadcxx/jellywatch/internal/naming"
	"strings"
)

func main() {
	testPath := "/mnt/NVME3/Sabnzbd/complete/tv/Dracula.2020.S01E01.1080p.NF.WEB-DL.DDP5.1.x264-NTG/Dracula.2020.S01E01.1080p.NF.WEB-DL.DDP5.1.x264-NTG.mkv"

	tvWatchPaths := []string{"/mnt/NVME3/Sabnzbd/complete/tv"}
	movieWatchPaths := []string{"/mnt/NVME3/Sabnzbd/complete/movies"}

	fmt.Println("=== Source Hint Test ===")
	fmt.Printf("Test path: %s\n\n", testPath)

	hint := getSourceHint(testPath, tvWatchPaths, movieWatchPaths)
	fmt.Printf("getSourceHint result: %v\n\n", hint)

	fmt.Println("=== IsTVEpisodeFromPath Tests ===")
	fmt.Printf("With SourceUnknown: %v\n", naming.IsTVEpisodeFromPath(testPath, naming.SourceUnknown))
	fmt.Printf("With SourceTV: %v\n", naming.IsTVEpisodeFromPath(testPath, naming.SourceTV))
	fmt.Printf("With SourceMovie: %v\n", naming.IsTVEpisodeFromPath(testPath, naming.SourceMovie))
	fmt.Printf("With computed hint (%v): %v\n", hint, naming.IsTVEpisodeFromPath(testPath, hint))

	fmt.Println("\n=== IsTVEpisodeFilename Test ===")
	filename := "Dracula.2020.S01E01.1080p.NF.WEB-DL.DDP5.1.x264-NTG.mkv"
	fmt.Printf("IsTVEpisodeFilename(%q): %v\n", filename, naming.IsTVEpisodeFilename(filename))
}

func getSourceHint(path string, tvWatchPaths, movieWatchPaths []string) naming.SourceHint {
	for _, tvPath := range tvWatchPaths {
		if strings.HasPrefix(path, tvPath) {
			return naming.SourceTV
		}
	}
	for _, moviePath := range movieWatchPaths {
		if strings.HasPrefix(path, moviePath) {
			return naming.SourceMovie
		}
	}
	return naming.SourceUnknown
}
