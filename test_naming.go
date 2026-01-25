package main

import (
	"fmt"
	"github.com/Nomadcxx/jellywatch/internal/naming"
)

func main() {
	filename := "FAST.&.FURIOUS.2009.1080p.CatchPlay.WEB-DL.H.264.AAC-HHWEB.mkv"
	path := "/mnt/NVME3/Sabnzbd/complete/movies/FAST.&amp;amp;.FURIOUS.2009.1080p.CatchPlay.WEB-DL.H.264.AAC-HHWEB/" + filename

	fmt.Printf("Filename: %s\n", filename)
	fmt.Printf("IsMovieFilename: %v\n", naming.IsMovieFilename(filename))
	fmt.Printf("IsTVEpisodeFilename: %v\n", naming.IsTVEpisodeFilename(filename))
	fmt.Printf("IsTVEpisodeFromPath: %v\n", naming.IsTVEpisodeFromPath(path, naming.SourceUnknown))
	fmt.Printf("IsMovieFromPath: %v\n", naming.IsMovieFromPath(path, naming.SourceUnknown))

	if info, err := naming.ParseMovieName(filename); err == nil {
		fmt.Printf("Movie parse: Title=%q Year=%q\n", info.Title, info.Year)
	} else {
		fmt.Printf("Movie parse error: %v\n", err)
	}
}
