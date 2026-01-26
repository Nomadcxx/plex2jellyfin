package naming

import (
	"fmt"
	"testing"
)

func TestRealWorldPaths(t *testing.T) {
	testPaths := []string{
		"/mnt/NVME3/Sabnzbd/complete/tv/The.White.Lotus.S02E07.Arrivederci.1080p.HMAX.WEB-DL.DDP5.1.x264-NTb/30e2dc4173fc4798bbe5fd40137ed621.mkv",
		"/mnt/NVME3/Sabnzbd/complete/tv/For.All.Mankind.S02E01.1080p.BluRay.x264-TABULARiA/abc123def456ghi789jkl.mkv",
		"/downloads/Inception.2010.1080p.BluRay.x264-GROUP/30e2dc4173fc4798bbe5fd40137ed621.mkv",
		"/downloads/Movie.2020.mkv",
		"/downloads/Show.S01E01.mkv",
	}

	fmt.Println("=== Real World Deobfuscation Test ===")

	for _, path := range testPaths {
		filename := ""
		for i := len(path) - 1; i >= 0; i-- {
			if path[i] == '/' {
				filename = path[i+1:]
				break
			}
		}

		isObfuscated := IsObfuscatedFilename(filename)
		isTVFromPath := IsTVEpisodeFromPath(path, SourceUnknown)

		fmt.Printf("Path: %s\n", path)
		fmt.Printf("  Filename: %s\n", filename)
		fmt.Printf("  Obfuscated: %v\n", isObfuscated)
		fmt.Printf("  Is TV (from path): %v\n", isTVFromPath)

		if isTVFromPath {
			tv, err := ParseTVShowFromPath(path)
			if err != nil {
				fmt.Printf("  Parse Error: %v\n", err)
			} else {
				fmt.Printf("  Parsed: %s Season %d Episode %d\n", tv.Title, tv.Season, tv.Episode)
			}
		} else {
			movie, err := ParseMovieFromPath(path)
			if err != nil {
				fmt.Printf("  Parse Error: %v\n", err)
			} else {
				fmt.Printf("  Parsed: %s (%s)\n", movie.Title, movie.Year)
			}
		}
		fmt.Println()
	}
}
